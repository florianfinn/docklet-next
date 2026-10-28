// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original structured archive validation and confined extraction proof.

package restore

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
)

type member struct {
	name, link, body string
	kind             byte
	pax              map[string]string
}

func archive(t *testing.T, members ...member) []byte {
	t.Helper()
	var result bytes.Buffer
	gz := gzip.NewWriter(&result)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		header := &tar.Header{Name: m.name, Typeflag: m.kind, Linkname: m.link, Mode: 0640, Uid: 123, Gid: 456, PAXRecords: m.pax}
		if m.kind == tar.TypeReg || m.kind == tar.TypeRegA {
			header.Size = int64(len(m.body))
		}
		require.NoError(t, tw.WriteHeader(header))
		if header.Size > 0 {
			_, err := tw.Write([]byte(m.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return result.Bytes()
}
func reason(t *testing.T, err error, want string) {
	t.Helper()
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, "volume.backup.upload-invalid", p.Code)
	require.Equal(t, want, p.Reason)
}

func TestArchiveValidationRulesLeaveTargetAndOutsideUnchanged(t *testing.T) {
	cases := []struct {
		name, want string
		members    []member
		raw        []byte
		free       int64
	}{
		{name: "absolute", want: "path", members: []member{{name: "/outside", kind: tar.TypeReg}}},
		{name: "parent", want: "path", members: []member{{name: "dir/../outside", kind: tar.TypeReg}}},
		{name: "duplicate-type", want: "duplicate", members: []member{{name: "dir", kind: tar.TypeDir}, {name: "./dir", kind: tar.TypeSymlink, link: "../outside"}}},
		{name: "symlink-child", want: "link", members: []member{{name: "dir", kind: tar.TypeSymlink, link: "../outside"}, {name: "dir/file", kind: tar.TypeReg, body: "new"}}},
		{name: "symlink-descendant", want: "link", members: []member{{name: "dir", kind: tar.TypeSymlink, link: "../outside"}, {name: "dir/sub/file", kind: tar.TypeReg, body: "new"}}},
		{name: "hardlink-symlink", want: "link", members: []member{{name: "link", kind: tar.TypeSymlink, link: "../outside"}, {name: "hard", kind: tar.TypeLink, link: "link"}}},
		{name: "hardlink-forward", want: "link", members: []member{{name: "hard", kind: tar.TypeLink, link: "later"}, {name: "later", kind: tar.TypeReg, body: "new"}}},
		{name: "character-device", want: "entry-type", members: []member{{name: "device", kind: tar.TypeChar}}},
		{name: "block-device", want: "entry-type", members: []member{{name: "device", kind: tar.TypeBlock}}},
		{name: "fifo", want: "entry-type", members: []member{{name: "pipe", kind: tar.TypeFifo}}},
		{name: "unknown-type", want: "entry-type", members: []member{{name: "unknown", kind: 'Z'}}},
		{name: "space-budget", want: "expanded-too-large", free: 10, members: []member{{name: "file", kind: tar.TypeReg, body: "12345678901"}}},
		{name: "sum-budget", want: "expanded-too-large", free: 99, members: []member{{name: "one", kind: tar.TypeReg, body: "123456789012345678901234567890123456789012345678901234567890"}, {name: "two", kind: tar.TypeReg, body: "1234567890123456789012345678901234567890"}}},
		{name: "not-gzip", want: "format", raw: []byte("not an archive")},
		{name: "empty-gzip", want: "format", raw: func() []byte {
			var b bytes.Buffer
			g := gzip.NewWriter(&b)
			require.NoError(t, g.Close())
			return b.Bytes()
		}()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			outside := filepath.Join(dir, "outside")
			target := filepath.Join(dir, "target")
			require.NoError(t, os.Mkdir(target, 0700))
			require.NoError(t, os.WriteFile(outside, []byte("outside-original"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(target, "file"), []byte("target-original"), 0600))
			raw := c.raw
			if raw == nil {
				raw = archive(t, c.members...)
			}
			free := c.free
			if free == 0 {
				free = 1 << 20
			}
			_, err := ValidateArchive(bytes.NewReader(raw), free)
			reason(t, err, c.want)
			got, err := os.ReadFile(outside)
			require.NoError(t, err)
			require.Equal(t, "outside-original", string(got))
			got, err = os.ReadFile(filepath.Join(target, "file"))
			require.NoError(t, err)
			require.Equal(t, "target-original", string(got))
		})
	}
}
func TestArchiveNormalizedExtractionRoundTrip(t *testing.T) {
	raw := archive(t,
		member{name: ".", kind: tar.TypeDir}, member{name: "./dir", kind: tar.TypeDir},
		member{name: "dir/white space\nname", kind: tar.TypeReg, body: "payload with whitespace\n"},
		member{name: "hard", kind: tar.TypeLink, link: "./dir/white space\nname"},
		member{name: "sym", kind: tar.TypeSymlink, link: "dir/white space\nname"},
		member{name: "external", kind: tar.TypeSymlink, link: "../outside"},
	)
	result, err := ValidateArchive(bytes.NewReader(raw), 1<<20)
	require.NoError(t, err)
	require.Equal(t, 6, result.Entries)
	var normalized bytes.Buffer
	require.NoError(t, NormalizeArchive(bytes.NewReader(raw), &normalized, 1<<20, result.SHA256))
	parent := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(parent, "outside"), []byte("sentinel"), 0600))
	scratch := filepath.Join(parent, "scratch")
	require.NoError(t, os.Mkdir(scratch, 0700))
	root, err := os.OpenRoot(scratch)
	require.NoError(t, err)
	defer root.Close()
	tr := tar.NewReader(bytes.NewReader(normalized.Bytes()))
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		require.NoError(t, e)
		require.Equal(t, 123, h.Uid)
		require.Equal(t, 456, h.Gid)
		switch h.Typeflag {
		case tar.TypeDir:
			require.NoError(t, root.MkdirAll(h.Name, 0700))
		case tar.TypeReg, tar.TypeRegA:
			require.NoError(t, root.MkdirAll(path.Dir(h.Name), 0700))
			f, e := root.OpenFile(h.Name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
			require.NoError(t, e)
			_, e = io.Copy(f, tr)
			require.NoError(t, e)
			require.NoError(t, f.Close())
		case tar.TypeSymlink:
			require.NoError(t, root.Symlink(h.Linkname, h.Name))
		case tar.TypeLink:
			require.NoError(t, root.Link(h.Linkname, h.Name))
		default:
			t.Fatalf("unsafe normalized type: %d", h.Typeflag)
		}
	}
	for _, name := range []string{"dir/white space\nname", "hard", "sym"} {
		got, e := root.ReadFile(name)
		require.NoError(t, e)
		require.Equal(t, "payload with whitespace\n", string(got))
	}
	got, err := os.ReadFile(filepath.Join(parent, "outside"))
	require.NoError(t, err)
	require.Equal(t, "sentinel", string(got))
	_, err = root.ReadFile("external")
	require.Error(t, err)
}
func TestArchiveTruncationAndHashMismatch(t *testing.T) {
	raw := archive(t, member{name: "file", kind: tar.TypeReg, body: "payload"})
	_, err := ValidateArchive(bytes.NewReader(raw[:len(raw)-4]), 1<<20)
	reason(t, err, "format")
	result, err := ValidateArchive(bytes.NewReader(raw), 1<<20)
	require.NoError(t, err)
	changed := archive(t, member{name: "file", kind: tar.TypeReg, body: "changed"})
	var output bytes.Buffer
	err = NormalizeArchive(bytes.NewReader(changed), &output, 1<<20, result.SHA256)
	reason(t, err, "format")
	require.NotEmpty(t, output.Bytes())
}
func TestArchiveEntryLimit(t *testing.T) {
	var output bytes.Buffer
	gz := gzip.NewWriter(&output)
	tw := tar.NewWriter(gz)
	for n := 0; n <= maxArchiveEntries; n++ {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("%d", n), Typeflag: tar.TypeDir, Mode: 0700}))
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	_, err := ValidateArchive(bytes.NewReader(output.Bytes()), 1<<20)
	reason(t, err, "too-many-entries")
}
