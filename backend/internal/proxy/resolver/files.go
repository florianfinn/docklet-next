// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

package resolver

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"golang.org/x/sys/unix"
)

const maxFile = 1 << 20

type File struct {
	dto.ResolverFile
	Mode   uint32 `json:"mode"`
	Before []byte `json:"-"`
	After  []byte `json:"-"`
}

func snapshot(b []byte, present bool) dto.FileState {
	s := dto.FileState{Present: present}
	if present {
		h := sha256.Sum256(b)
		s.Hash = hex.EncodeToString(h[:])
	}
	return s
}
func relative(p string) bool {
	return p != "" && p != "." && !filepath.IsAbs(p) && filepath.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../") && !strings.ContainsAny(p, "\x00\r\n\\")
}

// parent pins every directory through O_NOFOLLOW, including the configured root.
func parent(root, p string) (int, string, error) {
	if !relative(p) || !filepath.IsAbs(root) {
		return -1, "", dto.Fail(dto.TargetUnclear, 409)
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, "", err
	}
	for _, s := range strings.Split(filepath.Join(root, filepath.Dir(p)), "/") {
		if s == "" || s == "." {
			continue
		}
		next, e := unix.Openat(fd, s, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return -1, "", dto.Fail(dto.UnsupportedMetadata, 409)
		}
		fd = next
	}
	return fd, filepath.Base(p), nil
}
func readAt(fd int, name string) ([]byte, uint32, bool, error) {
	n, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(e, unix.ENOENT) {
		return nil, 0600, false, nil
	}
	if e != nil {
		return nil, 0, false, dto.Fail(dto.UnsupportedMetadata, 409)
	}
	f := os.NewFile(uintptr(n), name)
	defer f.Close()
	var st unix.Stat_t
	if e = unix.Fstat(n, &st); e != nil {
		return nil, 0, false, e
	}
	attrs, e := unix.Flistxattr(n, nil)
	if e != nil || attrs != 0 || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Mode&07000 != 0 || st.Mode&0022 != 0 {
		return nil, 0, false, dto.Fail(dto.UnsupportedMetadata, 409)
	}
	b, e := io.ReadAll(io.LimitReader(f, maxFile+1))
	if len(b) > maxFile {
		return nil, 0, false, dto.Fail(dto.InputLimit, 422)
	}
	return b, st.Mode & 0777, true, e
}
func Read(root, p string) (File, error) {
	fd, n, e := parent(root, p)
	if e != nil {
		return File{}, e
	}
	defer unix.Close(fd)
	b, m, present, e := readAt(fd, n)
	return File{ResolverFile: dto.ResolverFile{Path: p, Old: snapshot(b, present)}, Mode: m, Before: b}, e
}

// Replace compares hashes on the pinned directory immediately before rename or
// unlink. Owner-only .env modes are preserved; new secret files start at 0600.
func Replace(root, p string, expected dto.FileState, data []byte, present bool, mode uint32) error {
	fd, n, e := parent(root, p)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	b, _, exists, e := readAt(fd, n)
	if e != nil {
		return e
	}
	if snapshot(b, exists) != expected {
		return dto.Fail(dto.ThirdVersion, 409)
	}
	if !present {
		if !exists {
			return nil
		}
		if e = unix.Unlinkat(fd, n, 0); e != nil {
			return e
		}
		return unix.Fsync(fd)
	}
	if p == ".env" && mode&0077 != 0 {
		return dto.Fail(dto.UnsupportedMetadata, 409)
	}
	tmp := ".docklet-resolver-" + randomID() + ".tmp"
	nfd, e := unix.Openat(fd, tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(nfd), tmp)
	defer f.Close()
	defer unix.Unlinkat(fd, tmp, 0)
	if _, e = f.Write(data); e != nil {
		return e
	}
	if e = f.Chmod(os.FileMode(mode)); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	b, _, exists, e = readAt(fd, n)
	if e != nil {
		return e
	}
	if snapshot(b, exists) != expected {
		return dto.Fail(dto.ThirdVersion, 409)
	}
	if e = unix.Renameat(fd, tmp, fd, n); e != nil {
		return e
	}
	return unix.Fsync(fd)
}

func accessDirectory(fd int) error {
	defer unix.Close(fd)
	return unix.Faccessat(fd, ".", unix.W_OK, 0)
}

// unsupportedState marks a path that exists but is not a plain readable file.
const unsupportedState = "unsupported"

// observe hashes a path for confirmation without the writer's metadata rules:
// symlinks, special files and unreadable paths get a distinct stable state.
func observe(root, p string) (dto.FileState, error) {
	if !relative(p) || !filepath.IsAbs(root) {
		return dto.FileState{}, dto.Fail(dto.TargetUnclear, 409)
	}
	n, e := unix.Open(filepath.Join(root, p), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(e, unix.ENOENT) || errors.Is(e, unix.ENOTDIR) {
		return dto.FileState{}, nil
	}
	if errors.Is(e, unix.EIO) || errors.Is(e, unix.EAGAIN) || errors.Is(e, unix.EINTR) {
		return dto.FileState{}, e
	}
	if e != nil {
		return dto.FileState{Present: true, Hash: unsupportedState}, nil
	}
	f := os.NewFile(uintptr(n), p)
	defer f.Close()
	var st unix.Stat_t
	if e = unix.Fstat(n, &st); e != nil {
		return dto.FileState{}, e
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return dto.FileState{Present: true, Hash: unsupportedState}, nil
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return dto.FileState{}, e
	}
	return dto.FileState{Present: true, Hash: hex.EncodeToString(h.Sum(nil))}, nil
}
