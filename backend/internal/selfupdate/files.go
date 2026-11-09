// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const maxRecordBytes = 64 << 10

func ReadJSON(path string, out any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxRecordBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxRecordBytes {
		return errors.New("self-update record too large")
	}
	return json.Unmarshal(raw, out)
}
func WriteJSON(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return atomicFile(path, raw, 0640, nil)
}

// atomicFile never chowns. A same-owner temporary file is required before the
// rename, preserving the original mode/owner or refusing before replacement.
func atomicFile(path string, raw []byte, mode os.FileMode, owner *syscall.Stat_t) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".docklet-self-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(mode.Perm()); err != nil {
		return err
	}
	if owner != nil {
		info, err := f.Stat()
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != owner.Uid || stat.Gid != owner.Gid {
			return Refuse(CodeLayout)
		}
	}
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func writePin(raw []byte, variable, target string) []byte {
	lines := strings.Split(string(raw), "\n")
	out := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		key, _, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export ")), "=")
		if ok && strings.TrimSpace(key) == variable {
			continue
		}
		out = append(out, line)
	}
	text := strings.TrimRight(strings.Join(out, "\n"), "\n")
	if text != "" {
		text += "\n"
	}
	return []byte(text + variable + "=" + target + "\n")
}
func backupEnv(path string, raw []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chmod(mode.Perm()); err != nil {
		return err
	}
	if _, err = f.Write(raw); err != nil {
		return err
	}
	return f.Sync()
}
func envBytesEqual(path string, raw []byte) bool {
	current, err := os.ReadFile(path)
	return err == nil && bytes.Equal(raw, current)
}
