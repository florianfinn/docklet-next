// Source: florianfinn/docklet-next/docs/design/run-gate.md
// Revision: ab701984e86075694baabdc5efe14bed287768fc
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: Process restart and durable run-repair cases.

package activity

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRunGate_ProcessFixture(t *testing.T) {
	mode := os.Getenv("RUN_GATE_CHILD_MODE")
	if mode == "" {
		return
	}
	db, err := database.InitializeHub(t.Context(), os.Getenv("RUN_GATE_CHILD_URL"), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	svc := NewActivityService(db, nil)
	if mode == "job_complete" {
		id := uuid.NewString()
		stamp := time.Now().UTC().Truncate(time.Microsecond)
		run := scheduler.Run{ID: "job-" + id, ActivityID: "activity-" + id, JobID: "example-job", EnvironmentID: "0", Status: scheduler.Succeeded, CreatedAt: stamp.Add(-time.Minute), UpdatedAt: stamp}
		require.NoError(t, svc.SyncJobRun(t.Context(), run, "Example job"))
		fmt.Printf("RUN_ID:%s\n", run.ActivityID)
		os.Exit(0)
	}
	if mode == "ungated_queued" {
		run, err := svc.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy, Queue: true, DeferSlot: true})
		require.NoError(t, err)
		fmt.Printf("RUN_ID:%s\n", run.ID)
		os.Exit(0)
	}
	started, err := svc.StartGatedActivity(t.Context(), matrixGateRequest())
	require.NoError(t, err)
	if mode == "gated_cancelled_in_memory" {
		_, err = svc.CancelActivity(t.Context(), "0", started.Run.ID, "operator")
		require.NoError(t, err)
	}
	if mode == "before_commit" {
		tx := db.Begin()
		require.NoError(t, tx.Model(&Activity{}).Where("id = ?", started.Run.ID).Update("status", activity.StatusSuccess).Error)
		fmt.Printf("RUN_ID:%s\n", started.Run.ID)
		os.Exit(0)
	}
	if mode == "after_commit" {
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&Activity{}).Where("id = ?", started.Run.ID).Updates(map[string]any{
				"status": activity.StatusSuccess, "message_code": activity.CodeActivitySuccess,
				"latest_message": "done", "progress": 100,
				"ended_at": time.Now().UTC(), "updated_at": time.Now().UTC(),
				"duration_ms": int64(0),
			}).Error; err != nil {
				return err
			}
			return tx.Create(&ActivityMessage{ActivityID: started.Run.ID, Level: activity.MessageLevelSuccess, Message: "done", Code: string(activity.CodeActivitySuccess)}).Error
		})
		require.NoError(t, err)
	}
	fmt.Printf("RUN_ID:%s\n", started.Run.ID)
	os.Exit(0)
}

func matrixPreviousHub(t *testing.T, db *database.DB, mode string) string {
	t.Helper()
	var row struct{ File string }
	require.NoError(t, db.Raw("PRAGMA database_list").Scan(&row).Error)
	url := sqlitestore.URL(row.File)
	require.NoError(t, db.Close())
	exe, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestRunGate_ProcessFixture$", "-test.v")
	cmd.Env = append(os.Environ(), "RUN_GATE_CHILD_MODE="+mode, "RUN_GATE_CHILD_URL="+url)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	reopened, err := database.InitializeHub(t.Context(), url, database.MigrationOptions{})
	require.NoError(t, err)
	*db = *reopened
	for _, line := range strings.Split(string(output), "\n") {
		if id, ok := strings.CutPrefix(line, "RUN_ID:"); ok {
			return strings.TrimSpace(id)
		}
	}
	t.Fatalf("previous hub did not return a run ID: %s", output)
	return ""
}

func matrixNewHub(t *testing.T, db *database.DB) *ActivityService {
	t.Helper()

	return NewActivityService(db, nil)
}

func TestRunGate_Case25_CrashWhileRunning(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	id := matrixPreviousHub(t, db, "gated_running")
	svc := matrixNewHub(t, db)
	require.Equal(t, activity.StatusRunning, matrixRow(t, db.DB, id).Status)
	count, err := svc.RepairStartup(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	row := matrixRow(t, db.DB, id)
	require.Equal(t, activity.StatusFailed, row.Status)
	require.Equal(t, string(activity.CodeActivityFailed), row.MessageCode)
	require.NotNil(t, row.EndedAt)
	next := matrixGate(t, svc)
	require.NotEqual(t, id, next.Run.ID)
}

func TestRunGate_Case26_CrashWhileQueued(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	id := matrixPreviousHub(t, db, "ungated_queued")
	svc := matrixNewHub(t, db)
	count, err := svc.RepairStartup(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	row := matrixRow(t, db.DB, id)
	require.Equal(t, activity.StatusFailed, row.Status)
	require.Equal(t, string(activity.CodeActivityFailed), row.MessageCode)
	require.NotNil(t, row.EndedAt)
}

func TestRunGate_Case27_CrashDuringInMemoryCancellation(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	id := matrixPreviousHub(t, db, "gated_cancelled_in_memory")
	svc := matrixNewHub(t, db)
	count, err := svc.RepairStartup(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	row := matrixRow(t, db.DB, id)
	require.Equal(t, activity.StatusFailed, row.Status)
	require.Equal(t, string(activity.CodeActivityFailed), row.MessageCode)
	require.NotNil(t, row.Error)
	require.Empty(t, matrixMessages(t, db.DB, id, "activity.cancel_requested"))
	require.Len(t, matrixMessages(t, db.DB, id, string(activity.CodeActivityFailed)), 1)
}

func TestRunGate_Case28_CrashBeforeCompletionCommit(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	id := matrixPreviousHub(t, db, "before_commit")
	svc := matrixNewHub(t, db)
	require.Equal(t, activity.StatusRunning, matrixRow(t, db.DB, id).Status)
	count, err := svc.RepairStartup(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	_, err = svc.RepairStartup(t.Context())
	require.NoError(t, err)
	require.Equal(t, activity.StatusFailed, matrixRow(t, db.DB, id).Status)
	require.Len(t, matrixMessages(t, db.DB, id, string(activity.CodeActivityFailed)), 1)
}

func TestRunGate_Case29_CrashAfterCommitBeforeEvent(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	id := matrixPreviousHub(t, db, "after_commit")
	svc := matrixNewHub(t, db)
	count, err := svc.RepairStartup(t.Context())
	require.NoError(t, err)
	require.Zero(t, count)
	handler := NewHandler(svc, EnvironmentDependencies{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events := make(chan activity.StreamEvent, 2)
	go handler.RunLocalStreamProducer(ctx, 20, events)
	select {
	case event := <-events:
		require.Equal(t, "snapshot", event.Type)
		require.Len(t, event.Activities, 1)
		require.Equal(t, id, event.Activities[0].ID)
		require.Equal(t, activity.StatusSuccess, event.Activities[0].Status)
	case <-time.After(time.Second):
		t.Fatal("reconnect did not reload terminal snapshot")
	}
	require.Len(t, matrixMessages(t, db.DB, id, string(activity.CodeActivitySuccess)), 1)
}

func TestRunGate_Case30_JobReceiptSurvivesRestartAndPruning(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	id := matrixPreviousHub(t, db, "job_complete")
	svc := matrixNewHub(t, db)
	row := matrixRow(t, db.DB, id)
	stamp, err := time.Parse(time.RFC3339Nano, row.Metadata["runUpdatedAt"].(string))
	require.NoError(t, err)
	run := scheduler.Run{ID: row.Metadata["runId"].(string), ActivityID: id, JobID: "example-job", EnvironmentID: "0", Status: scheduler.Succeeded, CreatedAt: row.StartedAt, UpdatedAt: stamp}
	// A coordinator snapshot taken before this sync omits the run ID.
	require.NoError(t, svc.SyncJobRun(t.Context(), run, "Example job"))
	deleted, err := svc.PruneSyncReceipts(t.Context(), nil)
	require.NoError(t, err)
	require.Zero(t, deleted)
	require.NoError(t, db.Delete(&Activity{}, "id = ?", id).Error)
	require.NoError(t, svc.SyncJobRun(t.Context(), run, "Example job"))
	var count int64
	require.NoError(t, db.Model(&Activity{}).Where("id = ?", id).Count(&count).Error)
	require.Zero(t, count)
	var receipts int64
	require.NoError(t, db.Model(&ActivitySyncReceipt{}).Where("run_id = ?", run.ID).Count(&receipts).Error)
	require.EqualValues(t, 1, receipts)
}

func TestRunGate_Case35_PoolCompletionFailureKeepsScope(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := matrixNewHub(t, db)
	started := matrixGate(t, svc)
	busyError := testsqlite.BusyError(t)
	var attempts int
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:retryable_failure", func(tx *gorm.DB) {
		attempts++
		tx.AddError(busyError)
	}))
	_, err := svc.CompleteActivity(started.WorkContext, started.Run.ID, activity.StatusSuccess, "done", nil)
	require.Error(t, err)
	require.Equal(t, 3, attempts)
	require.Equal(t, activity.StatusRunning, matrixRow(t, db.DB, started.Run.ID).Status)
	request := matrixGateRequest()
	request.Operation = "remove"
	_, err = svc.StartGatedActivity(t.Context(), request)
	var conflict *GateConflict
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, started.Run.ID, conflict.RunID)
}
