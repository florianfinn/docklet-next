// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original per-case proof of the migrated notify setting.

package database

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
)

// Each case upgrades its own database holding one host and one policy, so the
// migrated updateNotify value proves that host's image-polling gate alone.
func TestUpdateNotifyMigrationPerHostGate(t *testing.T) {
	for _, poll := range []string{"default", "true", "false"} {
		for _, mode := range []string{"off", "notify", "apply"} {
			for _, gate := range []string{"active", "host-disabled", "owner-empty", "poll-disabled", "auto-disabled", "missing-schedule"} {
				t.Run(poll+"/"+mode+"/"+gate, func(t *testing.T) {
					url := sqlitestore.URL(filepath.Join(t.TempDir(), "notify.db"))
					db, e := connectDatabaseInternal(t.Context(), url)
					require.NoError(t, e)
					pool, e := db.SQLDB()
					require.NoError(t, e)
					require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), pool, dbProviderSQLite, MigrationOptions{}, 17))
					if poll != "default" {
						require.NoError(t, db.Exec("INSERT INTO settings(key,value) VALUES('pollingEnabled',?)", poll).Error)
					}
					id := "host"
					actor := "operator"
					if gate == "owner-empty" {
						actor = ""
					}
					require.NoError(t, db.Exec("INSERT INTO environments(id,name,api_url,enabled) VALUES(?,?,?,?)", id, id, "https://agent.example.com", gate != "host-disabled").Error)
					if gate != "missing-schedule" {
						require.NoError(t, db.Exec("INSERT INTO image_update_schedules(id,environment_id,registration_id,revision,polling_interval,auto_update_interval,polling_enabled,auto_update_enabled,actor_id,actor_key_id) VALUES(?,?,?,1,'0 0 * * * *','0 30 2 * * *',?,?,?,'')", id, id, id, gate != "poll-disabled", gate != "auto-disabled", actor).Error)
					}
					require.NoError(t, db.Exec("INSERT INTO stack_update_policies(id,environment_id,registration_id,target_id,target_identity,mode,revision,actor_id,actor_key_id,code,apply_code,bookkeeping_code,candidates) VALUES('policy',?,?,'stack','identity',?,1,'operator','','','','','[]')", id, id, mode).Error)
					require.NoError(t, db.Close())
					db, e = Initialize(t.Context(), url, MigrationOptions{})
					require.NoError(t, e)
					defer db.Close()
					// Notifications were really produced only through an effective image-polling gate.
					want := mode != "off" && poll != "false" && gate != "host-disabled" && gate != "owner-empty" && gate != "poll-disabled" && gate != "missing-schedule"
					var value string
					require.NoError(t, db.Raw("SELECT value FROM settings WHERE key='updateNotify'").Scan(&value).Error)
					require.Equal(t, fmt.Sprint(want), value)
				})
			}
		}
	}
}
