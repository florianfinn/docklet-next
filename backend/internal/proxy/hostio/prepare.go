// Source: docklet-next/issues/663
// Revision: 723c0de3cd289f18b8ddb41e919096e8d3479536
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original pinned staging and rename guards.

package hostio

import (
	"context"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"golang.org/x/sys/unix"
	"path"
)

type preparedFile struct {
	target, stage int
	old, next     unix.Stat_t
	loss          dto.MetadataLoss
}

func (p *preparedFile) close() {
	if p.target >= 0 {
		unix.Close(p.target)
	}
	if p.stage >= 0 {
		unix.Close(p.stage)
	}
}
func prepareAt(fd int, from, to, p string) (*preparedFile, error) {
	r := &preparedFile{target: -1, stage: -1}
	ok := false
	defer func() {
		if !ok {
			r.close()
		}
	}()
	var e error
	r.stage, e = unix.Openat(fd, from, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, unsupported()
	}
	if e = unix.Fstat(r.stage, &r.next); e != nil {
		return nil, e
	}
	if r.next.Mode&unix.S_IFMT != unix.S_IFREG || r.next.Nlink != 1 || r.next.Mode&0044 == 0 || r.next.Mode&07000 != 0 {
		return nil, unsupported()
	}
	r.target, e = unix.Openat(fd, to, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(e, unix.ENOENT) {
		r.target = -1
	} else if e != nil {
		return nil, unsupported()
	} else {
		if e = unix.Fstat(r.target, &r.old); e != nil {
			return nil, e
		}
		if r.old.Mode&unix.S_IFMT != unix.S_IFREG || r.old.Mode&0044 == 0 || r.old.Mode&07000 != 0 {
			return nil, unsupported()
		}
		if e = unix.Fchmod(r.stage, r.old.Mode&0777); e != nil {
			return nil, e
		}
		if e = unix.Fsync(r.stage); e != nil {
			return nil, e
		}
	}
	r.loss, e = CompareMetadata(r.target, r.stage, p)
	if e != nil {
		return nil, e
	}
	ok = true
	return r, nil
}
func (p *preparedFile) same(fd int, from, to string) bool {
	var st unix.Stat_t
	if unix.Fstatat(fd, from, &st, unix.AT_SYMLINK_NOFOLLOW) != nil || st.Dev != p.next.Dev || st.Ino != p.next.Ino {
		return false
	}
	e := unix.Fstatat(fd, to, &st, unix.AT_SYMLINK_NOFOLLOW)
	if p.target < 0 {
		return errors.Is(e, unix.ENOENT)
	}
	return e == nil && st.Dev == p.old.Dev && st.Ino == p.old.Ino
}
func (w ProjectWriter) Prepare(ctx context.Context, from, to string) (dto.MetadataLoss, error) {
	if e := ctx.Err(); e != nil {
		return dto.MetadataLoss{}, e
	}
	fd, name, e := w.parent(to)
	if e != nil {
		return dto.MetadataLoss{}, e
	}
	defer unix.Close(fd)
	if path.Dir(from) != path.Dir(to) {
		return dto.MetadataLoss{}, dto.Fail(dto.InvalidInput, 422)
	}
	p, e := prepareAt(fd, path.Base(from), name, to)
	if e != nil {
		return dto.MetadataLoss{}, e
	}
	defer p.close()
	return p.loss, nil
}

// Move preserves the inode, and refuses an occupied destination. It is used only
// for delete publication and rollback, which replace no file metadata.
func (w ProjectWriter) Move(ctx context.Context, from, to string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	fd, name, e := w.parent(to)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	if path.Dir(from) != path.Dir(to) {
		return dto.Fail(dto.InvalidInput, 422)
	}
	if e = metadata(fd, path.Base(from)); e != nil {
		return e
	}
	if e = unix.Renameat2(fd, path.Base(from), fd, name, unix.RENAME_NOREPLACE); e != nil {
		return e
	}
	return unix.Fsync(fd)
}
