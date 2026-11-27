// Source: docklet-next/issues/663
// Revision: 723c0de3cd289f18b8ddb41e919096e8d3479536
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original inherited ACL consumer regression proof.

package caddy

import (
	"encoding/binary"
	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setDefaultACL(t *testing.T, p string) {
	t.Helper()
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, 2)
	for _, v := range [][3]uint32{{1, 7, ^uint32(0)}, {2, 4, 12345}, {4, 5, ^uint32(0)}, {16, 5, ^uint32(0)}, {32, 0, ^uint32(0)}} {
		b = binary.LittleEndian.AppendUint16(b, uint16(v[0]))
		b = binary.LittleEndian.AppendUint16(b, uint16(v[1]))
		b = binary.LittleEndian.AppendUint32(b, v[2])
	}
	require.NoError(t, unix.Setxattr(p, "system.posix_acl_default", b, 0))
}

func TestM44StagingInheritedACL(t *testing.T) {
	x := fixture(t)
	setDefaultACL(t, filepath.Join(x.root, "routes"))
	q := x.request("create-route", baseSite())
	i, e := prepareChecked(t, x.e, q)
	require.NoError(t, e)
	i.StagingPath = "routes/.docklet-staging-example"
	require.NoError(t, validateChecked(t, x.e, i))
	files, e := os.ReadDir(filepath.Join(x.root, i.StagingPath))
	require.NoError(t, e)
	require.NotEmpty(t, files)
	require.NoError(t, x.d.Cleanup(t.Context(), i))
	x.noTransient()
}

func TestM44RootImportInheritedACL(t *testing.T) {
	x := fixture(t)
	setDefaultACL(t, x.root)
	without := strings.TrimSuffix(rootText, "import /etc/caddy/routes/*.caddy\n")
	require.NoError(t, os.Remove(filepath.Join(x.root, "Caddyfile")))
	x.put("Caddyfile", without)
	g, e := x.d.Graph(t.Context(), x.i)
	require.NoError(t, e)
	q := dto.ApplyRequest{ID: "insert-acl", Instance: x.i, Generation: x.a.Generation(), Registration: x.i.Registration, Target: "Caddyfile", Expected: proxy.Snapshot([]byte(without), true).FileState, GraphHashes: g, Change: dto.Change{Kind: "insert-import"}}
	o, e := applyChecked(t, x.a, q)
	require.NoError(t, e)
	require.Equal(t, dto.Verified, o.State)
	require.Equal(t, rootText, x.bytes("Caddyfile"))
	x.noTransient()
}

func TestM44DeleteAgentRollbackKeepsACL(t *testing.T) {
	x := fixture(t)
	dir := filepath.Join(x.root, "routes")
	setDefaultACL(t, dir)
	s := baseSite()
	x.serving()
	created, e := applyChecked(t, x.a, x.request("create-route", s))
	require.NoError(t, e)
	require.Equal(t, dto.Verified, created.State)

	target := filepath.Join(dir, "app.caddy")
	var before, after unix.Stat_t
	require.NoError(t, unix.Stat(target, &before))
	attrs := make([]byte, 256)
	n, e := unix.Getxattr(target, "system.posix_acl_access", attrs)
	require.NoError(t, e)
	attrs = attrs[:n]
	reloads := 0
	previous := x.execFn
	x.execFn = func(args []string) hostio.ExecResult {
		if args[0] == "caddy" && args[1] == "reload" {
			reloads++
			if reloads == 1 {
				return hostio.ExecResult{ExitCode: 1, Stderr: []byte("rejected candidate")}
			}
		}
		return previous(args)
	}

	q := x.request("delete-route", s)
	o, e := applyChecked(t, x.a, q)
	require.NoError(t, e)
	require.Equal(t, dto.Restored, o.State, "%+v", o)
	require.NoError(t, unix.Stat(target, &after))
	require.Equal(t, before.Ino, after.Ino)
	restored := make([]byte, 256)
	n, e = unix.Getxattr(target, "system.posix_acl_access", restored)
	require.NoError(t, e)
	require.Equal(t, attrs, restored[:n])
	x.noTransient()
}
