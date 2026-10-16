// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original restore safety tests for issue #32.

package backup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRestoreMarkerCrashPoints(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, CheckRestoreMarker(dir))
	for _, phase := range []string{"applying", "verifying"} {
		require.NoError(t, writeRestoreMarker(dir, restoreMarker{ArchiveSize: 2048, MigrationVersion: 1, Version: 1, RestoreID: "restore-1", DatabasePath: filepath.Join(dir, "hub.db"), Images: []imageManifest{{Name: hubImageName, Size: 512, SHA256: string(bytes.Repeat([]byte("a"), 64))}, {Name: francisImageName, Size: 512, SHA256: string(bytes.Repeat([]byte("b"), 64))}}, BackupID: "backup-1", ActivityID: "activity-1", Phase: phase}))
		require.ErrorIs(t, CheckRestoreMarker(dir), ErrRestoreRecoveryRequired)
		marker, err := readRestoreMarker(dir)
		require.NoError(t, err)
		require.Equal(t, phase, marker.Phase)
	}
	require.NoError(t, clearRestoreMarker(dir))
	require.NoError(t, CheckRestoreMarker(dir))
	require.NoError(t, os.WriteFile(markerPath(dir), []byte("invalid"), 0600))
	require.ErrorIs(t, CheckRestoreMarker(dir), ErrRestoreRecoveryRequired)
	_, err := readRestoreMarker(dir)
	require.ErrorIs(t, err, ErrRestoreRecoveryRequired)
}

func TestMaintenanceBlocksAdmissionAndDrains(t *testing.T) {
	gate := NewMaintenanceGate()
	first, err := gate.Admit()
	require.NoError(t, err)
	second, err := gate.Admit()
	require.NoError(t, err)
	require.NoError(t, gate.Begin("activity-1"))
	require.True(t, gate.Status().Active)
	_, err = gate.Admit()
	require.ErrorIs(t, err, ErrMaintenanceActive)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, gate.Wait(ctx), context.DeadlineExceeded)
	first()
	second()
	require.NoError(t, gate.Wait(t.Context()))
	gate.End()
	require.False(t, gate.Status().Active)
	release, err := gate.Admit()
	require.NoError(t, err)
	release()
}

func TestUploadLimitAndInvalidArchiveRemoveStaging(t *testing.T) {
	dir := t.TempDir()
	service := &Service{config: &config.Config{BackupDir: dir}}
	_, _, err := service.receiveArchiveWithLimit(t.Context(), "backup-1", bytes.NewBufferString("archive-overflow"), 5)
	require.ErrorIs(t, err, errArchiveLimit)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
	_, _, err = service.receiveArchiveWithLimit(t.Context(), "backup-1", bytes.NewBufferString("invalid"), 100)
	require.ErrorIs(t, err, errInvalidArchive)
	entries, err = os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}
