// Source: docklet-next/issues/663
// Revision: 723c0de3cd289f18b8ddb41e919096e8d3479536
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original real-writer restore regression proofs.

package proxy

import (
	"context"
	"encoding/binary"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestM44RuntimeRestoreLoss(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "equal", true: "foreign"}[foreign], func(t *testing.T) {
			x := newFixture(t)
			root := t.TempDir()
			runtimeDefaultACL(t, root)
			i := fileIntent(t, x, root)
			d := fileSeam(t, x, root)
			require.NoError(t, os.WriteFile(filepath.Join(root, i.Request.Target), i.Old.Content, 0644))
			require.NoError(t, unix.Setxattr(filepath.Join(root, i.Request.Target), "user.example", []byte("original"), 0))
			require.NoError(t, d.Stage(t.Context(), i))
			loss, e := (hostio.ProjectWriter{Root: root}).Prepare(t.Context(), i.TempPath, i.Request.Target)
			require.NoError(t, e)
			i.Request.Acknowledgements.MetadataLoss = []dto.MetadataLoss{loss}
			require.NoError(t, d.Publish(t.Context(), i))
			if foreign {
				require.NoError(t, unix.Setxattr(filepath.Join(root, i.Request.Target), "user.foreign", []byte("foreign"), 0))
			}
			x.a.Runtime = d
			o := dto.Operation{ID: i.Request.ID, Generation: x.a.Generation(), Registration: i.Request.Registration, InstanceID: i.Request.Instance.ID, State: dto.Pending, Phase: dto.Restoring}
			x.persist(t, i, o)
			require.NoError(t, x.a.Settle(t.Context()))
			result, err := x.a.Query(t.Context(), dto.Query{ID: i.Request.ID, Generation: o.Generation, Registration: o.Registration, Instance: i.Request.Instance})
			require.NoError(t, err)
			if foreign {
				require.Equal(t, dto.RecoveryRequired, result.State)
				require.Len(t, result.MetadataLoss, 1)
			} else {
				require.Equal(t, dto.Restored, result.State)
			}

			b, e := os.ReadFile(filepath.Join(root, i.Request.Target))
			require.NoError(t, e)
			if foreign {
				require.Equal(t, i.New.Content, b)
			} else {
				require.Equal(t, i.Old.Content, b)
			}
		})
	}
}
func TestM44DeleteRollbackKeepsInode(t *testing.T) {
	x := newFixture(t)
	root := t.TempDir()
	runtimeDefaultACL(t, root)
	i := fileIntent(t, x, root)
	i.New = Snapshot(nil, false)
	d := fileSeam(t, x, root)
	target := filepath.Join(root, i.Request.Target)
	require.NoError(t, os.WriteFile(target, i.Old.Content, 0644))
	require.NoError(t, unix.Setxattr(target, "user.example", []byte("original"), 0))
	var before, after unix.Stat_t
	require.NoError(t, unix.Stat(target, &before))
	// The Docker seam's graph is empty, matching this intent.
	require.NoError(t, d.Publish(t.Context(), i))
	require.NoError(t, d.Restore(t.Context(), i))
	require.NoError(t, unix.Stat(target, &after))
	require.Equal(t, before.Ino, after.Ino)
	b := make([]byte, 32)
	n, e := unix.Getxattr(target, "user.example", b)
	require.NoError(t, e)
	require.Equal(t, "original", string(b[:n]))
}

func TestM44CreatedFileRestoreRemovesAttributes(t *testing.T) {
	x := newFixture(t)
	root := t.TempDir()
	runtimeDefaultACL(t, root)
	i := fileIntent(t, x, root)
	i.Old = Snapshot(nil, false)
	d := fileSeam(t, x, root)
	target := filepath.Join(root, i.Request.Target)
	require.NoError(t, os.WriteFile(target, i.New.Content, 0644))
	require.NoError(t, unix.Setxattr(target, "user.example", []byte("inherited"), 0))
	require.NoError(t, d.Restore(t.Context(), i))
	_, err := os.Stat(target)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func runtimeDefaultACL(t *testing.T, root string) {
	t.Helper()
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, 2)
	for _, v := range [][3]uint32{{1, 7, ^uint32(0)}, {2, 4, 12345}, {4, 5, ^uint32(0)}, {16, 5, ^uint32(0)}, {32, 0, ^uint32(0)}} {
		b = binary.LittleEndian.AppendUint16(b, uint16(v[0]))
		b = binary.LittleEndian.AppendUint16(b, uint16(v[1]))
		b = binary.LittleEndian.AppendUint32(b, v[2])
	}
	require.NoError(t, unix.Setxattr(root, "system.posix_acl_default", b, 0))
}
func TestM44DeleteRollbackFallbackReceipt(t *testing.T) {
	x := newFixture(t)
	root := t.TempDir()
	i := fileIntent(t, x, root)
	i.New = Snapshot(nil, false)
	d := fileSeam(t, x, root)
	x.a.Runtime = d
	o := dto.Operation{ID: i.Request.ID, Generation: x.a.Generation(), Registration: i.Request.Registration, InstanceID: i.Request.Instance.ID, State: dto.Pending, Phase: dto.Restoring}
	x.persist(t, i, o)
	result, e := x.a.Query(t.Context(), dto.Query{ID: o.ID, Generation: o.Generation, Registration: o.Registration, Instance: i.Request.Instance})
	require.NoError(t, e)
	require.Equal(t, dto.Restored, result.State)
	require.Len(t, result.MetadataLoss, 1)
	require.Contains(t, result.Diagnostic.Detail, "original inode unavailable")
	b, e := os.ReadFile(filepath.Join(root, i.Request.Target))
	require.NoError(t, e)
	require.Equal(t, i.Old.Content, b)
}

func TestMetadataLossFallbackDetailKeepsUTF8(t *testing.T) {
	x := newFixture(t)
	root := t.TempDir()
	i := fileIntent(t, x, root)
	i.New = Snapshot(nil, false)
	d := fileSeam(t, x, root)
	prefix := strings.Repeat("x", 4096-2-len("metadata-loss: original inode unavailable; its metadata cannot be recovered")-1)
	o := dto.Operation{Diagnostic: &dto.Diagnostic{Detail: prefix + "é" + strings.Repeat("x", 80)}}
	persisted := false
	ctx := context.WithValue(t.Context(), restoreReportKey{}, restoreReport{operation: &o, persist: func() error {
		persisted = true
		require.True(t, utf8.ValidString(o.Diagnostic.Detail), "fallback diagnostic must be valid UTF-8 before persistence")
		return nil
	}})

	require.NoError(t, d.Restore(ctx, i))
	require.True(t, persisted)
	require.Equal(t, prefix+"; metadata-loss: original inode unavailable; its metadata cannot be recovered", o.Diagnostic.Detail)
	require.Len(t, o.Diagnostic.Detail, 4095)
}

func TestM44FailedFallbackDoesNotClaimCompletedRestore(t *testing.T) {
	x := newFixture(t)
	root := t.TempDir()
	i := fileIntent(t, x, root)
	i.New = Snapshot(nil, false)
	x.a.Runtime = fileSeam(t, x, root)
	require.NoError(t, os.Mkdir(filepath.Join(root, i.TempPath), 0755))
	o := dto.Operation{ID: i.Request.ID, Generation: x.a.Generation(), Registration: i.Request.Registration, InstanceID: i.Request.Instance.ID, State: dto.Pending, Phase: dto.Restoring}
	x.persist(t, i, o)
	result, e := x.a.Query(t.Context(), dto.Query{ID: o.ID, Generation: o.Generation, Registration: o.Registration, Instance: i.Request.Instance})
	require.NoError(t, e)
	require.Equal(t, dto.RecoveryRequired, result.State)
	require.Len(t, result.MetadataLoss, 1)
	require.NotContains(t, result.Diagnostic.Detail, "restored bytes", "failed restore cannot claim a completed fallback")
}

func TestM44FallbackWarningSurvivesRestart(t *testing.T) {
	x := newFixture(t)
	root := t.TempDir()
	i := fileIntent(t, x, root)
	i.New = Snapshot(nil, false)
	x.a.Runtime = fileSeam(t, x, root)
	o := dto.Operation{ID: i.Request.ID, Generation: x.a.Generation(), Registration: i.Request.Registration, InstanceID: i.Request.Instance.ID, State: dto.Pending, Phase: dto.Restoring}
	x.persist(t, i, o)
	x.a.afterEffect = func(phase dto.Phase) error {
		if phase == dto.Restoring {
			return errCrash
		}
		return nil
	}
	_, err := x.a.Query(t.Context(), dto.Query{ID: o.ID, Generation: o.Generation, Registration: o.Registration, Instance: i.Request.Instance})
	require.ErrorIs(t, err, errCrash)
	x.a.afterEffect = nil
	require.NoError(t, x.a.Settle(t.Context()))
	result, err := x.a.Query(t.Context(), dto.Query{ID: o.ID, Generation: o.Generation, Registration: o.Registration, Instance: i.Request.Instance})
	require.NoError(t, err)
	require.Equal(t, dto.Restored, result.State)
	require.Len(t, result.MetadataLoss, 1)
}
