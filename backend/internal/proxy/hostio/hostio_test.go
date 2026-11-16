// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original hostile-input, epoch and writer proofs.

package hostio

import (
	"archive/tar"
	"bytes"
	"context"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func archive(t *testing.T, headers []*tar.Header, payloads []string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for n, h := range headers {
		require.NoError(t, w.WriteHeader(h))
		if n < len(payloads) {
			_, err := w.Write([]byte(payloads[n]))
			require.NoError(t, err)
		}
	}
	require.NoError(t, w.Close())
	return b.Bytes()
}
func TestP14SecretHeadersNeverDecoded(t *testing.T) {
	b := archive(t, []*tar.Header{{Name: "routes/public.yml", Typeflag: tar.TypeReg, Size: 2}, {Name: "routes/nested.SECRET.dir/hidden.yml", Typeflag: tar.TypeReg, Size: 6}, {Name: "routes/key.secret.yml", Typeflag: tar.TypeSymlink, Linkname: "public.yml"}}, []string{"ok", "hidden", ""})
	files, names, err := ReadTar(t.Context(), bytes.NewReader(b), "/etc/routes", false)
	require.NoError(t, err)
	require.Len(t, files, 1, "only public payloads may be decoded")
	require.Equal(t, "ok", files[0].Content)
	require.Equal(t, []dto.OpaqueFile{{Path: "nested.SECRET.dir/hidden.yml"}, {Path: "key.secret.yml"}}, names)
}
func TestP18TarBounds(t *testing.T) {
	for _, tt := range []struct {
		name    string
		headers []*tar.Header
		payload []string
		single  bool
	}{
		{"traversal", []*tar.Header{{Name: "routes/../escape", Typeflag: tar.TypeReg}}, nil, false},
		{"absolute", []*tar.Header{{Name: "/routes/file", Typeflag: tar.TypeReg}}, nil, false},
		{"symlink", []*tar.Header{{Name: "routes/file", Typeflag: tar.TypeSymlink, Linkname: "other"}}, nil, false},
		{"hardlink", []*tar.Header{{Name: "routes/file", Typeflag: tar.TypeLink, Linkname: "other"}}, nil, false},
		{"oversized", []*tar.Header{{Name: "routes/file", Typeflag: tar.TypeReg, Size: MaxFileBytes + 1}}, []string{strings.Repeat("x", MaxFileBytes+1)}, false},
		{"extra-record", []*tar.Header{{Name: "file", Typeflag: tar.TypeReg}, {Name: "file", Typeflag: tar.TypeReg}}, nil, true},
		{"wrong-name", []*tar.Header{{Name: "different", Typeflag: tar.TypeReg}}, nil, true},
		{"depth", []*tar.Header{{Name: "routes/a/b/c/d/e/f/g/h/file", Typeflag: tar.TypeReg}}, nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := "routes"
			if tt.single {
				root = "file"
			}
			b := archive(t, tt.headers, tt.payload)
			_, _, err := ReadTar(t.Context(), bytes.NewReader(b), root, tt.single)
			require.Error(t, err)
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := ReadTar(ctx, strings.NewReader(""), "routes", false)
	require.ErrorIs(t, err, context.Canceled)
	_, err = LimitOutput(strings.NewReader(strings.Repeat("x", MaxExecBytes+1)))
	require.ErrorContains(t, err, "input-limit")
	headers := make([]*tar.Header, MaxFiles+1)
	for n := range headers {
		headers[n] = &tar.Header{Name: "routes/" + strings.Repeat("a", n+1), Typeflag: tar.TypeReg}
	}
	_, _, err = ReadTar(t.Context(), bytes.NewReader(archive(t, headers, nil)), "routes", false)
	require.ErrorContains(t, err, "input-limit")
}
func TestP16ArgumentAllowlistAndExit(t *testing.T) {
	args, budget, err := Args(dto.Caddy, "validate", "/etc/caddy/staged/Caddyfile", "", "")
	require.NoError(t, err)
	require.Equal(t, ExecTimeout, budget)
	require.Equal(t, []string{"caddy", "validate", "--config", "/etc/caddy/staged/Caddyfile", "--adapter", "caddyfile"}, args)
	for _, v := range []string{"-bad", "bad\nheader", "bad\rheader", "bad\x00header"} {
		_, _, err := Args(dto.Traefik, "probe", "", v, "http://127.0.0.1:8080/")
		require.ErrorContains(t, err, "invalid-input")
	}
	_, _, err = Args(dto.Traefik, "reload", "/etc/proxy/route.yml", "", "")
	require.Error(t, err)
	captured := []string{}
	_, err = Exec(t.Context(), func(_ context.Context, a []string, _ int) (ExecResult, error) {
		captured = a
		return ExecResult{ExitCode: 1}, nil
	}, dto.Caddy, "validate", "/etc/caddy/staged/Caddyfile", "", "")
	require.ErrorContains(t, err, "exec-failed")
	require.Equal(t, args, captured)
	_, err = Exec(t.Context(), func(context.Context, []string, int) (ExecResult, error) {
		return ExecResult{Stdout: make([]byte, MaxExecBytes+1)}, nil
	}, dto.Caddy, "version", "", "", "")
	require.ErrorContains(t, err, "input-limit")
	argv, _, err := Args(dto.Traefik, "probe", "", "example.com", "http://127.0.0.1:8080/path")
	require.NoError(t, err)
	require.Equal(t, []string{"wget", "-Y", "off", "-S", "-T", "10", "-O", "-", "--header", "Host: example.com", "http://127.0.0.1:8080/path"}, argv)
}
func TestP8LogBatchAndSettlementFence(t *testing.T) {
	epoch := Epoch{ContainerID: "example-container", StartedAt: "epoch-one", Running: true}
	f := LogFence{Expected: epoch, Inspect: func(context.Context) (Epoch, error) { return epoch, nil }}
	for _, kind := range []string{"restart", "recreation"} {
		t.Run(kind, func(t *testing.T) {
			epoch = f.Expected
			payload, err := f.Batch(t.Context(), func(context.Context) ([]byte, error) {
				if kind == "restart" {
					epoch.StartedAt = "epoch-two"
				} else {
					epoch.ContainerID = "other-container"
				}
				return []byte("old logs"), nil
			})
			require.ErrorContains(t, err, "container-changed")
			require.Nil(t, payload)
			require.ErrorContains(t, f.Settle(t.Context()), "container-changed")
		})
	}
}
func TestP17ProjectWriterMetadataAndConfinement(t *testing.T) {
	root := t.TempDir()
	w := ProjectWriter{Root: root}
	require.NoError(t, os.WriteFile(filepath.Join(root, "target.yml"), []byte("old"), 0640))
	require.NoError(t, w.Write(t.Context(), ".docklet-one.tmp", []byte("new"), 0640))
	require.NoError(t, w.Replace(t.Context(), ".docklet-one.tmp", "target.yml"))
	got, err := os.ReadFile(filepath.Join(root, "target.yml"))
	require.NoError(t, err)
	require.Equal(t, "new", string(got))
	st, err := os.Stat(filepath.Join(root, "target.yml"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0640), st.Mode().Perm())
	require.NoError(t, os.Symlink(root, filepath.Join(root, "link")))
	require.Error(t, w.Write(t.Context(), "link/escape", []byte("x"), 0644))
	require.Error(t, w.Write(t.Context(), "../escape", nil, 0644))
	for _, mode := range []string{"owner-only", "hard-link", "xattr"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			writer := ProjectWriter{Root: dir}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "target"), []byte("operator"), 0644))
			switch mode {
			case "owner-only":
				require.NoError(t, os.Chmod(filepath.Join(dir, "target"), 0600))
			case "hard-link":
				require.NoError(t, os.Link(filepath.Join(dir, "target"), filepath.Join(dir, "second")))
			case "xattr":
				require.NoError(t, unix.Setxattr(filepath.Join(dir, "target"), "user.example", []byte("x"), 0))
			}
			require.NoError(t, writer.Write(t.Context(), "temp", []byte("new"), 0644))
			require.ErrorContains(t, writer.Replace(t.Context(), "temp", "target"), "unsupported-metadata")
			b, err := os.ReadFile(filepath.Join(dir, "target"))
			require.NoError(t, err)
			require.Equal(t, "operator", string(b))
		})
	}
}

// N7: an oversized secret payload is skipped, not counted against the read limits.
func TestTarLimitsCountOnlyReadPayloads(t *testing.T) {
	big := strings.Repeat("x", int(MaxFileBytes)+1)
	b := archive(t, []*tar.Header{{Name: "routes/public.yml", Typeflag: tar.TypeReg, Size: 2}, {Name: "routes/large.secret.yml", Typeflag: tar.TypeReg, Size: int64(len(big))}}, []string{"ok", big})
	files, names, err := ReadTar(t.Context(), bytes.NewReader(b), "/etc/routes", false)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, []dto.OpaqueFile{{Path: "large.secret.yml"}}, names)
	b = archive(t, []*tar.Header{{Name: "routes/large.yml", Typeflag: tar.TypeReg, Size: int64(len(big))}}, []string{big})
	_, _, err = ReadTar(t.Context(), bytes.NewReader(b), "/etc/routes", false)
	require.Error(t, err, "a readable oversized payload stays refused")
}
