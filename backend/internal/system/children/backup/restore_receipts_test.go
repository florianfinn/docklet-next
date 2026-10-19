// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant repair proof at actual paired restore finalizer and online execution boundaries.

package backup

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func restoreArchiveSessionCount(t *testing.T, f *restoreArchiveFixture) int64 {
	t.Helper()
	var n int64
	require.NoError(t, f.db.Raw("SELECT count(*) FROM user_sessions WHERE revoked_at IS NULL").Scan(&n).Error)
	return n
}
func restoreArchiveHostCount(t *testing.T, f *restoreArchiveFixture) int {
	t.Helper()
	pool, err := sqlitestore.Open(t.Context(), sqlitestore.FrancisPath(f.db.Path()), false, false)
	require.NoError(t, err)
	defer pool.Close()
	var n int
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT count(*) FROM arcane_francis_hosts").Scan(&n))
	return n
}

func TestRestoreReceiptsActualFinalizer(t *testing.T) {
	for _, fault := range []string{"none", "update", "cancelled", "sessions"} {
		t.Run(fault, func(t *testing.T) {
			source := restoreArchiveNew(t)
			source.seed(t)
			if fault == "update" {
				require.NoError(t, source.db.Exec("CREATE TRIGGER fixture_restore_fault BEFORE UPDATE OF status ON activities WHEN NEW.error = 'backup.interrupted' BEGIN SELECT RAISE(ABORT,'fixture repair refusal'); END").Error)
			}
			if fault == "sessions" {
				require.NoError(t, source.db.Exec("CREATE TRIGGER fixture_restore_fault BEFORE UPDATE ON user_sessions BEGIN SELECT RAISE(ABORT,'fixture session refusal'); END").Error)
			}
			record := restoreArchiveMetadata(t, source)
			target := restoreArchiveInstall(t, source, record)
			before := restoreArchiveRead(t, target.db.DB)
			require.EqualValues(t, 1, restoreArchiveSessionCount(t, target))
			require.Equal(t, 1, restoreArchiveHostCount(t, target))
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			if fault == "cancelled" {
				cancel()
			}
			err := finalizeRestoredDatabase(ctx, target.db, target.cfg)
			after := restoreArchiveRead(t, target.db.DB)
			if fault == "none" {
				require.NoError(t, err)
				restoreArchiveDeltas(t, before, after, target.legacy, "")
				require.Zero(t, restoreArchiveSessionCount(t, target))
				require.Zero(t, restoreArchiveHostCount(t, target))
			} else {
				require.ErrorIs(t, err, ErrRestoreRecoveryRequired)
				if fault == "sessions" {
					restoreArchiveDeltas(t, before, after, target.legacy, "")
				} else {
					require.Equal(t, before, after, "required repair failure cannot alter its atomic stage")
				}
				require.EqualValues(t, 1, restoreArchiveSessionCount(t, target))
				if fault == "cancelled" {
					require.Equal(t, 1, restoreArchiveHostCount(t, target))
				} else {
					require.Zero(t, restoreArchiveHostCount(t, target), "Francis clearing precedes Activity repair and has no enclosing rollback")
				}
				require.ErrorIs(t, CheckPendingRestore(target.cfg), ErrRestoreRecoveryRequired)
				if fault != "cancelled" {
					require.NoError(t, target.db.Exec("DROP TRIGGER fixture_restore_fault").Error)
				}
				require.NoError(t, finalizeRestoredDatabase(t.Context(), target.db, target.cfg))
			}
			restoreArchiveHeld(t, target)
		})
	}
}

func restoreArchiveOperation(t *testing.T, f *restoreArchiveFixture, id string, started activitytypes.Activity, status activitytypes.Status) {
	t.Helper()
	var row activity.Activity
	require.NoError(t, f.db.First(&row, "id = ?", started.ID).Error)
	require.Equal(t, started.ID, row.ID)
	require.Equal(t, started.CreatedAt, row.CreatedAt)
	require.Equal(t, started.StartedAt, row.StartedAt)
	require.Equal(t, new("example-restorer"), row.StartedByUserID)
	require.Equal(t, new("Restore operator"), row.StartedByUsername)
	require.Equal(t, "0", row.EnvironmentID)
	require.Equal(t, activitytypes.TypeResourceAction, row.Type)
	require.Equal(t, new("system_backup"), row.ResourceType)
	require.Equal(t, new(id), row.ResourceID)
	require.Equal(t, "system-backup", row.GateScope)
	require.Equal(t, "restore:"+id, row.GateKey)
	require.Equal(t, status, row.Status)
	if status == activitytypes.StatusRunning {
		require.Nil(t, row.EndedAt)
		require.Equal(t, "run.started", row.MessageCode)
	} else {
		require.NotNil(t, row.EndedAt)
		require.Equal(t, "activity.success", row.MessageCode)
		require.Equal(t, "backup.restored", row.LatestMessage)
	}
	var receipts int64
	require.NoError(t, f.db.Model(&activity.RuntimeReceipt{}).Where("run_id = ?", started.ID).Count(&receipts).Error)
	require.Zero(t, receipts, "restore recreation must not synthesize a runtime receipt")
}

// The test owns the gate context so executeRestore can be called synchronously
// and fully joined. StartRestore exercises its real joined admission path for
// that same current operation; neither high-level repair boundary is mocked.
func restoreArchiveStart(t *testing.T, s *Service, f *restoreArchiveFixture, record *backuptypes.SystemBackupRun) (context.Context, activitytypes.Activity) {
	t.Helper()
	actor := usertypes.Actor{ID: "example-restorer", Username: "Restore operator"}
	answer, err := f.activities.StartGatedActivity(t.Context(), activity.GateRequest{EnvironmentID: "0", Operation: "restore:" + record.ID, Scope: "system-backup",
		Type: activitytypes.TypeResourceAction, ResourceType: "system_backup", ResourceID: record.ID, StartedBy: &actor})
	require.NoError(t, err)
	require.NotNil(t, answer.WorkContext)
	operation, err := s.StartRestore(t.Context(), actor, "", record.ID, record.ID)
	require.NoError(t, err)
	require.Equal(t, answer.Run.ID, operation.ActivityID)
	require.Equal(t, record.ID, operation.BackupID)
	require.Equal(t, string(activitytypes.StatusRunning), operation.Status)
	return answer.WorkContext, answer.Run
}

func TestRestoreReceiptsActualOnlineFlow(t *testing.T) {
	source := restoreArchiveNew(t)
	source.seed(t)
	before := restoreArchiveRead(t, source.db.DB)
	record := restoreArchiveMetadata(t, source)
	target := restoreArchiveNew(t)
	target.cfg.BackupDir = source.cfg.BackupDir
	target.binding = source.binding
	target.held = source.held
	target.heldID = source.heldID
	var first, second restoreArchiveSnapshot
	var intermediate []string
	var started activitytypes.Activity
	saves, refreshes, unready := 0, 0, 0
	s := NewService(Dependencies{DB: target.db, Config: target.cfg, Activity: target.activities, Store: Store{
		Run: func(context.Context, string) (*backuptypes.SystemBackupRun, error) { return record, nil },
		SaveRun: func(ctx context.Context, saved *backuptypes.SystemBackupRun) error {
			saves++
			require.Equal(t, started.ID, saved.ActivityID)
			require.Equal(t, record.ID, saved.ID)
			first = restoreArchiveRead(t, target.db.DB)
			restoreArchiveDeltas(t, before, first, source.legacy, "")
			require.Zero(t, restoreArchiveSessionCount(t, target))
			// Fresh rows at the between-calls barrier prevent the direct repair from
			// masking a broken recreation helper, including terminal receipt ownership.
			fresh := target.command("between-calls")
			freshID := target.admit(t, fresh)
			require.NoError(t, target.db.Model(&activity.Activity{}).Where("id = ?", freshID).UpdateColumns(map[string]any{"gate_scope": "foreign:copied-between", "gate_key": "foreign-key"}).Error)
			terminal := target.command("between-terminal")
			terminalID := target.admit(t, terminal)
			now := time.Now().UTC().Truncate(time.Microsecond)
			require.NoError(t, target.db.Model(&activity.RuntimeReceipt{}).Where("run_id = ?", terminalID).Updates(map[string]any{"executor_state": "success", "executor_settled_at": now, "settled_at": now}).Error)
			intermediate = append(intermediate, target.legacyRow(t, "ordinary:between-calls", activitytypes.StatusQueued))
			second = restoreArchiveRead(t, target.db.DB)
			return ctx.Err()
		}}})
	s.SetExitUnready(func() { unready++ })
	s.SetRestoreSchedulerRefresh(func(ctx context.Context) error {
		refreshes++
		restoreArchiveDeltas(t, second, restoreArchiveRead(t, target.db.DB), intermediate, started.ID)
		restoreArchiveOperation(t, target, record.ID, started, activitytypes.StatusRunning)
		restoreArchiveHeld(t, target)
		require.ErrorIs(t, CheckPendingRestore(target.cfg), ErrRestoreRecoveryRequired)
		return ctx.Err()
	})
	workCtx, run := restoreArchiveStart(t, s, target, record)
	started = run
	ctx, cancel := context.WithTimeout(workCtx, 20*time.Second)
	defer cancel()
	s.executeRestore(ctx, usertypes.Actor{ID: "example-restorer", Username: "Restore operator"}, "", record.ID, started)
	require.Equal(t, 1, saves)
	require.Equal(t, 1, refreshes)
	require.Zero(t, unready)
	require.False(t, s.maintenance.Status().Active)
	require.NoError(t, CheckPendingRestore(target.cfg))
	restoreArchiveOperation(t, target, record.ID, started, activitytypes.StatusSuccess)
	restoreArchiveDeltas(t, second, restoreArchiveRead(t, target.db.DB), intermediate, started.ID)
	var messages []activity.ActivityMessage
	require.NoError(t, target.db.Where("activity_id = ?", started.ID).Find(&messages).Error)
	require.Len(t, messages, 1)
	require.Equal(t, "backup.restored", messages[0].Message)
	restoreArchiveHeld(t, target)
}

func TestRestoreReceiptsActualOnlineFailures(t *testing.T) {
	for _, fault := range []string{"first-update", "second-update", "insert", "collision", "commit", "missing-table", "save-run"} {
		t.Run(fault, func(t *testing.T) {
			source := restoreArchiveNew(t)
			source.seed(t)
			if fault == "first-update" {
				require.NoError(t, source.db.Exec("CREATE TRIGGER fixture_restore_fault BEFORE UPDATE OF status ON activities WHEN NEW.error = 'backup.interrupted' BEGIN SELECT RAISE(ABORT,'fixture first repair refusal'); END").Error)
			}
			before := restoreArchiveRead(t, source.db.DB)
			record := restoreArchiveMetadata(t, source)
			target := restoreArchiveNew(t)
			target.cfg.BackupDir = source.cfg.BackupDir
			target.held = source.held
			target.heldID = source.heldID
			target.binding = source.binding
			saves, refreshes, unready := 0, 0, 0
			var started activitytypes.Activity
			var atomicBefore restoreArchiveSnapshot
			s := NewService(Dependencies{DB: target.db, Config: target.cfg, Activity: target.activities, Store: Store{
				Run: func(context.Context, string) (*backuptypes.SystemBackupRun, error) { return record, nil },
				SaveRun: func(ctx context.Context, _ *backuptypes.SystemBackupRun) error {
					saves++
					restoreArchiveDeltas(t, before, restoreArchiveRead(t, target.db.DB), source.legacy, "")
					target.legacyRow(t, "ordinary:between-calls", activitytypes.StatusQueued)
					switch fault {
					case "second-update":
						require.NoError(t, target.db.Exec("CREATE TRIGGER fixture_restore_fault BEFORE UPDATE OF status ON activities WHEN NEW.error = 'backup.interrupted' BEGIN SELECT RAISE(ABORT,'fixture second repair refusal'); END").Error)
					case "insert":
						require.NoError(t, target.db.Exec("CREATE TRIGGER fixture_restore_fault BEFORE INSERT ON activities BEGIN SELECT RAISE(ABORT,'fixture recreation refusal'); END").Error)
					case "collision":
						row := activity.Activity{BaseModel: database.BaseModel{ID: started.ID, CreatedAt: started.CreatedAt}, EnvironmentID: "0", Type: activitytypes.TypeResourceAction, Status: activitytypes.StatusSuccess, StartedAt: started.StartedAt}
						require.NoError(t, target.db.Create(&row).Error)
					case "commit":
						require.NoError(t, target.db.Exec("CREATE TABLE fixture_restore_parent(id INTEGER PRIMARY KEY)").Error)
						require.NoError(t, target.db.Exec("CREATE TABLE fixture_restore_deferred(id INTEGER REFERENCES fixture_restore_parent(id) DEFERRABLE INITIALLY DEFERRED)").Error)
						require.NoError(t, target.db.Exec("CREATE TRIGGER fixture_restore_fault AFTER INSERT ON activities BEGIN INSERT INTO fixture_restore_deferred VALUES(1); END").Error)
					}
					atomicBefore = restoreArchiveRead(t, target.db.DB)
					if fault == "missing-table" {
						require.NoError(t, target.db.Exec("ALTER TABLE runtime_receipts RENAME TO fixture_restore_receipts").Error)
					}
					if fault == "save-run" {
						return ErrSystemBackupUnavailable
					}
					return ctx.Err()
				}}})
			s.SetExitUnready(func() { unready++ })
			s.SetRestoreSchedulerRefresh(func(context.Context) error { refreshes++; return nil })
			workCtx, run := restoreArchiveStart(t, s, target, record)
			started = run
			ctx, cancel := context.WithTimeout(workCtx, 20*time.Second)
			defer cancel()
			s.executeRestore(ctx, usertypes.Actor{ID: "example-restorer", Username: "Restore operator"}, "", record.ID, started)
			require.Equal(t, 1, unready)
			require.Zero(t, refreshes)
			require.True(t, s.maintenance.Status().Active)
			require.Equal(t, "blocked", s.maintenance.Status().Phase)
			require.ErrorIs(t, CheckPendingRestore(target.cfg), ErrRestoreRecoveryRequired)
			_, err := s.maintenance.Admit()
			require.ErrorIs(t, err, ErrMaintenanceActive, "no resume past required failure")
			if fault == "first-update" {
				require.Zero(t, saves)
				require.Equal(t, before, restoreArchiveRead(t, target.db.DB))
			} else {
				require.Equal(t, 1, saves)
				if fault == "missing-table" {
					require.NoError(t, target.db.Exec("ALTER TABLE fixture_restore_receipts RENAME TO runtime_receipts").Error)
				}
				require.Equal(t, atomicBefore, restoreArchiveRead(t, target.db.DB), "failed recreation transaction has no partial legacy update or operation insert")
			}
			var finals int64
			require.NoError(t, target.db.Model(&activity.ActivityMessage{}).Where("activity_id = ?", started.ID).Count(&finals).Error)
			require.Zero(t, finals, "blocked restore cannot report successful completion")
			restoreArchiveHeld(t, target)
			if fault == "first-update" || fault == "second-update" || fault == "insert" || fault == "commit" {
				require.NoError(t, target.db.Exec("DROP TRIGGER fixture_restore_fault").Error)
			}
		})
	}
}

// This binds an explicitly ended fixture owner to the restored generation. It
// proves the composed writer remains usable after actual pair repair; F-d owns
// real process continuity, intake ordering and readiness before any resume.
func TestRestoreReceiptsActualPairStartupComposition(t *testing.T) {
	source := restoreArchiveNew(t)
	source.seed(t)
	require.NoError(t, source.db.Model(&activity.RuntimeReceipt{}).Where("run_id <> ? AND environment_id = '0' AND settled_at IS NULL", source.heldID).UpdateColumn("executor_generation", uuid.NewString()).Error)
	record := restoreArchiveMetadata(t, source)
	target := restoreArchiveInstall(t, source, record)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	require.NoError(t, finalizeRestoredDatabase(ctx, target.db, target.cfg))
	started := activitytypes.Activity{ID: "example-current-restore", CreatedAt: time.Now().UTC().Truncate(time.Microsecond), StartedAt: time.Now().UTC().Truncate(time.Microsecond)}
	require.NoError(t, target.activities.RestoreGatedActivity(ctx, started, record.ID))
	before := restoreArchiveRead(t, target.db.DB)
	restoreArchiveHeld(t, target)
	startup := activity.NewRuntimeService(target.activities, restoreArchiveResolver{})
	count, err := startup.SettleLocalStartup(ctx, target.binding)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	after := restoreArchiveRead(t, target.db.DB)
	var receipt activity.RuntimeReceipt
	require.NoError(t, target.db.First(&receipt, "run_id = ?", target.heldID).Error)
	require.Equal(t, new("failed"), receipt.ExecutorState)
	require.Equal(t, new("runtime.interrupted"), receipt.ExecutorResultCode)
	require.NotNil(t, receipt.SettledAt)
	var run activity.Activity
	require.NoError(t, target.db.First(&run, "id = ?", target.heldID).Error)
	require.Equal(t, activitytypes.StatusFailed, run.Status)
	require.Equal(t, "runtime.interrupted", run.MessageCode)
	var finals []activity.ActivityMessage
	require.NoError(t, target.db.Where("activity_id = ? AND code = 'runtime.interrupted'", target.heldID).Find(&finals).Error)
	require.Len(t, finals, 1)
	require.Equal(t, false, finals[0].Payload["effectVerified"])
	// Remove exactly the composed pair and its one final from comparison; every
	// other receipt, Activity, binding, generation and retained message is exact.
	for i, row := range after.Receipts {
		if row.RunID == target.heldID {
			for _, old := range before.Receipts {
				if old.RunID == row.RunID {
					after.Receipts[i] = old
				}
			}
		}
	}
	for i, row := range after.Activities {
		if row.ID == target.heldID {
			for _, old := range before.Activities {
				if old.ID == row.ID {
					after.Activities[i] = old
				}
			}
		}
	}
	messages := make([]activity.ActivityMessage, 0, len(after.Messages))
	for _, m := range after.Messages {
		if m.ID != finals[0].ID {
			messages = append(messages, m)
		}
	}
	after.Messages = messages
	require.Equal(t, before, after)
	stable := restoreArchiveRead(t, target.db.DB)
	count, err = startup.SettleLocalStartup(ctx, target.binding)
	require.NoError(t, err)
	require.Zero(t, count)
	require.Equal(t, stable, restoreArchiveRead(t, target.db.DB))
	require.ErrorIs(t, CheckPendingRestore(target.cfg), ErrRestoreRecoveryRequired, "fixture settlement is not restore readiness")
}
