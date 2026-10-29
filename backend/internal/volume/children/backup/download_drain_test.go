// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original download maintenance, rediscovery and confirmed helper cleanup proof.

package backup

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

func TestDownloadMaintenanceWaitsForConfirmedHelperAndOverviewFindsOwner(t *testing.T) {
	f := newRuntimePlan(t)
	db := f.service.deps.DB
	require.NoError(t, db.AutoMigrate(activity.Models()...))
	require.NoError(t, db.AutoMigrate(&backuptarget.Target{}, &policies.VolumeBackupPolicy{}))
	f.service.deps.Activity = activity.NewActivityService(db, nil)
	f.service.deps.Store.Run = func(context.Context, string) (*volume.Backup, error) {
		return &volume.Backup{ID: "row", VolumeName: "data", Format: "rustic", Destination: "local", LocalSnapshotID: strings.Repeat("a", 64)}, nil
	}
	reader, _, err := f.service.DownloadBackup(t.Context(), "row", nil)
	require.NoError(t, err)
	defer reader.Close()
	overview, err := f.service.Overview(t.Context())
	require.NoError(t, err)
	require.Len(t, overview.Volumes, 1)
	require.NotNil(t, overview.Volumes[0].Running)
	require.Equal(t, "download", overview.Volumes[0].Running.Kind)
	require.NotEmpty(t, overview.Volumes[0].Running.ActivityID)
	held := &atomic.Bool{}
	held.Store(true)
	f.cleanupHeld = held
	f.cleanupInspect = make(chan struct{}, 1)
	defer held.Store(false)
	drained := make(chan error, 1)
	go func() { drained <- f.service.deps.Activity.CancelAndDrain(t.Context(), "") }()
	select {
	case <-f.cleanupInspect:
	case <-time.After(time.Second):
		t.Fatal("maintenance did not cancel download and inspect helper")
	}
	select {
	case <-drained:
		t.Fatal("maintenance passed an unconfirmed download helper")
	case <-time.After(60 * time.Millisecond):
	}
	lease, ok, err := f.service.deps.Engine.TryAcquireRun(t.Context(), backup.VolumeAdmissionScope, "data")
	require.NoError(t, err)
	if ok {
		lease.Release(t.Context())
	}
	require.False(t, ok, "durable admission must outlive unconfirmed helper removal")
	unlock, ok := f.service.deps.Locks.TryLock("data")
	if ok {
		unlock()
	}
	require.False(t, ok)
	held.Store(false)
	select {
	case err := <-drained:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("confirmed helper did not release maintenance")
	}
	require.NoError(t, reader.Close())
	unlock, ok = f.service.deps.Locks.TryLock("data")
	require.True(t, ok)
	unlock()
	overview, err = f.service.Overview(t.Context())
	require.NoError(t, err)
	require.Nil(t, overview.Volumes[0].Running)
}
