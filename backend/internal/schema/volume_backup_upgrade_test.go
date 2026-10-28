// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original populated 011 hub and agent lifecycle upgrade proof.

package schema

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVolumeBackup012Upgrade(t *testing.T) {
	for _, mode := range []string{"hub", "agent"} {
		t.Run(mode, func(t *testing.T) {
			g, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
			require.NoError(t, err)
			pool, err := g.DB()
			require.NoError(t, err)
			pool.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, pool.Close()) })
			names, err := filepath.Glob("../../resources/migrations/sqlite/*.sql")
			require.NoError(t, err)
			fs := fstest.MapFS{}
			for _, name := range names {
				b, err := os.ReadFile(name)
				require.NoError(t, err)
				fs[filepath.Base(name)] = &fstest.MapFile{Data: b}
			}
			provider, err := goose.NewProvider(goose.DialectSQLite3, pool, fs)
			require.NoError(t, err)
			_, err = provider.UpTo(t.Context(), 11)
			require.NoError(t, err)
			for _, row := range []struct {
				id, volume, created string
				updated             any
			}{
				{"old", "data", "2026-10-01", "2026-10-02"},
				{"new-a", "data", "2026-10-03", nil},
				{"new-z", "data", "2026-10-03", nil},
				{"updated", "other", "2026-10-01", "2026-10-05"},
				{"created", "other", "2026-10-04", nil},
			} {
				require.NoError(t, g.Exec(`INSERT INTO volume_backup_policies(id,volume_name,created_at,updated_at,schedule) VALUES(?,?,?,?,?)`, row.id, row.volume, row.created, row.updated, "0 0 2 * * *").Error)
			}
			require.NoError(t, g.Exec(`INSERT INTO volume_backups(id,volume_name,created_at,size,status,format,local_snapshot_id) VALUES('running','data','2026-10-01',42,'running','rustic','snapshot')`).Error)
			require.NoError(t, g.Exec(`INSERT INTO settings(key,value) VALUES('systemVolumeBackupConfig','{}'),('preserved','value')`).Error)
			require.NoError(t, g.Exec(`INSERT INTO system_backup_recovery_config(id,created_at,encrypted_recovery_key) VALUES('system-recovery','2026-10-01','ciphertext')`).Error)
			_, err = provider.Up(t.Context())
			require.NoError(t, err)
			var ids []string
			require.NoError(t, g.Table("volume_backup_policies").Order("id").Pluck("id", &ids).Error)
			require.Equal(t, []string{"new-z", "updated"}, ids)
			var target string
			require.NoError(t, g.Raw(`SELECT target_id FROM volume_backup_policies WHERE id='new-z'`).Scan(&target).Error)
			require.Equal(t, "local", target)
			require.Error(t, g.Exec(`INSERT INTO volume_backup_policies(id,volume_name,schedule) VALUES('duplicate','data','0 0 2 * * *')`).Error)
			var state struct {
				Status            string
				StopContainers    bool
				ActivityID        *string
				StoppedContainers *string
				ErrorCode         *string
			}
			require.NoError(t, g.Table("volume_backups").Where("id='running'").First(&state).Error)
			require.Equal(t, "running", state.Status)
			require.False(t, state.StopContainers)
			require.Nil(t, state.ActivityID)
			require.Nil(t, state.StoppedContainers)
			require.Nil(t, state.ErrorCode)
			require.Error(t, g.Exec(`UPDATE volume_backups SET stopped_containers='invalid'`).Error)
			require.NoError(t, g.Exec(`INSERT INTO volume_restore_runs(id,volume_name,kind,phase,status,started_at) VALUES('restore','data','full','preparing','running','2026-10-01')`).Error)
			var restore struct {
				Mode                                          string
				PendingRestart, CreatedVolume, CleanupPending bool
			}
			require.NoError(t, g.Table("volume_restore_runs").First(&restore).Error)
			require.Equal(t, "replace", restore.Mode)
			require.False(t, restore.PendingRestart)
			require.False(t, restore.CreatedVolume)
			require.False(t, restore.CleanupPending)
			for _, invalid := range []string{"kind='invalid'", "phase='invalid'", "status='invalid'", "mode='invalid'", "stopped_containers='invalid'"} {
				require.Error(t, g.Exec("UPDATE volume_restore_runs SET "+invalid).Error, invalid)
			}
			var count int64
			require.NoError(t, g.Table("settings").Where("key='systemVolumeBackupConfig'").Count(&count).Error)
			require.Zero(t, count)
			var value string
			require.NoError(t, g.Raw(`SELECT value FROM settings WHERE key='preserved'`).Scan(&value).Error)
			require.Equal(t, "value", value)
			require.NoError(t, g.Raw(`SELECT encrypted_recovery_key FROM system_backup_recovery_config WHERE id='system-recovery'`).Scan(&value).Error)
			require.Equal(t, "ciphertext", value)
			testTarget012Constraints(t, g)
		})
	}
}

func testTarget012Constraints(t *testing.T, g *gorm.DB) {
	t.Helper()
	insert := `INSERT INTO backup_targets(id,name,kind,host_id,path,server,share,username,secret_enc,options,recovery_key_enc,config_version,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`
	for _, row := range []struct {
		id, kind string
		host     any
	}{
		{"local", "local", "0"}, {"mount", "mount", "host"}, {"smb", "smb", nil}, {"nfs", "nfs", nil},
	} {
		require.NoError(t, g.Exec(insert, row.id, row.id, row.kind, row.host, "/target", "", "", "", "encrypted-secret", "", "encrypted-key", 1, "2026-10-01").Error)
	}
	for _, row := range []struct {
		id, kind string
		host     any
	}{
		{"unknown", "other", nil}, {"local-no-host", "local", nil}, {"mount-no-host", "mount", nil}, {"smb-host", "smb", "0"}, {"nfs-host", "nfs", "0"},
	} {
		require.Error(t, g.Exec(insert, row.id, row.id, row.kind, row.host, "/target", "", "", "", "", "", "", 1, "2026-10-01").Error, row.id)
	}
	require.Error(t, g.Exec(insert, "duplicate", "local", "local", "0", "/target", "", "", "", "", "", "", 1, "2026-10-01").Error)
	require.NoError(t, g.Exec(`UPDATE backup_targets SET retired_at='2026-10-02' WHERE id='local'`).Error)
	require.NoError(t, g.Exec(insert, "replacement", "local", "local", "0", "/target", "", "", "", "", "", "", 2, "2026-10-02").Error)
	require.NoError(t, g.Exec(`INSERT INTO backup_target_checks(target_id,host_id,set_version,config_version,activity_id,ok,at) VALUES('smb','0',1,1,'activity',1,'2026-10-01')`).Error)
	require.Error(t, g.Exec(`INSERT INTO backup_target_checks(target_id,host_id,set_version,config_version,activity_id,ok,at) VALUES('smb','0',1,1,'activity',1,'2026-10-01')`).Error)
}
