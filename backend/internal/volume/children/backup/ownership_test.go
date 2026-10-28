// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original durable-then-workspace ownership contention proofs.

package backup

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/volume/operationlock"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

func TestSafetyCaptureUsesExistingOwnershipWithoutNestedAdmission(t *testing.T) {
	f := newRuntimePlan(t)
	db := f.service.deps.DB
	require.NoError(t, db.AutoMigrate(&event.Event{}))
	f.service.deps.Events = event.NewEventService(db, nil, nil)
	f.service.deps.Store.Create = func(ctx context.Context, row *volume.Backup) error {
		return db.WithContext(ctx).Table("volume_backups").Create(row).Error
	}
	f.stdout = `{"id":"` + strings.Repeat("a", 64) + `","summary":{"total_bytes_processed":0}}`
	owned, owner, err := f.service.acquireOperation(t.Context(), "data", "")
	require.NoError(t, err)
	defer owner.Release(owned)
	id, err := f.service.safetyBackupInternal(owned, "data", user.SystemUser)
	require.NoError(t, err)
	var row volume.Backup
	require.NoError(t, db.Table("volume_backups").First(&row, "id = ?", id).Error)
	require.Equal(t, "local", string(row.Destination))
	require.Equal(t, "safety", row.Trigger)
	unlock, ok := f.service.deps.Locks.TryLock("data")
	if ok {
		unlock()
	}
	require.False(t, ok, "safety completion cannot release outer workspace ownership")
	lease, ok, err := f.service.deps.Engine.TryAcquireRun(t.Context(), backup.VolumeAdmissionScope, "data")
	require.NoError(t, err)
	if ok {
		lease.Release(t.Context())
	}
	require.False(t, ok)
}

func TestBackupTerminalPersistenceRetainsVolumeOwnershipUntilCommit(t *testing.T) {
	f := newRuntimePlan(t)
	owned, owner, err := f.service.acquireOperation(t.Context(), "data", "")
	require.NoError(t, err)
	defer owner.Release(owned)
	var unavailable atomic.Bool
	unavailable.Store(true)
	defer unavailable.Store(false)
	attempted := make(chan struct{}, 1)
	var commits atomic.Int64
	f.service.deps.Store.Save = func(context.Context, *volume.Backup) error {
		if unavailable.Load() {
			select {
			case attempted <- struct{}{}:
			default:
			}
			return errors.New("synthetic terminal persistence unavailable")
		}
		commits.Add(1)
		return nil
	}
	finished := make(chan error, 1)
	go func() {
		defer owner.Release(owned)
		finished <- f.service.completeBackupInternal(owned, &volume.Backup{VolumeName: "data"}, nil)
	}()
	select {
	case <-attempted:
	case <-time.After(time.Second):
		t.Fatal("terminal commit was not attempted")
	}
	require.Never(t, func() bool {
		select {
		case <-finished:
			return true
		default:
			return false
		}
	}, 150*time.Millisecond, 5*time.Millisecond, "backup released ownership before terminal commit")
	unlock, ok := f.service.deps.Locks.TryLock("data")
	if ok {
		unlock()
	}
	require.False(t, ok)
	unavailable.Store(false)
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("backup did not finish after database recovery")
	}
	require.EqualValues(t, 1, commits.Load())
}
