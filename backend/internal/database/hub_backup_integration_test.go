//go:build postgres_integration

// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original fresh and forward migration tests for issue #32.

package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestHubBackupArchiveMigration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, dsn)
	source, err := url.Parse(dsn)
	require.NoError(t, err)
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminSQL.Close()) })

	for _, startVersion := range []int64{0, 7} {
		name := "fresh"
		if startVersion == 7 {
			name = "forward"
		}
		t.Run(name, func(t *testing.T) {
			var random [8]byte
			_, err := rand.Read(random[:])
			require.NoError(t, err)
			dbName := "docklet_backup_" + hex.EncodeToString(random[:])
			require.NoError(t, admin.Exec("CREATE DATABASE "+dbName).Error)
			t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE "+dbName+" WITH (FORCE)").Error) })
			local := *source
			local.Path = "/" + dbName
			conn, err := gorm.Open(postgres.Open(local.String()), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := conn.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			if startVersion == 7 {
				require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), sqlDB, dbProviderPostgres, MigrationOptions{}, 7))
				require.NoError(t, conn.Exec(`INSERT INTO system_backup_runs (id,created_at,status,trigger,destination,size) VALUES ('legacy',NOW(),'succeeded','manual','local',10)`).Error)
				require.NoError(t, conn.Exec(`INSERT INTO system_backup_policies (id,created_at,schedule,retention_count,enabled) VALUES ('policy',NOW(),'0 0 3 * * *',7,true)`).Error)
			}
			require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), sqlDB, dbProviderPostgres, MigrationOptions{}, 8))
			require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), sqlDB, dbProviderPostgres, MigrationOptions{}, 8))
			assertHubArchiveSchema(t, sqlDB)
			_, err = sqlDB.ExecContext(t.Context(), `INSERT INTO system_backup_runs (id,created_at,status,trigger,size,copy_state) VALUES ('oversize',NOW(),'succeeded','manual',2147483649,'none')`)
			require.Error(t, err)
			_, err = sqlDB.ExecContext(t.Context(), `INSERT INTO system_backup_runs (id,created_at,status,trigger,size,copy_state,archive_name) VALUES ('bad-name',NOW(),'succeeded','manual',1,'none','../unsafe.dump')`)
			require.Error(t, err)
			_, err = sqlDB.ExecContext(t.Context(), `INSERT INTO system_backup_runs (id,created_at,status,trigger,size,copy_state) VALUES ('bad-state',NOW(),'succeeded','manual',1,'unknown')`)
			require.Error(t, err)
			if startVersion == 7 {
				var count int
				require.NoError(t, conn.Raw("SELECT COUNT(*) FROM system_backup_runs WHERE id='legacy'").Scan(&count).Error)
				require.Zero(t, count)
				var enabled bool
				require.NoError(t, conn.Raw("SELECT enabled FROM system_backup_policies WHERE id='policy'").Scan(&enabled).Error)
				require.False(t, enabled)
			}
		})
	}
}

func assertHubArchiveSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, column := range []string{"archive_name", "activity_id", "completed_at", "source_server_major", "migration_version", "copy_target_id", "copy_state", "copy_error_code", "error_code"} {
		var exists bool
		require.NoError(t, db.QueryRowContext(t.Context(), `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='system_backup_runs' AND column_name=$1)`, column).Scan(&exists))
		require.True(t, exists, column)
	}
	for _, column := range []string{"destination", "local_snapshot_id", "error"} {
		var exists bool
		require.NoError(t, db.QueryRowContext(t.Context(), `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='system_backup_runs' AND column_name=$1)`, column).Scan(&exists))
		require.False(t, exists, column)
	}
	var targetID, localEnabled bool
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='system_backup_policies' AND column_name='target_id'), EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='system_backup_policies' AND column_name='local_enabled')`).Scan(&targetID, &localEnabled))
	require.True(t, targetID)
	require.False(t, localEnabled)
	var recoveryTable bool
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT to_regclass('system_backup_recovery_config') IS NOT NULL`).Scan(&recoveryTable))
	require.True(t, recoveryTable)
	for _, name := range []string{"idx_system_backup_runs_archive_name", "idx_system_backup_runs_activity_id"} {
		var exists bool
		require.NoError(t, db.QueryRowContext(t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE tablename='system_backup_runs' AND indexname=$1)`, name).Scan(&exists))
		require.True(t, exists, name)
	}
	for _, name := range []string{"system_backup_runs_size_limit", "system_backup_runs_archive_name", "system_backup_runs_server_major", "system_backup_runs_migration_version", "system_backup_runs_copy_state", "system_backup_policies_retention_count"} {
		var exists bool
		require.NoError(t, db.QueryRowContext(t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname=$1)`, name).Scan(&exists))
		require.True(t, exists, name)
	}
}
