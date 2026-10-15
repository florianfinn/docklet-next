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
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRestoreMarkerCrashPoints(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, CheckRestoreMarker(dir))
	for _, phase := range []string{"applying", "verifying"} {
		require.NoError(t, writeRestoreMarker(dir, restoreMarker{BackupID: "backup-1", ActivityID: "activity-1", Phase: phase}))
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
	_, _, err := service.receiveArchiveWithLimit(t.Context(), "backup-1", bytes.NewBufferString("PGDMP-overflow"), 5)
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

func TestArchiveValidationRejectsMajorAndMigration(t *testing.T) {
	fake := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  --list) printf ';     Dumped from database version: %s\n' "$FAKE_MAJOR" ;;
  --data-only) printf 'COPY public.goose_db_version (id, version_id, is_applied, tstamp) FROM stdin;\n1\t%s\tt\t2026-01-01\n\\.\n' "$FAKE_MIGRATION" ;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(fake, "pg_restore"), []byte(script), 0700))
	t.Setenv("PATH", fake+string(os.PathListSeparator)+os.Getenv("PATH"))
	archive := filepath.Join(t.TempDir(), "backup.dump")
	require.NoError(t, os.WriteFile(archive, []byte("PGDMPsynthetic"), 0600))
	t.Setenv("FAKE_MAJOR", "17.5")
	t.Setenv("FAKE_MIGRATION", "8")
	_, err := validateArchive(t.Context(), archive, 8)
	require.ErrorIs(t, err, errArchiveMajor)
	t.Setenv("FAKE_MAJOR", "18.6")
	t.Setenv("FAKE_MIGRATION", "9")
	_, err = validateArchive(t.Context(), archive, 8)
	require.ErrorIs(t, err, errMigration)
	t.Setenv("FAKE_MIGRATION", "8")
	info, err := validateArchive(t.Context(), archive, 8)
	require.NoError(t, err)
	require.Equal(t, int64(8), info.migration)
}

func TestRestoreCommandErrorIsSanitized(t *testing.T) {
	fake := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(fake, "pg_restore"), []byte("#!/bin/sh\necho 'private-password-example' >&2\nexit 1\n"), 0700))
	t.Setenv("PATH", fake+string(os.PathListSeparator)+os.Getenv("PATH"))
	err := runPGRestore(t.Context(), "postgres://example:test-only@db.example.com/example", "/not-a-real-file")
	require.Error(t, err)
	require.EqualError(t, err, "backup.restore_failed")
	require.False(t, strings.Contains(err.Error(), "test-only"))
	require.False(t, strings.Contains(err.Error(), "private-password-example"))
}
