// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package database

import (
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestHubBackupArchiveMigration(t *testing.T) {
	for _, start := range []int64{0, 1} {
		t.Run(string(rune('0'+start)), func(t *testing.T) {
			url := sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))
			db, err := OpenHubForRestore(t.Context(), url)
			require.NoError(t, err)
			defer db.Close()
			pool, err := db.SQLDB()
			require.NoError(t, err)
			if start == 1 {
				require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), pool, dbProviderSQLite, MigrationOptions{}, 1))
				require.NoError(t, db.Exec("INSERT INTO system_backup_runs(id,status,trigger,destination) VALUES('legacy','succeeded','manual','local')").Error)
				require.NoError(t, db.Exec("INSERT INTO system_backup_policies(id,schedule,retention_count,enabled) VALUES('policy','0 0 3 * * *',7,true)").Error)
			}
			require.NoError(t, db.Migrate(t.Context()))
			require.NoError(t, db.Migrate(t.Context()))
			for _, column := range []string{"archive_name", "activity_id", "completed_at", "archive_version", "migration_version", "copy_target_id", "copy_state", "copy_error_code", "error_code"} {
				require.True(t, db.Migrator().HasColumn("system_backup_runs", column), column)
			}
			for _, column := range []string{"destination", "local_snapshot_id", "error"} {
				require.False(t, db.Migrator().HasColumn("system_backup_runs", column), column)
			}
			require.True(t, db.Migrator().HasTable("system_backup_recovery_config"))
			require.True(t, db.Migrator().HasColumn("system_backup_policies", "target_id"))
			require.False(t, db.Migrator().HasColumn("system_backup_policies", "local_enabled"))
			for _, statement := range []string{
				"INSERT INTO system_backup_runs(id,status,trigger,size,copy_state) VALUES('oversize','succeeded','manual',2147483649,'none')",
				"INSERT INTO system_backup_runs(id,status,trigger,size,copy_state,archive_name) VALUES('bad-name','succeeded','manual',1,'none','../unsafe.tar')",
				"INSERT INTO system_backup_runs(id,status,trigger,size,copy_state) VALUES('bad-state','succeeded','manual',1,'unknown')",
				"INSERT INTO system_backup_runs(id,status,trigger,size,copy_state,archive_version) VALUES('bad-version','succeeded','manual',1,'none',2)",
			} {
				require.Error(t, db.Exec(statement).Error)
			}
			if start == 1 {
				var count int64
				require.NoError(t, db.Table("system_backup_runs").Count(&count).Error)
				require.Zero(t, count)
				var enabled bool
				require.NoError(t, db.Raw("SELECT enabled FROM system_backup_policies WHERE id='policy'").Scan(&enabled).Error)
				require.False(t, enabled)
			}
			rows, err := pool.QueryContext(t.Context(), "PRAGMA foreign_key_check")
			require.NoError(t, err)
			require.False(t, rows.Next())
			require.NoError(t, rows.Close())
		})
	}
}
