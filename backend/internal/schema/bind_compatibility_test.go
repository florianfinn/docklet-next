// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion; M17 retains the exact historical release baseline fixture.

package schema

import (
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestReleaseDatabaseStoresBindKeysWithoutMigration(t *testing.T) {
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	defer pool.Close()
	names, err := filepath.Glob("../../resources/migrations/sqlite/*.sql")
	require.NoError(t, err)
	fs := fstest.MapFS{}
	for _, name := range names {
		if filepath.Base(name) > "001_baseline.sql" {
			continue
		}
		body, err := os.ReadFile(name)
		require.NoError(t, err)
		fs[filepath.Base(name)] = &fstest.MapFile{Data: body}
	}
	require.Len(t, fs, 1)
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, fs)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 1)
	require.NoError(t, err)
	require.NoError(t, db.Exec("INSERT INTO volume_backup_policies(id,volume_name,schedule,retention_count) VALUES ('baseline','data','0 0 2 * * *',11)").Error)
	key := volume.BindKey("/srv/app")
	require.NoError(t, db.Exec("INSERT INTO volume_backup_policies(id,volume_name,schedule) VALUES ('bind-plan',?,'0 0 2 * * *')", key).Error)
	require.NoError(t, db.Exec("INSERT INTO volume_backups(id,volume_name,created_at,size,status,format,local_snapshot_id) VALUES ('bind-backup',?,'2026-11-01',24,'succeeded','rustic','snapshot')", key).Error)
	require.NoError(t, db.Exec("INSERT INTO volume_restore_runs(id,volume_name,kind,phase,status,started_at) VALUES ('bind-restore',?,'full','preparing','running','2026-11-01')", key).Error)
	var retention int
	require.NoError(t, db.Raw("SELECT retention_count FROM volume_backup_policies WHERE id='baseline'").Scan(&retention).Error)
	require.Equal(t, 11, retention)
	for _, table := range []string{"volume_backup_policies", "volume_backups", "volume_restore_runs"} {
		var got string
		require.NoError(t, db.Table(table).Select("volume_name").Where("volume_name = ?", key).Scan(&got).Error)
		require.Equal(t, key, got)
	}
	version, err := provider.GetDBVersion(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, version)
}
