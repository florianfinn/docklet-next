// Source: docklet-next/docs/design/hub-backup.md
// Revision: 73e62f9e35cd5ae59d1bb79802831f8204490866
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original regression coverage for snapshot reservation deadlines and teardown.

package backup

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSnapshotDeadlineIncludesHubWriterReservation(t *testing.T) {
	db, cfg := archiveFixture(t)
	service := NewService(Dependencies{DB: db, Config: cfg})
	pool, err := db.SQLDB()
	require.NoError(t, err)
	held, err := pool.Conn(t.Context())
	require.NoError(t, err)
	defer held.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	start := time.Now()
	_, _, err = service.snapshotArchive(ctx, "reservation", "reservation")
	require.ErrorIs(t, err, errArchiveTimeout)
	require.Less(t, time.Since(start), 35*time.Second, "reservation must use the snapshot cap, not the archive deadline")
	require.False(t, service.MaintenanceGate().Status().Active)
	entries, err := os.ReadDir(cfg.BackupDir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestSnapshotReleasesMaintenanceWhenDrainFails(t *testing.T) {
	db, cfg := archiveFixture(t)
	service := NewService(Dependencies{DB: db, Config: cfg})
	leave, err := service.MaintenanceGate().Admit()
	require.NoError(t, err)
	defer leave()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = service.snapshotArchive(ctx, "cancelled", "cancelled")
	require.Error(t, err)
	require.False(t, service.MaintenanceGate().Status().Active)
}
