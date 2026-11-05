// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original real goose 017 upgrade, identity, gate and manual-scan proofs.

package database

import (
	"fmt"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestUpdatePolicyForwardMigrationFrom017(t *testing.T) {
	for _, poll := range []string{"default", "true", "false"} {
		for _, auto := range []string{"default", "true", "false"} {
			t.Run(poll+"/"+auto, func(t *testing.T) {
				url := sqlitestore.URL(filepath.Join(t.TempDir(), "upgrade.db"))
				db, e := connectDatabaseInternal(t.Context(), url)
				require.NoError(t, e)
				pool, e := db.SQLDB()
				require.NoError(t, e)
				require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), pool, dbProviderSQLite, MigrationOptions{}, 17))
				for key, value := range map[string]string{"pollingEnabled": poll, "autoUpdate": auto} {
					if value != "default" {
						require.NoError(t, db.Exec("INSERT INTO settings(key,value) VALUES(?,?)", key, value).Error)
					}
				}
				type expectation struct {
					id, mode, kind, clock string
					day                   *int
				}
				expected := []expectation{}
				notify := false
				for _, mode := range []string{"off", "notify", "apply"} {
					for _, gate := range []string{"active", "disabled", "owner-empty", "auto-disabled", "poll-disabled", "missing-schedule"} {
						for index, cron := range []string{"0 30 2 * * *", "0 15 3 * * 2", "0 */5 * * * *", "0 0 25 * * *"} {
							id := fmt.Sprintf("%s-%s-%d", mode, gate, index)
							enabled := gate != "disabled"
							actor := "operator"
							if gate == "owner-empty" {
								actor = ""
							}
							require.NoError(t, db.Exec("INSERT INTO environments(id,name,api_url,enabled) VALUES(?,?,?,?)", id, id, "https://agent.example.com", enabled).Error)
							if gate != "missing-schedule" {
								require.NoError(t, db.Exec("INSERT INTO image_update_schedules(id,environment_id,registration_id,revision,polling_interval,auto_update_interval,polling_enabled,auto_update_enabled,actor_id,actor_key_id) VALUES(?,?,?,1,?,?,?,?,?,?)", id, id, id, "0 0 * * * *", cron, gate != "poll-disabled", gate != "auto-disabled", actor, "").Error)
							}
							require.NoError(t, db.Exec("INSERT INTO stack_update_policies(id,environment_id,registration_id,target_id,target_identity,mode,revision,actor_id,actor_key_id,code,apply_code,bookkeeping_code,candidates) VALUES(?,?,?,?,?,?,7,?,?, '', '', '', '[]')", id, id, id, "stack", "retained-identity", mode, "operator", "").Error)
							want := expectation{id: id, mode: "check"}
							if mode == "apply" && poll != "false" && auto == "true" && enabled && actor != "" && gate != "auto-disabled" && gate != "missing-schedule" {
								want.mode = "scheduled"
								want.kind = "weekly"
								want.clock = "04:00"
								want.day = new(0)
								if index == 0 {
									want.kind = "daily"
									want.clock = "02:30"
									want.day = nil
								}
								if index == 1 {
									want.clock = "03:15"
									want.day = new(2)
								}
							}
							if mode != "off" && poll != "false" && enabled && actor != "" && gate != "poll-disabled" && gate != "missing-schedule" {
								notify = true
							}
							expected = append(expected, want)
						}
					}
				}
				require.NoError(t, db.Exec("INSERT INTO runtime_receipts(run_id,kind,store_generation,manager_id,registration_id,environment_id,executor_generation,request_digest,caller_id,operation_id,target_kind,target_id,gate_scope,gate_key,admitted_at,dispatch_state,executor_state,cancel_requested,release_acknowledged,final_write_at,result) VALUES('retained-runtime','receipt','generation','manager','registration','0','generation','digest','operator','update-project','stack','stack','stack:stack','update',CURRENT_TIMESTAMP,'pending','pending',0,0,CURRENT_TIMESTAMP,'retained-result')").Error)
				require.NoError(t, db.Exec("INSERT INTO runtime_key_bindings(environment_id,idempotency_key,run_id,caller_id,request_digest,first_seen_at) VALUES('0','retained-key','retained-runtime','operator','digest',CURRENT_TIMESTAMP)").Error)
				// Dependent identities must survive the rebuild byte for byte.
				for i, status := range []string{"prepared", "admitted", "refused", "admitted"} {
					code := ""
					if i == 3 {
						code = "runtime.outcome-unknown"
					}
					require.NoError(t, db.Exec("INSERT INTO image_policy_children(id,job_run_id,policy_id,revision,status,code,runtime_id) VALUES(?,?,?,?,?,?,?)", fmt.Sprint(i), fmt.Sprint(i), expected[0].id, 7, status, code, "retained-runtime").Error)
				}
				for _, status := range []string{"pending", "sending"} {
					require.NoError(t, db.Exec("INSERT INTO image_update_candidates(id,policy_id,created_at) VALUES(?,?,CURRENT_TIMESTAMP)", status, expected[0].id).Error)
					require.NoError(t, db.Exec("INSERT INTO image_update_notifications(id,candidate_id,environment_id,payload,status,delivered,created_at) VALUES(?,?,?,'{}',?,0,CURRENT_TIMESTAMP)", status, status, expected[0].id, status).Error)
				}
				require.NoError(t, db.Close())
				db, e = Initialize(t.Context(), url, MigrationOptions{})
				require.NoError(t, e)
				require.NoError(t, db.Close())
				db, e = Initialize(t.Context(), url, MigrationOptions{})
				require.NoError(t, e)
				defer db.Close()
				for _, want := range expected {
					var got struct {
						ID, Mode, TargetKind, TargetIdentity string
						WindowKind, WindowTime               *string
						WindowDay                            *int
						Revision                             int
					}
					require.NoError(t, db.Table("update_policies").Where("id=?", want.id).Take(&got).Error)
					require.Equal(t, want.id, got.ID)
					require.Equal(t, "stack", got.TargetKind)
					require.Equal(t, "retained-identity", got.TargetIdentity)
					require.Equal(t, 7, got.Revision)
					require.Equal(t, want.mode, got.Mode)
					if want.mode == "scheduled" {
						require.Equal(t, want.kind, *got.WindowKind)
						require.Equal(t, want.clock, *got.WindowTime)
						require.Equal(t, want.day, got.WindowDay)
					} else {
						require.Nil(t, got.WindowKind)
						require.Nil(t, got.WindowTime)
						require.Nil(t, got.WindowDay)
					}
				}
				var retained string
				require.NoError(t, db.Raw("SELECT result FROM runtime_receipts WHERE run_id='retained-runtime'").Scan(&retained).Error)
				require.Equal(t, "retained-result", retained)
				var value string
				require.NoError(t, db.Raw("SELECT value FROM settings WHERE key='updateNotify'").Scan(&value).Error)
				require.Equal(t, fmt.Sprint(notify), value)
				for _, table := range []string{"stack_update_policies", "image_update_schedules"} {
					require.False(t, db.Migrator().HasTable(table))
				}
				var n int64
				require.NoError(t, db.Table("settings").Where("key IN ?", []string{"autoUpdate", "autoUpdateInterval", "pollingEnabled", "pollingInterval", "vulnerabilityScanEnabled"}).Count(&n).Error)
				require.Zero(t, n)
				require.NoError(t, db.Table("image_policy_children").Where("policy_id=? AND revision=7 AND runtime_id=?", expected[0].id, "retained-runtime").Count(&n).Error)
				require.EqualValues(t, 4, n)
				require.NoError(t, db.Table("image_update_notifications").Where("status IN ?", []string{"pending", "sending"}).Count(&n).Error)
				require.EqualValues(t, 2, n)
				require.NoError(t, db.Raw("PRAGMA integrity_check").Scan(&value).Error)
				require.Equal(t, "ok", value)
				var failures []map[string]any
				require.NoError(t, db.Raw("PRAGMA foreign_key_check").Scan(&failures).Error)
				require.Empty(t, failures)
			})
		}
	}
}

func TestManualScanForwardMigrationFrom017(t *testing.T) {
	for _, enabled := range []string{"true", "TRUE", "1", "false", "0"} {
		for _, schedule := range []string{"", `{"mode":"daily","time":"02:30"}`, `{"mode":"weekly","day":"sun","time":"04:00"}`, `{"mode":"daily","time":"25:00"}`, `broken`} {
			t.Run(enabled+schedule, func(t *testing.T) {
				url := sqlitestore.URL(filepath.Join(t.TempDir(), "scan.db"))
				db, e := connectDatabaseInternal(t.Context(), url)
				require.NoError(t, e)
				pool, e := db.SQLDB()
				require.NoError(t, e)
				require.NoError(t, migrateDatabaseToVersionInternal(t.Context(), pool, dbProviderSQLite, MigrationOptions{}, 17))
				require.NoError(t, db.Exec("INSERT INTO settings(key,value) VALUES('vulnerabilityScanEnabled',?),('vulnerabilityScanSchedule',?)", enabled, schedule).Error)
				require.NoError(t, db.Close())
				db, e = Initialize(t.Context(), url, MigrationOptions{})
				require.NoError(t, e)
				defer db.Close()
				var got string
				require.NoError(t, db.Raw("SELECT value FROM settings WHERE key='vulnerabilityScanSchedule'").Scan(&got).Error)
				if (enabled == "true" || enabled == "TRUE" || enabled == "1") && (schedule == `{"mode":"daily","time":"02:30"}` || schedule == `{"mode":"weekly","day":"sun","time":"04:00"}`) {
					require.JSONEq(t, schedule, got)
				} else {
					require.JSONEq(t, `{"mode":"manual"}`, got)
				}
			})
		}
	}
}
