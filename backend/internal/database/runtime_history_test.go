// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original real-goose M16 upgrade coverage for history and preferences.

package database

import (
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestHB10PF7HistoryUpgradePreservesDataAndConstraints(t *testing.T) {
	db, err := connectDatabaseInternal(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "upgrade.db")))
	require.NoError(t, err)
	defer db.Close()
	pool, err := db.SQLDB()
	require.NoError(t, err)
	require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), pool, dbProviderSQLite, MigrationOptions{}, 15))
	require.NoError(t, db.Exec(`INSERT INTO users(id,username,password_hash,time_format,preferences) VALUES('operator','operator','retained','auto','{"iconCatalog":"example"}')`).Error)
	for _, status := range []string{"pending", "sending", "sent", "failed", "unconfigured", "expired"} {
		var lease, settled any
		if status == "sending" {
			lease = "2026-11-03 12:00:00"
		}
		if status != "pending" && status != "sending" {
			settled = "2026-11-03 11:00:00"
		}
		require.NoError(t, db.Exec("INSERT INTO notification_outbox(id,event,environment_id,dedup_key,payload,status,attempts,lease_until,created_at,settled_at) VALUES(?,'host_offline','host',?,'{}',?,2,?,'2026-11-03 10:00:00',?)", status, "key-"+status, status, lease, settled).Error)
	}
	require.NoError(t, db.Migrate(t.Context()))
	var preferences, legacy, name string
	require.NoError(t, db.Raw("SELECT ui_preferences,preferences,username FROM users WHERE id='operator'").Row().Scan(&preferences, &legacy, &name))
	require.Equal(t, "{}", preferences)
	require.Equal(t, `{"iconCatalog":"example"}`, legacy)
	require.Equal(t, "operator", name)
	var count int64
	require.NoError(t, db.Table("notification_outbox").Count(&count).Error)
	require.EqualValues(t, 6, count)
	for _, status := range []string{"pending", "sending", "sent", "failed", "unconfigured", "expired"} {
		var attempts int
		require.NoError(t, db.Raw("SELECT attempts FROM notification_outbox WHERE id=? AND dedup_key=? AND status=?", status, "key-"+status, status).Scan(&attempts).Error)
		require.Equal(t, 2, attempts)
	}
	require.NoError(t, db.Exec("INSERT INTO notification_outbox(id,event,environment_id,dedup_key,payload,status,created_at) VALUES('load','sustained_load','host','new-key','{}','pending',CURRENT_TIMESTAMP)").Error)
	require.Error(t, db.Exec("INSERT INTO notification_outbox(id,event,environment_id,dedup_key,payload,status,created_at) VALUES('invalid','invented','host','invalid-key','{}','pending',CURRENT_TIMESTAMP)").Error)
	require.Error(t, db.Exec("INSERT INTO notification_outbox(id,event,environment_id,dedup_key,payload,status,created_at) VALUES('duplicate','sustained_load','host','key-pending','{}','pending',CURRENT_TIMESTAMP)").Error)
	for _, index := range []string{"idx_notification_outbox_dedup_key", "idx_notification_outbox_delivery"} {
		require.NoError(t, db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", index).Scan(&count).Error)
		require.EqualValues(t, 1, count)
	}
	require.NoError(t, db.Exec("INSERT INTO runtime_stop_intents(container_id,container_name,project_id,recorded_at) VALUES('container','example','project',CURRENT_TIMESTAMP)").Error)
	require.Error(t, db.Exec("UPDATE users SET ui_preferences='invalid' WHERE id='operator'").Error)
	var sql string
	require.NoError(t, db.Raw("SELECT sql FROM sqlite_master WHERE name='history_buckets'").Scan(&sql).Error)
	require.Contains(t, sql, "WITHOUT ROWID")
}
