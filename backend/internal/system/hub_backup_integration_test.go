//go:build postgres_integration

// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original startup reconciliation and failure-write regression test for issue #32.

package system

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

func TestHubBackupFailureWritesAfterArchiveMigration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, dsn, "DATABASE_URL must point to the test PostgreSQL service")
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminSQL.Close()) })
	var suffix [8]byte
	_, err = rand.Read(suffix[:])
	require.NoError(t, err)
	name := "docklet_backup_fail_" + hex.EncodeToString(suffix[:])
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE "+name+" WITH (FORCE)").Error) })
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	parsed.Path = "/" + name
	db, err := database.Initialize(t.Context(), parsed.String(), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	service := &SystemService{db: db}
	store := service.backupStoreInternal()

	// Startup must succeed even when there are no abandoned rows to update.
	require.NoError(t, service.ReconcileInterruptedBackups(t.Context()))
	for _, id := range []string{"interrupted", "protected", "failed"} {
		require.NoError(t, store.CreateRun(t.Context(), &backuptypes.SystemBackupRun{
			ID: id, CreatedAt: time.Now().UTC(),
			Status: backuptypes.SystemBackupStatusRunning, Trigger: backuptypes.SystemBackupTriggerManual,
		}))
	}
	require.NoError(t, service.ReconcileInterruptedBackups(t.Context(), "protected"))
	require.NoError(t, store.FailRun(t.Context(), "protected", "private failure detail"))
	for _, tc := range []struct{ id, code string }{
		{"interrupted", "backup.interrupted"},
		{"protected", "backup.failed"},
		{"failed", "backup.interrupted"},
	} {
		var run SystemBackupRun
		require.NoError(t, db.First(&run, "id = ?", tc.id).Error)
		require.Equal(t, SystemBackupStatusFailed, run.Status)
		require.Equal(t, tc.code, run.ErrorCode)
	}
}
