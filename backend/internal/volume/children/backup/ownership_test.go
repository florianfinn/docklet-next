// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original durable-then-workspace ownership contention proofs.

package backup

import (
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/volume/operationlock"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOperationOwnershipRetainsBothNamesUntilDetachedEffectSettles(t *testing.T) {
	engine := backup.NewEngine(t.Context(), newVolumeAdmissionForTestInternal(t), nil)
	locks := &utils.KeyedMutex{}
	s := NewService(Dependencies{Engine: engine, Locks: locks, AlreadyRunning: errors.New("busy")})
	ctx, owner, err := s.acquireOperation(t.Context(), "source-data", "destination-data")
	require.NoError(t, err)
	for _, name := range []string{"source-data", "destination-data"} {
		lease, acquired, err := engine.TryAcquireRun(t.Context(), backup.VolumeAdmissionScope, name)
		require.NoError(t, err)
		if acquired {
			lease.Release(t.Context())
		}
		require.False(t, acquired, "operation released durable admission before settlement")
		unlock, ok := locks.TryLock(name)
		if ok {
			unlock()
		}
		require.False(t, ok)
		_, _, err = s.acquireOperation(t.Context(), name, "")
		require.ErrorContains(t, err, "volume.backup.busy")
	}
	retained := operationlock.Retain(ctx, "source-data")
	owner.Release(ctx)
	unlock, ok := locks.TryLock("destination-data")
	if ok {
		unlock()
	}
	require.False(t, ok)
	retained()
	for _, name := range []string{"source-data", "destination-data"} {
		unlock, ok := locks.TryLock(name)
		require.True(t, ok)
		unlock()
	}
	_, next, err := s.acquireOperation(t.Context(), "source-data", "destination-data")
	require.NoError(t, err)
	next.Release(ctx)
}
func TestOperationOwnershipRefusesWorkspaceContentionAndReleasesAdmission(t *testing.T) {
	engine := backup.NewEngine(t.Context(), newVolumeAdmissionForTestInternal(t), nil)
	locks := &utils.KeyedMutex{}
	s := NewService(Dependencies{Engine: engine, Locks: locks})
	unlock := locks.Lock("data")
	_, _, err := s.acquireOperation(t.Context(), "data", "")
	require.ErrorContains(t, err, "volume.backup.busy")
	unlock()
	_, owner, err := s.acquireOperation(t.Context(), "data", "")
	require.NoError(t, err)
	owner.Release(t.Context())
}
