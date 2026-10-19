// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant restore repair ownership, atomic storage and startup composition proofs.

package activity

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"modernc.org/sqlite"
)

type restoreReceiptSnapshot struct {
	Receipts    []RuntimeReceipt
	Bindings    []RuntimeKeyBinding
	Generations []RuntimeStoreGeneration
	Activities  []Activity
	Messages    []ActivityMessage
}

func restoreReceiptRead(t *testing.T, db *gorm.DB) restoreReceiptSnapshot {
	t.Helper()
	var s restoreReceiptSnapshot
	require.NoError(t, db.Order("run_id").Find(&s.Receipts).Error)
	require.NoError(t, db.Order("environment_id,idempotency_key").Find(&s.Bindings).Error)
	require.NoError(t, db.Order("id").Find(&s.Generations).Error)
	require.NoError(t, db.Order("id").Find(&s.Activities).Error)
	require.NoError(t, db.Order("activity_id,id").Find(&s.Messages).Error)
	return s
}

func restoreLegacy(t *testing.T, f *b1FixtureInternal, scope string, status activitytypes.Status) string {
	t.Helper()
	id := uuid.NewString()
	key := ""
	if scope != "" {
		key = "example-operation"
	}
	row := Activity{BaseModel: database.BaseModel{ID: id, CreatedAt: f.store.now},
		EnvironmentID: "0", Type: activitytypes.TypeResourceAction, Status: status, StartedAt: f.store.now,
		GateScope: scope, GateKey: key, StartedByUsername: new("Original operator"),
		Metadata: database.JSON{"example": "retained"}, AffectedServices: database.JSON{}}
	require.NoError(t, f.store.svc.db.Create(&row).Error)
	return id
}

func restoreReturnedRun(f *b1FixtureInternal) activitytypes.Activity {
	return activitytypes.Activity{ID: uuid.NewString(), CreatedAt: f.store.now.Add(-time.Minute), StartedAt: f.store.now,
		StartedBy: &activitytypes.StartedBy{UserID: "example-owner", Username: "Original operator"}}
}

func restoreInvoke(ctx context.Context, f *b1FixtureInternal, recreate bool, run activitytypes.Activity) error {
	if recreate {
		return f.store.svc.RestoreGatedActivity(ctx, run, "example-backup")
	}
	return f.store.svc.FailInterruptedAfterRestore(ctx)
}

// Each helper invocation gets a fresh migrated database, including independently
// active legacy controls. The first invocation cannot mask the second's writes.
func TestRestoreReceiptsOwnership(t *testing.T) {
	for _, recreate := range []bool{false, true} {
		t.Run(fmt.Sprintf("recreate=%t", recreate), func(t *testing.T) {
			f := b1Fixture(t)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			var legacy []string
			for i, phase := range []string{"pending", "accepted", "executing", "dispatching", "unknown", "terminal", "disposed", "foreign-generation"} {
				for _, activityStatus := range []activitytypes.Status{activitytypes.StatusQueued, activitytypes.StatusRunning} {
					c := f.command()
					c.Target.ID = "example-" + phase + "-" + string(activityStatus)
					if phase == "dispatching" || phase == "unknown" || phase == "disposed" {
						c.Store.EnvironmentID = "example-remote"
						c.Store.ExecutorGeneration = uuid.NewString()
					}
					if phase == "foreign-generation" {
						c.Store.ExecutorGeneration = uuid.NewString()
					}
					if i%2 == 0 {
						c.Operation = RuntimeStartContainer
						c.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat(fmt.Sprintf("%x", i+1), 63) + map[activitytypes.Status]string{activitytypes.StatusQueued: "a", activitytypes.StatusRunning: "b"}[activityStatus], ParentProjectID: new("example-parent-" + phase + "-" + string(activityStatus))}
					}
					id := b3Admit(t, f, c)
					changes := map[string]any{}
					switch phase {
					case "accepted", "executing":
						changes["executor_state"] = phase
					case "dispatching":
						changes["dispatch_state"] = "dispatching"
						changes["dispatched_at"] = f.store.now
					case "unknown":
						changes["dispatch_state"] = "dispatching"
						changes["dispatched_at"] = f.store.now
						changes["executor_state"] = "unknown"
						changes["unknown_reason"] = "executor-unavailable"
					case "terminal":
						changes["executor_state"] = "success"
						changes["executor_settled_at"] = f.store.now
						changes["settled_at"] = f.store.now
					case "disposed":
						changes["executor_state"] = "unknown"
						changes["unknown_reason"] = "executor-unavailable"
						changes["hub_disposition"] = "released"
						changes["hub_disposition_by"] = "example-admin"
						changes["hub_disposition_reason"] = "Example acknowledged release"
						changes["hub_disposition_at"] = f.store.now
						changes["release_acknowledged"] = true
						changes["settled_at"] = f.store.now
					}
					if i%2 == 1 {
						changes["cancel_requested"] = true
						changes["cancel_requested_at"] = f.store.now
					}
					if len(changes) > 0 {
						require.NoError(t, f.store.svc.db.Model(&RuntimeReceipt{}).Where("run_id = ?", id).Updates(changes).Error)
					}
					scope, key := "", ""
					if i%2 == 1 {
						scope = "foreign:copied-" + phase + "-" + string(activityStatus)
						key = "foreign-key"
					}
					status := activityStatus
					require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", id).UpdateColumns(map[string]any{"gate_scope": scope, "gate_key": key, "status": status, "output": OutputLines{{Text: "Retained example output"}}, "output_truncated": true}).Error)
				}
			}
			for _, scope := range []string{"", "stack:example-no-receipt", "system-backup", "job:example", "coordinator:example", "ordinary:example"} {
				legacy = append(legacy, restoreLegacy(t, f, scope, activitytypes.StatusRunning))
			}
			job := restoreLegacy(t, f, "job:queued", activitytypes.StatusQueued)
			legacy = append(legacy, job)
			require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", job).UpdateColumn("type", activitytypes.TypeJobRun).Error)
			tomb := restoreLegacy(t, f, "stack:example-tombstone", activitytypes.StatusQueued)
			legacy = append(legacy, tomb)
			require.NoError(t, f.store.svc.db.Create(&RuntimeReceipt{RunID: tomb, Kind: "not_received", StoreGeneration: f.store.generation, ExecutorGeneration: f.store.generation, ManagerID: "example-manager", RegistrationID: "example-registration", EnvironmentID: "0", RequestDigest: "example-tombstone", SettledAt: new(f.store.now), FinalWriteAt: f.store.now}).Error)
			wrong := f.command()
			wrong.Store.EnvironmentID = "example-other-environment"
			wrong.Target.ID = "example-wrong-environment"
			wrongID := b3Admit(t, f, wrong)
			legacy = append(legacy, wrongID)
			require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", wrongID).UpdateColumn("environment_id", "0").Error)
			for _, status := range []activitytypes.Status{activitytypes.StatusSuccess, activitytypes.StatusFailed, activitytypes.StatusCancelled} {
				restoreLegacy(t, f, "terminal:"+string(status), status)
			}
			before := restoreReceiptRead(t, f.store.svc.db.DB)
			returned := restoreReturnedRun(f)
			lower := time.Now().UTC().Add(-time.Second)
			require.NoError(t, restoreInvoke(ctx, f, recreate, returned))
			after := restoreReceiptRead(t, f.store.svc.db.DB)
			expected := before
			expected.Activities = append([]Activity(nil), before.Activities...)
			expectedLegacy := map[string]bool{}
			for _, id := range legacy {
				expectedLegacy[id] = true
			}
			for i, row := range expected.Activities {
				if !expectedLegacy[row.ID] {
					continue
				}
				actual := b3Activity(t, f, row.ID)
				require.Equal(t, activitytypes.StatusFailed, actual.Status, "legacy control %s must repair", row.GateScope)
				require.Equal(t, "activity.failed", actual.MessageCode)
				require.Equal(t, new("backup.interrupted"), actual.Error)
				require.NotNil(t, actual.EndedAt)
				require.NotNil(t, actual.UpdatedAt)
				require.NotNil(t, actual.DurationMs)
				require.True(t, !actual.EndedAt.Before(lower) && !actual.EndedAt.After(time.Now().UTC()))
				require.Equal(t, actual.EndedAt, actual.UpdatedAt)
				require.Equal(t, max(int64(0), (actual.EndedAt.UnixMicro()-row.StartedAt.UnixMicro())/1000), *actual.DurationMs)
				row.Status = actual.Status
				row.MessageCode = actual.MessageCode
				row.Error = actual.Error
				row.EndedAt = actual.EndedAt
				row.UpdatedAt = actual.UpdatedAt
				row.DurationMs = actual.DurationMs
				expected.Activities[i] = row
			}
			if recreate {
				restored := b3Activity(t, f, returned.ID)
				require.Equal(t, returned.CreatedAt.UTC().Truncate(time.Microsecond), restored.CreatedAt)
				require.Equal(t, returned.StartedAt.UTC().Truncate(time.Microsecond), restored.StartedAt)
				require.Equal(t, new("example-owner"), restored.StartedByUserID)
				require.Equal(t, new("Original operator"), restored.StartedByUsername)
				require.Equal(t, "0", restored.EnvironmentID)
				require.Equal(t, activitytypes.TypeResourceAction, restored.Type)
				require.Equal(t, new("system_backup"), restored.ResourceType)
				require.Equal(t, new("example-backup"), restored.ResourceID)
				require.Equal(t, "system-backup", restored.GateScope)
				require.Equal(t, "restore:example-backup", restored.GateKey)
				require.Equal(t, activitytypes.StatusRunning, restored.Status)
				require.Equal(t, "run.started", restored.MessageCode)
				// The sole extra row is the returned restore operation, never a receipt.
				filtered := after.Activities[:0]
				for _, a := range after.Activities {
					if a.ID != returned.ID {
						filtered = append(filtered, a)
					}
				}
				after.Activities = filtered
			}
			require.Equal(t, expected, after, "only named legacy repair and restore recreation deltas are allowed")
		})
	}
}

func restoreDeferredFault(t *testing.T, db *gorm.DB, trigger string) {
	t.Helper()
	require.NoError(t, db.Exec("CREATE TABLE fixture_restore_parent(id INTEGER PRIMARY KEY)").Error)
	require.NoError(t, db.Exec("CREATE TABLE fixture_restore_deferred(id INTEGER REFERENCES fixture_restore_parent(id) DEFERRABLE INITIALLY DEFERRED)").Error)
	require.NoError(t, db.Exec(trigger+" BEGIN INSERT INTO fixture_restore_deferred VALUES(1); END").Error)
}

func restoreHeldPeer(t *testing.T, f *b1FixtureInternal, c RuntimeCommand, id string) {
	t.Helper()
	peer := f.contender(t, &b1ResolverInternal{})
	for _, parent := range []bool{false, true} {
		contender := c
		contender.Key = uuid.NewString()
		contender.Operation = RuntimeStopContainer
		if parent {
			contender.Operation = RuntimeDownProject
			contender.Target = RuntimeTarget{Kind: "stack", ID: *c.Target.ParentProjectID}
		}
		result, err := peer.Admit(t.Context(), contender)
		var conflict *GateConflict
		require.ErrorAs(t, err, &conflict)
		require.Equal(t, id, conflict.RunID)
		require.Empty(t, result.ActivityID)
	}
}

func restoreChild(t *testing.T, f *b1FixtureInternal) (RuntimeCommand, string) {
	t.Helper()
	c := f.command()
	c.Operation = RuntimeStartContainer
	c.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64), ParentProjectID: new("example-parent")}
	id := b3Admit(t, f, c)
	require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", id).UpdateColumns(map[string]any{"gate_scope": "", "gate_key": ""}).Error)
	return c, id
}

func TestRestoreReceiptsAtomicFailures(t *testing.T) {
	for _, recreate := range []bool{false, true} {
		for _, fault := range []string{"update", "insert", "collision", "commit", "cancelled", "missing-table", "full"} {
			if !recreate && (fault == "insert" || fault == "collision" || fault == "commit") {
				continue
			}
			t.Run(fmt.Sprintf("recreate=%t/%s", recreate, fault), func(t *testing.T) {
				f := b1Fixture(t)
				c, id := restoreChild(t, f)
				restoreLegacy(t, f, "legacy:example", activitytypes.StatusQueued)
				returned := restoreReturnedRun(f)
				if fault == "collision" {
					returned.ID = restoreLegacy(t, f, "terminal:collision", activitytypes.StatusSuccess)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				db := f.store.svc.db.DB
				switch fault {
				case "update":
					require.NoError(t, db.Exec("CREATE TRIGGER fixture_restore_fault BEFORE UPDATE OF status ON activities BEGIN SELECT RAISE(ABORT,'fixture repair refusal'); END").Error)
				case "insert":
					require.NoError(t, db.Exec("CREATE TRIGGER fixture_restore_fault BEFORE INSERT ON activities BEGIN SELECT RAISE(ABORT,'fixture recreation refusal'); END").Error)
				case "commit":
					restoreDeferredFault(t, db, "CREATE TRIGGER fixture_restore_fault AFTER INSERT ON activities")
				case "cancelled":
					cancel()
				case "missing-table":
					require.NoError(t, db.Exec("ALTER TABLE runtime_receipts RENAME TO fixture_restore_receipts").Error)
				case "full":
					require.NoError(t, db.Exec("CREATE TABLE fixture_restore_growth(payload BLOB NOT NULL)").Error)
					require.NoError(t, db.Exec("CREATE TRIGGER fixture_restore_fault AFTER UPDATE OF status ON activities BEGIN INSERT INTO fixture_restore_growth VALUES(zeroblob(1048576)); END").Error)

				}
				if fault == "missing-table" {
					require.NoError(t, db.Exec("ALTER TABLE fixture_restore_receipts RENAME TO runtime_receipts").Error)
				}
				before := restoreReceiptRead(t, db)
				if fault == "missing-table" {
					require.NoError(t, db.Exec("ALTER TABLE runtime_receipts RENAME TO fixture_restore_receipts").Error)
				}
				var err error
				if fault == "full" {
					require.NoError(t, db.WithContext(ctx).Connection(func(pin *gorm.DB) error {
						var pages int64
						require.NoError(t, pin.Raw("PRAGMA page_count").Scan(&pages).Error)
						require.NoError(t, pin.Exec(fmt.Sprintf("PRAGMA max_page_count = %d", pages+2)).Error)
						defer func() { require.NoError(t, pin.Exec("PRAGMA max_page_count = 1000000").Error) }()
						service := NewActivityService(&database.DB{DB: pin}, nil)
						if recreate {
							err = service.RestoreGatedActivity(ctx, returned, "example-backup")
						} else {
							err = service.FailInterruptedAfterRestore(ctx)
						}
						return nil
					}))
				} else {
					err = restoreInvoke(ctx, f, recreate, returned)
				}
				assert.Error(t, err)
				if fault == "cancelled" {
					assert.ErrorIs(t, err, context.Canceled)
				}
				if fault == "commit" {
					assert.ErrorContains(t, err, "FOREIGN KEY constraint failed")
				}
				if fault == "full" {
					var driver *sqlite.Error
					require.ErrorAs(t, err, &driver)
					require.Equal(t, 13, driver.Code()&0xff)
					require.False(t, dbutil.IsRetryableWriteError(err))
				}
				if fault == "missing-table" {
					require.NoError(t, db.Exec("ALTER TABLE fixture_restore_receipts RENAME TO runtime_receipts").Error)
				}
				assert.Equal(t, before, restoreReceiptRead(t, db), "failed atomic repair must leave all five tables unchanged")
				if fault == "full" {
					require.NoError(t, db.Exec("PRAGMA max_page_count = 1000000").Error)
				}
				if fault == "update" || fault == "insert" || fault == "commit" || fault == "full" {
					require.NoError(t, db.Exec("DROP TRIGGER fixture_restore_fault").Error)
				}
				restoreHeldPeer(t, f, c, id)
				if fault == "collision" {
					returned.ID = uuid.NewString()
				}
				require.NoError(t, restoreInvoke(t.Context(), f, recreate, returned))
				require.Equal(t, before.Receipts, restoreReceiptRead(t, db).Receipts)
				restoreHeldPeer(t, f, c, id)
			})
		}
	}
}

// This explicit ended-owner, same-generation fixture proves writer composition.
// It does not establish real restored process continuity or readiness.
func TestRestoreReceiptsStartupComposition(t *testing.T) {
	for _, fault := range []string{"none", "receipt", "activity", "final", "commit"} {
		t.Run(fault, func(t *testing.T) {
			f := b1Fixture(t)
			c, id := restoreChild(t, f)
			for _, phase := range []string{"remote-unknown", "foreign-generation", "terminal"} {
				other := f.command()
				other.Target.ID = "example-" + phase
				if phase == "remote-unknown" {
					other.Store.EnvironmentID = "example-remote"
					other.Store.ExecutorGeneration = uuid.NewString()
				}
				if phase == "foreign-generation" {
					other.Store.ExecutorGeneration = uuid.NewString()
				}
				otherID := b3Admit(t, f, other)
				changes := map[string]any{"executor_state": "unknown", "unknown_reason": "executor-unavailable"}
				if phase == "foreign-generation" {
					changes = map[string]any{}
				}
				if phase == "terminal" {
					changes = map[string]any{"executor_state": "success", "executor_settled_at": f.store.now, "settled_at": f.store.now}
				}
				if len(changes) > 0 {
					require.NoError(t, f.store.svc.db.Model(&RuntimeReceipt{}).Where("run_id = ?", otherID).Updates(changes).Error)
				}
			}
			restoreLegacy(t, f, "ordinary:example", activitytypes.StatusRunning)
			require.NoError(t, f.store.svc.FailInterruptedAfterRestore(t.Context()))
			require.NoError(t, f.store.svc.RestoreGatedActivity(t.Context(), restoreReturnedRun(f), "example-backup"))
			before := restoreReceiptRead(t, f.store.svc.db.DB)
			db := f.store.svc.db.DB
			switch fault {
			case "receipt":
				require.NoError(t, db.Exec("CREATE TRIGGER fixture_restore_fault BEFORE UPDATE OF executor_settled_at ON runtime_receipts BEGIN SELECT RAISE(ABORT,'fixture receipt refusal'); END").Error)
			case "activity":
				require.NoError(t, db.Exec("CREATE TRIGGER fixture_restore_fault BEFORE UPDATE OF status ON activities BEGIN SELECT RAISE(ABORT,'fixture activity refusal'); END").Error)
			case "final":
				require.NoError(t, db.Exec("CREATE TRIGGER fixture_restore_fault BEFORE INSERT ON activity_messages BEGIN SELECT RAISE(ABORT,'fixture final refusal'); END").Error)
			case "commit":
				restoreDeferredFault(t, db, "CREATE TRIGGER fixture_restore_fault AFTER INSERT ON activity_messages")
			}
			// runtimeFixture holds the actual hub lifetime lock. No worker was registered;
			// a new service models the explicitly ended fixture owner, not a production restart.
			startup := NewRuntimeService(f.store.svc, &b1ResolverInternal{})
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			count, err := startup.SettleLocalStartup(ctx, c.Store)
			if fault != "none" {
				assert.Error(t, err)
				assert.Zero(t, count)
				assert.Equal(t, before, restoreReceiptRead(t, db))
				if fault == "commit" {
					assert.ErrorContains(t, err, "FOREIGN KEY constraint failed")
				}
				restoreHeldPeer(t, f, c, id)
				require.NoError(t, db.Exec("DROP TRIGGER fixture_restore_fault").Error)
				count, err = startup.SettleLocalStartup(ctx, c.Store)
			}
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			b3Interrupted(t, f, id, nil)
			after := restoreReceiptRead(t, db)
			expected := before
			expected.Receipts = append([]RuntimeReceipt(nil), before.Receipts...)
			expected.Activities = append([]Activity(nil), before.Activities...)
			for i, row := range expected.Receipts {
				if row.RunID == id {
					expected.Receipts[i] = b3Receipt(t, f, id)
				}
			}
			for i, row := range expected.Activities {
				if row.ID == id {
					expected.Activities[i] = b3Activity(t, f, id)
				}
			}
			var final ActivityMessage
			require.NoError(t, db.First(&final, "activity_id = ? AND code = 'runtime.interrupted'", id).Error)
			require.Equal(t, false, final.Payload["effectVerified"])
			// Compare preexisting messages separately from the sole conditional final.
			actualMessages := after.Messages[:0]
			for _, m := range after.Messages {
				if m.ID != final.ID {
					actualMessages = append(actualMessages, m)
				}
			}
			after.Messages = actualMessages
			require.Equal(t, expected, after, "remote, foreign-generation and terminal pairs must remain unchanged")
			stable := restoreReceiptRead(t, db)
			count, err = startup.SettleLocalStartup(ctx, c.Store)
			require.NoError(t, err)
			require.Zero(t, count)
			require.Equal(t, stable, restoreReceiptRead(t, db))
		})
	}
}

func TestRestoreReceiptsBusyPeer(t *testing.T) {
	for _, recreate := range []bool{false, true} {
		t.Run(fmt.Sprintf("recreate=%t", recreate), func(t *testing.T) {
			f := b1Fixture(t)
			c, id := restoreChild(t, f)
			restoreLegacy(t, f, "ordinary:busy", activitytypes.StatusQueued)
			before := restoreReceiptRead(t, f.store.svc.db.DB)
			peer := runtimeContender(t, f.store)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			held := f.store.svc.db.WithContext(ctx).Begin()
			require.NoError(t, held.Error)
			defer held.Rollback()
			require.NoError(t, peer.WithContext(ctx).Connection(func(pin *gorm.DB) error {
				require.NoError(t, pin.Exec("PRAGMA busy_timeout = 5").Error)
				defer func() { require.NoError(t, pin.Exec("PRAGMA busy_timeout = 2500").Error) }()
				service := NewActivityService(&database.DB{DB: pin}, nil)
				var err error
				if recreate {
					err = service.RestoreGatedActivity(ctx, restoreReturnedRun(f), "example-backup")
				} else {
					err = service.FailInterruptedAfterRestore(ctx)
				}
				var driver *sqlite.Error
				require.ErrorAs(t, err, &driver)
				require.Equal(t, 5, driver.Code()&0xff)
				return nil
			}))
			require.NoError(t, held.Rollback().Error)
			require.Equal(t, before, restoreReceiptRead(t, f.store.svc.db.DB))
			restoreHeldPeer(t, f, c, id)
			require.NoError(t, restoreInvoke(ctx, f, recreate, restoreReturnedRun(f)))
			require.Equal(t, before.Receipts, restoreReceiptRead(t, f.store.svc.db.DB).Receipts)
		})
	}
}
