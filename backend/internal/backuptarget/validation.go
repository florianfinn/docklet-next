// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original target input and immutable-location validation.

package backuptarget

import (
	"net"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	backup "github.com/florianfinn/docklet-next/backend/internal/backup/keyformat"
	dto "github.com/florianfinn/docklet-next/types/backup"
)

var hostPattern = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$`)
var modePattern = regexp.MustCompile(`^0[0-7]{3}$`)

func invalid() error { return dto.NewProblem(422, "backup.target.invalid", nil) }

// Validate normalizes a target before either hub persistence or agent set admission.
func Validate(input dto.TargetInput) (dto.TargetInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	if len(input.Name) < 1 || len(input.Name) > 64 || strings.ContainsAny(input.Name, "\x00\r\n") {
		return input, invalid()
	}
	switch input.Kind {
	case "local", "mount":
		if input.HostID == nil || strings.TrimSpace(*input.HostID) == "" || !validAbsolute(input.Path) || input.Server != "" || input.Share != "" || input.Username != "" || input.Secret != nil && *input.Secret != "" || input.Options != "" {
			return input, invalid()
		}
		input.Path = path.Clean(input.Path)
	case "smb", "nfs":
		if input.HostID != nil || len(input.Server) > 253 || (net.ParseIP(input.Server) == nil && !hostPattern.MatchString(input.Server)) || !validRelative(input.Path) {
			return input, invalid()
		}
		input.Server = strings.ToLower(input.Server)
		input.Path = path.Clean(input.Path)
		if input.Path == "." {
			input.Path = ""
		}
		if input.Kind == "smb" {
			if len(input.Share) < 1 || len(input.Share) > 80 || strings.ContainsAny(input.Share, "/\\\x00\r\n") || strings.ContainsAny(input.Username, ",=\x00\r\n") || input.Secret != nil && strings.ContainsAny(*input.Secret, ",=\x00\r\n") {
				return input, invalid()
			}
		} else {
			if !path.IsAbs(input.Share) || hasDotDot(input.Share) || strings.ContainsAny(input.Share, "\x00\r\n") || input.Username != "" || input.Secret != nil && *input.Secret != "" {
				return input, invalid()
			}
			input.Share = path.Clean(input.Share)
		}
		options, err := normalizeOptions(input.Kind, input.Options)
		if err != nil {
			return input, err
		}
		input.Options = options
	default:
		return input, invalid()
	}
	if input.RecoveryKey != "" {
		input.RecoveryKey = strings.TrimSpace(input.RecoveryKey)
		if backup.ValidateRecoveryKey(input.RecoveryKey) != nil {
			return input, invalid()
		}
	}
	return input, nil
}

func hasDotDot(value string) bool {
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}
func validAbsolute(value string) bool {
	if !path.IsAbs(value) || len(value) > 255 || hasDotDot(value) || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	value = path.Clean(value)
	if value == "/" {
		return false
	}
	for _, prefix := range []string{"/proc", "/sys", "/dev", "/etc", "/var/lib/docker"} {
		if value == prefix || strings.HasPrefix(value, prefix+"/") {
			return false
		}
	}
	return true
}
func validRelative(value string) bool {
	return !path.IsAbs(value) && len(value) <= 255 && !hasDotDot(value) && !strings.ContainsAny(value, "\x00\r\n")
}
func normalizeOptions(kind, value string) (string, error) {
	if value == "" && kind == "nfs" {
		return "hard", nil
	}
	if value == "" {
		return "", nil
	}
	options := strings.Split(value, ",")
	seen := map[string]bool{}
	for _, option := range options {
		key, val, has := strings.Cut(option, "=")
		if key == "" || seen[key] || strings.ContainsAny(val, ",=\x00\r\n") {
			return "", invalid()
		}
		seen[key] = true
		valid := false
		if kind == "smb" {
			switch key {
			case "vers":
				valid = has && contains([]string{"2.0", "2.1", "3.0", "3.02", "3.1.1"}, val)
			case "domain":
				valid = has && val != ""
			case "seal":
				valid = !has
			case "uid", "gid":
				valid = has && decimal(val, 0, 4294967295)
			case "file_mode", "dir_mode":
				valid = has && modePattern.MatchString(val)
			}
		} else {
			switch key {
			case "nfsvers":
				valid = has && contains([]string{"3", "4", "4.1", "4.2"}, val)
			case "hard", "soft", "nolock":
				valid = !has
			case "timeo", "retrans":
				valid = has && decimal(val, 1, 2147483647)
			}
		}
		if !valid {
			return "", invalid()
		}
	}
	if kind == "nfs" {
		if seen["hard"] && seen["soft"] {
			return "", invalid()
		}
		if !seen["hard"] && !seen["soft"] {
			options = append(options, "hard")
		}
	}
	sort.Strings(options)
	return strings.Join(options, ","), nil
}
func decimal(value string, min, max uint64) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, err := strconv.ParseUint(value, 10, 64)
	return err == nil && n >= min && n <= max
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func sameLocation(a, b dto.TargetLocation) bool {
	return a.Kind == b.Kind && equalHost(a.HostID, b.HostID) && a.Path == b.Path && a.Server == b.Server && a.Share == b.Share
}
func equalHost(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
func (t Target) location() dto.TargetLocation {
	return dto.TargetLocation{Kind: t.Kind, HostID: t.HostID, Path: t.Path, Server: t.Server, Share: t.Share}
}
