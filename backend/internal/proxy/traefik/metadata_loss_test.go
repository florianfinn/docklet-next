// Source: docklet-next/issues/663
// Revision: 723c0de3cd289f18b8ddb41e919096e8d3479536
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original inherited ACL consumer regression proof.

package traefik

import (
	"encoding/binary"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
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

func TestM44MarkerInheritedACL(t *testing.T) {
	x := fixture(t)
	setDefaultACL(t, filepath.Join(x.root, "dynamic"))
	in := x.prepare(x.request("edit-route"))
	require.NoError(t, x.e.Mark(t.Context(), in))
	require.Contains(t, x.read(in.MarkerPath), "docklet-check")
	require.NoError(t, x.d.Cleanup(t.Context(), in))
	_, e := os.Stat(filepath.Join(x.root, in.MarkerPath))
	require.ErrorIs(t, e, os.ErrNotExist)
}
