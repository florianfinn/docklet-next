// Source: docklet-next/issues/663
// Revision: 723c0de3cd289f18b8ddb41e919096e8d3479536
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original resolver ACL regression proofs.

package resolver_test

import (
	"encoding/binary"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/resolver"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func resolverACL() []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, 2)
	for _, v := range [][3]uint32{{1, 7, ^uint32(0)}, {2, 4, 12345}, {4, 5, ^uint32(0)}, {16, 5, ^uint32(0)}, {32, 0, ^uint32(0)}} {
		b = binary.LittleEndian.AppendUint16(b, uint16(v[0]))
		b = binary.LittleEndian.AppendUint16(b, uint16(v[1]))
		b = binary.LittleEndian.AppendUint32(b, v[2])
	}
	return b
}
func TestM44ResolverInheritedACL(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, unix.Setxattr(root, "system.posix_acl_default", resolverACL(), 0))
	p := filepath.Join(root, ".env")
	require.NoError(t, os.WriteFile(p, []byte("OLD=1\n"), 0644))
	old, e := resolver.Read(root, ".env")
	require.NoError(t, e)
	require.NoError(t, resolver.Replace(root, ".env", old.Old, []byte("NEW=1\n"), true, 0600))
	acl := make([]byte, 256)
	n, e := unix.Getxattr(p, "system.posix_acl_access", acl)
	require.NoError(t, e)
	found := false
	for i := 4; i < n; i += 8 {
		if binary.LittleEndian.Uint16(acl[i:]) == 16 {
			found = true
			require.Zero(t, binary.LittleEndian.Uint16(acl[i+2:]))
		}
	}
	require.True(t, found)
	next, e := resolver.Read(root, ".env")
	require.NoError(t, e)
	require.NoError(t, resolver.Replace(root, ".env", next.Old, old.Before, true, old.Mode))
	acl = resolverACL()
	binary.LittleEndian.PutUint32(acl[16:20], 12346)
	require.NoError(t, unix.Setxattr(p, "system.posix_acl_access", acl, 0))
	e = resolver.Replace(root, ".env", old.Old, []byte("NEW=1\n"), true, 0600)
	require.ErrorContains(t, e, "unsupported-metadata")
	var problem *dto.Problem
	require.True(t, errors.As(e, &problem))
	require.Contains(t, problem.Detail, "system.posix_acl_access")
	b, e := os.ReadFile(p)
	require.NoError(t, e)
	require.Equal(t, old.Before, b)
}
func TestM44ResolverRestoreForeignAttribute(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "equal", true: "foreign"}[foreign], func(t *testing.T) {
			e, f, q, o := fixture(t)
			f.failStartup = true
			f.observing = func() {
				if foreign {
					require.NoError(t, unix.Setxattr(filepath.Join(f.target.Directory, "config", "static.yaml"), "user.example", []byte("foreign"), 0))
				}
			}
			result, err := e.Run(t.Context(), q, *o)
			require.NoError(t, err)
			if foreign {
				require.Equal(t, dto.RecoveryRequired, result.State)
				require.Len(t, result.MetadataLoss, 1)
				require.Equal(t, []string{"user.example"}, result.MetadataLoss[0].Attributes)
			} else {
				require.Equal(t, dto.RolledBack, result.State)
			}
		})
	}
}

// M44 row 22 at the receipt: a forward resolver refusal carries the lost attribute names to the operator.
func TestM44ResolverRefusalReceiptListsLoss(t *testing.T) {
	e, f, q, o := fixture(t)
	require.NoError(t, unix.Setxattr(filepath.Join(f.target.Directory, "config", "static.yaml"), "user.example", []byte("foreign"), 0))
	result, err := e.Run(t.Context(), q, *o)
	require.NoError(t, err)
	require.Equal(t, dto.Refused, result.State)
	require.Equal(t, dto.UnsupportedMetadata, result.Reason)
	require.Len(t, result.MetadataLoss, 1)
	require.Equal(t, "config/static.yaml", result.MetadataLoss[0].Path)
	require.Equal(t, []string{"user.example"}, result.MetadataLoss[0].Attributes)
}
