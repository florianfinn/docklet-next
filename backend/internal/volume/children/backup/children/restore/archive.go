// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original two-pass portable archive validation and normalized tar bridge.

package restore

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"path"
	"strings"

	dto "github.com/florianfinn/docklet-next/types/backup"
)

const maxArchiveEntries = 200000

type ArchiveValidation struct {
	SHA256       string
	Entries      int
	RegularBytes int64
}

func archiveProblem(reason string, cause error) error {
	problem := dto.NewProblem(422, "volume.backup.upload-invalid", cause)
	problem.Reason = reason
	return problem
}

// ValidateArchive reads the complete gzip stream, including its checksum, and
// never touches a destination. The expanded-byte cap is independent of daemon-host filesystem capacity.
func ValidateArchive(input io.Reader, freeBytes int64) (ArchiveValidation, error) {
	return archivePass(input, freeBytes, nil, "")
}

// NormalizeArchive repeats validation while re-encoding allowed headers only.
// The caller copies this stream exclusively to an operation-owned scratch helper.
// Hash verification precedes the end-of-tar marker, so changed input fails copy.
func NormalizeArchive(input io.Reader, output io.Writer, freeBytes int64, expected string) error {
	_, err := archivePass(input, freeBytes, output, expected)
	return err
}
func archivePass(input io.Reader, freeBytes int64, output io.Writer, expected string) (result ArchiveValidation, err error) {
	if freeBytes < 0 {
		return result, archiveProblem("expanded-too-large", nil)
	}
	budget := min(freeBytes, dto.MaxExpandedUploadBytes)
	hash := sha256.New()
	gz, err := gzip.NewReader(io.TeeReader(input, hash))
	if err != nil {
		return result, archiveProblem("format", err)
	}
	defer gz.Close()
	counted := &archiveCounter{Reader: gz}
	tr := tar.NewReader(counted)
	var tw *tar.Writer
	if output != nil {
		tw = tar.NewWriter(output)
	}
	seen := map[string]byte{}
	descendants := map[string]bool{}
	for {
		header, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return result, archiveProblem("format", e)
		}
		result.Entries++
		if result.Entries > maxArchiveEntries {
			return result, archiveProblem("too-many-entries", nil)
		}
		name, e := archiveName(header.Name)
		if e != nil {
			return result, e
		}
		if _, exists := seen[name]; exists {
			return result, archiveProblem("duplicate", nil)
		}
		if header.Typeflag == tar.TypeSymlink && descendants[name] {
			return result, archiveProblem("link", nil)
		}
		for ancestor := path.Dir(name); ancestor != "." && ancestor != "/"; ancestor = path.Dir(ancestor) {
			descendants[ancestor] = true
			if seen[ancestor] == tar.TypeSymlink {
				return result, archiveProblem("link", nil)
			}
		}
		for key := range header.PAXRecords {
			if strings.HasPrefix(key, "GNU.sparse.") || key == "SCHILY.realsize" {
				return result, archiveProblem("entry-type", nil)
			}
		}
		link := header.Linkname
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > budget-result.RegularBytes {
				return result, archiveProblem("expanded-too-large", nil)
			}
			result.RegularBytes += header.Size
		case tar.TypeDir, tar.TypeSymlink:
		case tar.TypeLink:
			link, e = archiveName(header.Linkname)
			if e != nil {
				return result, archiveProblem("link", e)
			}
			kind, exists := seen[link]
			if !exists || kind != tar.TypeReg && kind != tar.TypeRegA {
				return result, archiveProblem("link", nil)
			}
		default:
			return result, archiveProblem("entry-type", nil)
		}
		if name == "." && header.Typeflag != tar.TypeDir {
			return result, archiveProblem("path", nil)
		}
		seen[name] = header.Typeflag
		if tw != nil {
			clean := &tar.Header{Name: name, Typeflag: header.Typeflag, Linkname: link, Mode: header.Mode, ModTime: header.ModTime, Uid: header.Uid, Gid: header.Gid, Size: header.Size, Format: tar.FormatPAX}
			if e = tw.WriteHeader(clean); e != nil {
				return result, archiveProblem("format", e)
			}
			if _, e = io.Copy(tw, tr); e != nil {
				return result, archiveProblem("format", e)
			}
		} else if _, e = io.Copy(io.Discard, tr); e != nil {
			return result, archiveProblem("format", e)
		}
	}
	if counted.n < 1024 {
		return result, archiveProblem("format", nil)
	}
	// Reading to EOF validates gzip CRC/trailer, including concatenated members.
	if _, err = io.Copy(io.Discard, gz); err != nil {
		return result, archiveProblem("format", err)
	}
	result.SHA256 = hex.EncodeToString(hash.Sum(nil))
	if expected != "" && result.SHA256 != expected {
		return result, archiveProblem("format", errors.New("archive changed between validation passes"))
	}
	if tw != nil {
		if err = tw.Close(); err != nil {
			return result, archiveProblem("format", err)
		}
	}
	return result, nil
}
func archiveName(value string) (string, error) {
	if value == "" || path.IsAbs(value) || strings.ContainsRune(value, '\x00') {
		return "", archiveProblem("path", nil)
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return "", archiveProblem("path", nil)
		}
	}
	return path.Clean(value), nil
}

type archiveCounter struct {
	io.Reader
	n int64
}

func (r *archiveCounter) Read(b []byte) (int, error) {
	n, err := r.Reader.Read(b)
	r.n += int64(n)
	return n, err
}
