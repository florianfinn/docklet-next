// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy contract foundation.

// Package hostio confines and bounds proxy container I/O before engines parse it.
package hostio

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"io"
	"path"
	"strings"
	"time"
)

const (
	MaxFiles      = 512
	MaxFileBytes  = 1 << 20
	MaxDepth      = 8
	MaxTotalBytes = 16 << 20
	MaxExecBytes  = 256 << 10
	MaxHeaders    = 4096
)

func Secret(p string) bool {
	for _, s := range strings.Split(p, "/") {
		if strings.Contains(strings.ToLower(s), ".secret") {
			return true
		}
	}
	return false
}
func Relative(p string) bool {
	return p != "" && !path.IsAbs(p) && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../") && !strings.ContainsAny(p, "\x00\r\n\\")
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (c contextReader) Read(b []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(b)
}
func bad(p string, c dto.Reason) error { return &dto.Problem{Status: 422, Code: c, Path: p} }

// ReadTar accepts Docker's basename-rooted archive. Secret headers are projected
// before type checks and before payload decoding; Next discards their payload.
type FileHeader struct {
	Path          string
	Type          byte
	Size          int64
	Mode          uint32
	ModifiedAt    time.Time
	HasAttributes bool
}

func ReadTar(ctx context.Context, r io.Reader, root string, single bool) ([]dto.File, []dto.OpaqueFile, error) {
	return ReadTarWithHeaders(ctx, r, root, single, nil)
}

// Header metadata stays inside the host engine. Opaque API projections still
// contain names only; engines use modification times to fence provider health.
func ReadTarWithHeaders(ctx context.Context, r io.Reader, root string, single bool, visit func(FileHeader) error) ([]dto.File, []dto.OpaqueFile, error) {
	tr := tar.NewReader(contextReader{ctx, r})
	files := []dto.File{}
	opaque := []dto.OpaqueFile{}
	seen := map[string]bool{}
	var total int64
	count := 0
	root = path.Base(path.Clean(root))
	for headers := 0; ; headers++ {
		if headers >= MaxHeaders {
			return nil, nil, bad(root, dto.InputLimit)
		}
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			return nil, nil, bad(root, dto.InvalidInput)
		}
		name := strings.TrimSuffix(h.Name, "/")
		if !Relative(name) {
			return nil, nil, bad(name, dto.InvalidInput)
		}
		if single {
			if name != root || h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA || count != 0 {
				return nil, nil, bad(name, dto.InvalidInput)
			}
		} else if name != root && !strings.HasPrefix(name, root+"/") {
			return nil, nil, bad(name, dto.InvalidInput)
		}
		rel := strings.TrimPrefix(name, root+"/")
		if name == root && !single {
			if h.Typeflag != tar.TypeDir {
				return nil, nil, bad(name, dto.InvalidInput)
			}
			continue
		}
		if seen[rel] {
			return nil, nil, bad(rel, dto.InvalidInput)
		}
		seen[rel] = true
		if strings.Count(rel, "/")+1 > MaxDepth {
			return nil, nil, bad(rel, dto.InputLimit)
		}
		if h.Size < 0 {
			return nil, nil, bad(rel, dto.InputLimit)
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		count++
		if count > MaxFiles {
			return nil, nil, bad(rel, dto.InputLimit)
		}
		// Byte limits count only payloads that are read; secret payloads are skipped.
		if !Secret(name) {
			if h.Size > MaxFileBytes {
				return nil, nil, bad(rel, dto.InputLimit)
			}
			total += h.Size
			if total > MaxTotalBytes {
				return nil, nil, bad(rel, dto.InputLimit)
			}
		}
		if visit != nil {
			if err := visit(FileHeader{Path: rel, Type: h.Typeflag, Size: h.Size, Mode: uint32(h.Mode) & 0777, ModifiedAt: h.ModTime, HasAttributes: len(h.Xattrs) > 0}); err != nil {
				return nil, nil, err
			}
		}
		if Secret(name) {
			opaque = append(opaque, dto.OpaqueFile{Path: rel})
			continue
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return nil, nil, bad(rel, dto.InvalidInput)
		}
		b, err := io.ReadAll(io.LimitReader(tr, MaxFileBytes+1))
		if err != nil {
			return nil, nil, err
		}
		if int64(len(b)) != h.Size {
			return nil, nil, bad(rel, dto.InvalidInput)
		}
		sum := sha256.Sum256(b)
		files = append(files, dto.File{Path: rel, Content: string(b), Hash: hex.EncodeToString(sum[:]), Mode: uint32(h.Mode) & 0777, ModifiedAt: h.ModTime})
	}
	if single && count != 1 {
		return nil, nil, bad(root, dto.InvalidInput)
	}
	return files, opaque, nil
}
