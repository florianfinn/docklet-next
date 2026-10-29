// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package schema

import (
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestNotifications013UpgradeBothRoles(t *testing.T) {
	for _, role := range []string{"hub", "agent"} {
		t.Run(role, func(t *testing.T) {
			db, e := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
			require.NoError(t, e)
			pool, e := db.DB()
			require.NoError(t, e)
			pool.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, pool.Close()) })
			names, e := filepath.Glob("../../resources/migrations/sqlite/*.sql")
			require.NoError(t, e)
			fs := fstest.MapFS{}
			for _, name := range names {
				b, e := os.ReadFile(name)
				require.NoError(t, e)
				fs[filepath.Base(name)] = &fstest.MapFile{Data: b}
			}
			provider, e := goose.NewProvider(goose.DialectSQLite3, pool, fs)
			require.NoError(t, e)
			_, e = provider.UpTo(t.Context(), 12)
			require.NoError(t, e)
			require.NoError(t, db.Exec(`INSERT INTO notification_settings(provider,enabled,config) VALUES('ntfy',1,'{"host":"notify.example.com","topic":"system"}')`).Error)
			_, e = provider.Up(t.Context())
			require.NoError(t, e)
			var n int64
			require.NoError(t, db.Table("notification_settings").Where("provider='ntfy'").Count(&n).Error)
			require.EqualValues(t, 1, n)
			require.NoError(t, db.Exec(`INSERT INTO notification_outbox(id,event,environment_id,dedup_key,payload,status,attempts,created_at) VALUES('a','backup_failed','0','a','{}','pending',0,'2026-10-29')`).Error)
			for _, update := range []string{"event='unknown'", "status='unknown'", "status='sending'", "attempts=-1", "payload='bad'", "lease_until='2026-10-29'"} {
				require.Error(t, db.Exec("UPDATE notification_outbox SET "+update).Error, update)
			}
			require.NoError(t, db.Exec(`INSERT INTO auto_heal_restarts(run_id,container_id,container_name,stack,started_at,baseline_started_at,baseline_restart_count,outcome) VALUES('run','container','web','app','2026-10-29','before',0,'attempting')`).Error)
			require.Error(t, db.Exec(`UPDATE auto_heal_restarts SET outcome='unknown',settled_at='2026-10-29'`).Error)
			require.Error(t, db.Exec(`UPDATE auto_heal_restarts SET outcome='restarted'`).Error)
			require.NoError(t, db.Exec(`UPDATE auto_heal_restarts SET outcome='interrupted',settled_at='2026-10-29'`).Error)
			require.Error(t, db.Exec(`INSERT INTO auto_heal_restarts(run_id,container_id,container_name,stack,started_at,baseline_started_at,baseline_restart_count,outcome) VALUES('run','container','web','app','2026-10-29','before',0,'attempting')`).Error)
		})
	}
}
