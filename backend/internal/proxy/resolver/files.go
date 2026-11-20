// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

package resolver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	// WriteMode overrides Mode for the forward write only (a changed .env is
	// written owner-only); restore always uses the original Mode.
	WriteMode uint32 `json:"-"`
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
		return -1, "", refuse(err)
	}
	for _, s := range strings.Split(filepath.Join(root, filepath.Dir(p)), "/") {
		if s == "" || s == "." {
			continue
		}
		next, e := unix.Openat(fd, s, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return -1, "", refuse(e)
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
		return nil, 0, false, refuse(e)
	}
	f := os.NewFile(uintptr(n), name)
	defer f.Close()
	var st unix.Stat_t
	if e = unix.Fstat(n, &st); e != nil {
		return nil, 0, false, refuse(e)
	}
	attrs, e := unix.Flistxattr(n, nil)
	if e != nil {
		return nil, 0, false, refuse(e)
	}
	if attrs != 0 || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Mode&07000 != 0 || st.Mode&0022 != 0 {
		return nil, 0, false, dto.Fail(dto.UnsupportedMetadata, 409)
	}
	b, e := io.ReadAll(io.LimitReader(f, maxFile+1))
	if e != nil {
		return nil, 0, false, refuse(e)
	}
	if len(b) > maxFile {
		return nil, 0, false, dto.Fail(dto.InputLimit, 422)
	}
	return b, st.Mode & 0777, true, nil
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
// unlink and writes with the given mode; Build decides the .env write mode.
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

// transient reports syscall errors that may succeed on retry; every other
// refusal of a touched path is persistent.
func transient(e error) bool {
	for _, t := range []unix.Errno{unix.EIO, unix.EAGAIN, unix.EINTR, unix.ENOMEM, unix.ENFILE, unix.EMFILE} {
		if errors.Is(e, t) {
			return true
		}
	}
	return false
}
func refuse(e error) error {
	if transient(e) {
		return e
	}
	return dto.Fail(dto.UnsupportedMetadata, 409)
}

// observe fingerprints a path for confirmation without the writer's metadata
// rules. It walks pinned no-follow components like parent. A regular file up to
// maxFile keeps its content hash; anything else (symlink, other type, oversized
// or obstructing component) gets a bounded, change-sensitive descriptor.
func observe(root, p string) (dto.FileState, error) {
	if !relative(p) || !filepath.IsAbs(root) {
		return dto.FileState{}, dto.Fail(dto.TargetUnclear, 409)
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		if transient(e) {
			return dto.FileState{}, e
		}
		var st unix.Stat_t
		if er := unix.Lstat("/", &st); er != nil {
			if transient(er) {
				return dto.FileState{}, er
			}
			return descriptor(-1, nil, "", e), nil
		}
		return descriptor(-1, &st, "", e), nil
	}
	parts := []string{}
	for _, s := range strings.Split(filepath.Join(root, p), "/") {
		if s != "" {
			parts = append(parts, s)
		}
	}
	// walked is the no-follow stat of the directory fd currently pins; it stands in
	// when fresh metadata of that pinned directory is persistently refused.
	var walked *unix.Stat_t
	for i, s := range parts {
		var st unix.Stat_t
		if e = unix.Fstatat(fd, s, &st, unix.AT_SYMLINK_NOFOLLOW); e != nil {
			defer unix.Close(fd)
			if errors.Is(e, unix.ENOENT) {
				return dto.FileState{}, nil
			}
			if transient(e) {
				return dto.FileState{}, e
			}
			// The obstructing parent (e.g. no search permission) is fingerprinted, so
			// replacing it changes the confirmation.
			parent, er := statFD(fd)
			if er != nil {
				return dto.FileState{}, er
			}
			if parent == nil {
				parent = walked
			}
			return descriptor(i, parent, "", e), nil
		}
		kind := st.Mode & unix.S_IFMT
		if i < len(parts)-1 && kind == unix.S_IFDIR {
			next, er := unix.Openat(fd, s, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if er != nil {
				defer unix.Close(fd)
				if transient(er) {
					return dto.FileState{}, er
				}
				return descriptor(i, &st, "", er), nil
			}
			unix.Close(fd)
			fd, walked = next, &st
			continue
		}
		defer unix.Close(fd)
		if kind == unix.S_IFLNK {
			buf := make([]byte, unix.PathMax)
			n, er := unix.Readlinkat(fd, s, buf)
			if transient(er) {
				return dto.FileState{}, er
			}
			return descriptor(i, &st, string(buf[:max(n, 0)]), er), nil
		}
		if i < len(parts)-1 || kind != unix.S_IFREG || st.Size > maxFile {
			return descriptor(i, &st, "", nil), nil
		}
		return content(fd, s, i, st)
	}
	unix.Close(fd)
	return dto.FileState{}, dto.Fail(dto.TargetUnclear, 409)
}

// statFD stats a pinned descriptor. A persistent Fstat refusal falls back to a
// descriptor-relative no-follow stat; nil means nothing is observable.
func statFD(fd int) (*unix.Stat_t, error) {
	var st unix.Stat_t
	e := unix.Fstat(fd, &st)
	if e == nil {
		return &st, nil
	}
	if transient(e) {
		return nil, e
	}
	if e = unix.Fstatat(fd, "", &st, unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW); e == nil {
		return &st, nil
	}
	if transient(e) {
		return nil, e
	}
	return nil, nil
}

// content hashes a regular file that still matches its pinned stat; growth past
// maxFile or replacement after stat falls back to a descriptor.
func content(dir int, name string, i int, st unix.Stat_t) (dto.FileState, error) {
	n, e := unix.Openat(dir, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		if transient(e) {
			return dto.FileState{}, e
		}
		return descriptor(i, &st, "", e), nil
	}
	f := os.NewFile(uintptr(n), name)
	defer f.Close()
	var now unix.Stat_t
	if e = unix.Fstat(n, &now); e != nil {
		if transient(e) {
			return dto.FileState{}, e
		}
		return descriptor(i, &st, "", e), nil
	}
	if now.Dev != st.Dev || now.Ino != st.Ino || now.Mode&unix.S_IFMT != unix.S_IFREG {
		return descriptor(i, &now, "", nil), nil
	}
	b, e := io.ReadAll(io.LimitReader(f, maxFile+1))
	if e != nil {
		if transient(e) {
			return dto.FileState{}, e
		}
		return descriptor(i, &now, "", e), nil
	}
	if len(b) > maxFile {
		return descriptor(i, &now, "", nil), nil
	}
	return snapshot(b, true), nil
}

// descriptor never reads content: component index, type, identity, size,
// change times, link destination and a persistent error code.
func descriptor(i int, st *unix.Stat_t, link string, e error) dto.FileState {
	d := struct {
		Component int    `json:"component"`
		Mode      uint32 `json:"mode,omitempty"`
		Dev       uint64 `json:"dev,omitempty"`
		Ino       uint64 `json:"ino,omitempty"`
		Size      int64  `json:"size,omitempty"`
		Mtime     int64  `json:"mtime,omitempty"`
		Ctime     int64  `json:"ctime,omitempty"`
		Link      string `json:"link,omitempty"`
		Error     string `json:"error,omitempty"`
	}{Component: i, Link: link}
	if st != nil {
		d.Mode, d.Dev, d.Ino, d.Size = st.Mode, st.Dev, st.Ino, st.Size
		d.Mtime, d.Ctime = st.Mtim.Nano(), st.Ctim.Nano()
	}
	if e != nil {
		d.Error = e.Error()
	}
	b, _ := json.Marshal(d)
	h := sha256.Sum256(b)
	return dto.FileState{Present: true, Hash: "meta:" + hex.EncodeToString(h[:])}
}
