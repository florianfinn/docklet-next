// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy host boundary.

package hostio

import (
	"context"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"go.getarcane.app/acfs"
	"golang.org/x/sys/unix"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ProjectWriter uses directory descriptors and O_NOFOLLOW for every segment.
// LogicalPath shares the projects confinement contract; rename and parent sync
// happen on the same pinned descriptor, including deletion and restoration.
type ProjectWriter struct{ Root string }

func (w ProjectWriter) parent(target string) (int, string, error) {
	abs := filepath.Join(w.Root, target)
	if _, err := acfs.LogicalPath(w.Root, abs); err != nil {
		return -1, "", err
	}
	if !Relative(target) {
		return -1, "", dto.Fail(dto.InvalidInput, 422)
	}
	root, err := filepath.Abs(w.Root)
	if err != nil {
		return -1, "", err
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, "", err
	}
	parts := strings.Split(strings.TrimPrefix(filepath.ToSlash(filepath.Join(root, path.Dir(target))), "/"), "/")
	for _, s := range parts {
		if s == "" || s == "." {
			continue
		}
		next, er := unix.Openat(fd, s, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if er != nil {
			return -1, "", dto.Fail(dto.SourceReadOnly, 409)
		}
		fd = next
	}
	return fd, path.Base(target), nil
}
func metadata(fd int, name string) error {
	file, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return dto.Fail(dto.UnsupportedMetadata, 409)
	}
	defer unix.Close(file)
	var st unix.Stat_t
	if err = unix.Fstat(file, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Mode&0044 == 0 || st.Mode&07000 != 0 {
		return dto.Fail(dto.UnsupportedMetadata, 409)
	}
	n, err := unix.Flistxattr(file, nil)
	if err != nil || n != 0 {
		return dto.Fail(dto.UnsupportedMetadata, 409)
	}
	return nil
}
func (w ProjectWriter) Write(ctx context.Context, target string, b []byte, mode uint32) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(b) > MaxFileBytes {
		return dto.Fail(dto.InputLimit, 422)
	}
	fd, name, err := w.parent(target)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if err = metadata(fd, name); err != nil {
		return err
	}
	temp, err := unix.Openat(fd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, mode&0777)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(temp), name)
	defer f.Close()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Chmod(os.FileMode(mode & 0777)); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	return unix.Fsync(fd)
}
func (w ProjectWriter) Replace(ctx context.Context, from, to string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fd, name, err := w.parent(to)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if path.Dir(from) != path.Dir(to) {
		return dto.Fail(dto.InvalidInput, 422)
	}
	if err = metadata(fd, name); err != nil {
		return err
	}
	if err = metadata(fd, path.Base(from)); err != nil {
		return err
	}
	var target unix.Stat_t
	if err = unix.Fstatat(fd, name, &target, unix.AT_SYMLINK_NOFOLLOW); err == nil {
		staged, er := unix.Openat(fd, path.Base(from), unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if er != nil {
			return er
		}
		er = unix.Fchmod(staged, target.Mode&0777)
		if er == nil {
			er = unix.Fsync(staged)
		}
		_ = unix.Close(staged)
		if er != nil {
			return er
		}
	} else if !errors.Is(err, unix.ENOENT) {
		return err
	}
	if err = unix.Renameat(fd, path.Base(from), fd, name); err != nil {
		return err
	}
	return unix.Fsync(fd)
}
func (w ProjectWriter) Remove(ctx context.Context, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fd, name, err := w.parent(target)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if !strings.HasPrefix(name, ".docklet-") && !strings.HasPrefix(name, "docklet-check-") {
		if err = metadata(fd, name); err != nil {
			return err
		}
	}
	if err = unix.Unlinkat(fd, name, 0); errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	return unix.Fsync(fd)
}

func (w ProjectWriter) Mkdir(ctx context.Context, p string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fd, name, err := w.parent(p)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if err = unix.Mkdirat(fd, name, 0755); err != nil {
		return err
	}
	return unix.Fsync(fd)
}
func (w ProjectWriter) RemoveTree(ctx context.Context, p string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !Relative(p) || !strings.HasPrefix(path.Base(p), ".docklet-staging-") {
		return dto.Fail(dto.InvalidInput, 422)
	}
	fd, _, err := w.parent(p)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	root, err := os.OpenRoot(w.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err = root.RemoveAll(p); err != nil {
		return err
	}
	return unix.Fsync(fd)
}
