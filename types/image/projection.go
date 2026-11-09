// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229; M21 compares tag-pattern candidates by their version.

package image

import (
	"strconv"
	"strings"
)

// ProjectUpdate compares store-native keys at one verified platform: config
// digests under classic, platform manifests under containerd. Callers prove roles.
func ProjectUpdate(info *UpdateInfo) {
	info.CheckedAt = info.CheckTime
	if info.AuthState == "" {
		info.AuthState = "unknown"
		switch info.AuthMethod {
		case "anonymous", "credential", "authentication-required", "authentication-failed":
			info.AuthState = info.AuthMethod
		}
		if info.UsedCredential {
			info.AuthState = "credential"
		}
	}
	info.AuthUsername = ""
	info.UsedCredential = info.AuthState == "credential"
	if info.AuthState == "credential" || info.AuthState == "anonymous" {
		info.AuthMethod = info.AuthState
	}
	if info.Error != "" {
		info.State = "error"
		info.HasUpdate = false
		if info.Code == "" {
			info.Code = "update.check-unavailable"
		}
		info.Error = "The update check could not be completed."
		return
	}
	if info.CheckTime.IsZero() {
		info.State = "unchecked"
		info.HasUpdate = false
		return
	}
	if info.UpdateType == "local" {
		info.State = "local"
		info.HasUpdate = false
		return
	}
	if strings.Contains(info.ImageRef, "@sha256:") {
		info.State = "current"
		info.HasUpdate = false
		return
	}
	if info.Platform == "" || info.CurrentDigest == "" || info.LatestDigest == "" {
		info.State = "unknown"
		info.HasUpdate = false
		return
	}
	info.State = "current"
	info.HasUpdate = false
	if info.CurrentDigest != info.LatestDigest {
		info.State = "rebuild"
		info.HasUpdate = true
	}
	currentVersion, latestVersion := comparableVersions(info)
	current := "v" + strings.TrimPrefix(currentVersion, "v")
	candidate := "v" + strings.TrimPrefix(latestVersion, "v")
	if compared, valid := CompareRelease(candidate, current); valid && compared > 0 && info.CurrentDigest != info.LatestDigest {
		info.State = "version"
		info.HasUpdate = true
		a := strings.Split(strings.TrimPrefix(currentVersion, "v"), ".")
		b := strings.Split(strings.TrimPrefix(latestVersion, "v"), ".")
		info.UpdateType = "patch"
		if a[0] != b[0] {
			info.UpdateType = "major"
		} else if a[1] != b[1] {
			info.UpdateType = "minor"
		}
	}
}

// comparableVersions cuts a tag-pattern candidate to its version. The checker
// stores the current tag's version capture but the candidate's literal tag, which
// execution needs; the candidate loses the prefix and suffix around the current
// capture in the current tag. Other shapes compare unchanged.
func comparableVersions(info *UpdateInfo) (string, string) {
	current, latest := info.CurrentVersion, info.LatestVersion
	name, _, _ := strings.Cut(info.ImageRef, "@")
	colon := strings.LastIndex(name, ":")
	if current == "" || colon < strings.LastIndex(name, "/") {
		return current, latest
	}
	tag := name[colon+1:]
	at := strings.Index(tag, current)
	if tag == current || at < 0 {
		return current, latest
	}
	prefix, suffix := tag[:at], tag[at+len(current):]
	if len(latest) > len(prefix)+len(suffix) && strings.HasPrefix(latest, prefix) && strings.HasSuffix(latest, suffix) {
		latest = latest[len(prefix) : len(latest)-len(suffix)]
	}
	return current, latest
}

// CompareRelease accepts stable numeric SemVer releases; prerelease/ambiguous
// labels never become a version candidate through lexical or time ordering.
func CompareRelease(a, b string) (int, bool) {
	parse := func(raw string) ([3]uint64, bool) {
		var values [3]uint64
		raw = strings.TrimPrefix(raw, "v")
		raw, _, _ = strings.Cut(raw, "+")
		parts := strings.Split(raw, ".")
		if len(parts) != 3 {
			return values, false
		}
		for i, p := range parts {
			if p == "" || (len(p) > 1 && p[0] == '0') {
				return values, false
			}
			for _, ch := range p {
				if ch < '0' || ch > '9' {
					return values, false
				}
			}
			v, e := strconv.ParseUint(p, 10, 64)
			if e != nil {
				return values, false
			}
			values[i] = v
		}
		return values, true
	}
	av, ok := parse(a)
	if !ok {
		return 0, false
	}
	bv, ok := parse(b)
	if !ok {
		return 0, false
	}
	for i := range av {
		if av[i] > bv[i] {
			return 1, true
		}
		if av[i] < bv[i] {
			return -1, true
		}
	}
	return 0, true
}
