//go:build postgres_integration

// Source: florianfinn/docklet-next/docs/design/run-gate.md
// Revision: ab701984e86075694baabdc5efe14bed287768fc
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: PostgreSQL cases for the run-gate transition matrix.

package activity

import (
	"context"
	"sync"
	"testing"
	"time"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func matrixGateRequest() GateRequest {
	return GateRequest{Operation: "deploy", Scope: "stack:example", Type: activity.TypeProjectDeploy}
}

func matrixGate(t *testing.T, svc *ActivityService) GateAnswer {
	t.Helper()
	answer, err := svc.StartGatedActivity(t.Context(), matrixGateRequest())
	require.NoError(t, err)
	return answer
}

func matrixRow(t *testing.T, db *gorm.DB, id string) Activity {
	t.Helper()
	var row Activity
	require.NoError(t, db.First(&row, "id = ?", id).Error)
	return row
}

func matrixMessages(t *testing.T, db *gorm.DB, id, code string) []ActivityMessage {
	t.Helper()
	var rows []ActivityMessage
	require.NoError(t, db.Where("activity_id = ? AND code = ?", id, code).Find(&rows).Error)
	return rows
}

func TestRunGate_Case01_UngatedQueuedNoScope(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	run, err := NewActivityService(db, nil).StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy, Queue: true, DeferSlot: true})
	require.NoError(t, err)
	require.Equal(t, activity.StatusQueued, run.Status)
	require.Equal(t, string(activity.CodeActivityQueued), run.MessageCode)
	require.Empty(t, matrixRow(t, db.DB, run.ID).GateScope)
}

func TestRunGate_Case02_NewGatedOneWorker(t *testing.T) {
	svc := NewActivityService(setupActivityServiceTestDBInternal(t), nil)
	started := matrixGate(t, svc)
	require.Equal(t, activity.StatusRunning, started.Run.Status)
	require.Equal(t, string(activity.CodeRunStarted), started.Run.MessageCode)
	require.NotNil(t, started.WorkContext)
	joined, err := svc.StartGatedActivity(t.Context(), matrixGateRequest())
	require.NoError(t, err)
	require.Nil(t, joined.WorkContext)
	require.Equal(t, started.Run.ID, joined.Run.ID)
}

func TestRunGate_Case03_QueuedSlotBecomesRunning(t *testing.T) {
	svc := NewActivityService(setupActivityServiceTestDBInternal(t), nil)
	run, err := svc.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy, Queue: true, DeferSlot: true})
	require.NoError(t, err)
	require.NoError(t, svc.AwaitActivitySlot(t.Context(), run.ID, "0"))
	current, err := svc.GetActivityDetail(t.Context(), "0", run.ID, 10)
	require.NoError(t, err)
	require.Equal(t, activity.StatusRunning, current.Activity.Status)
	require.Equal(t, string(activity.CodeActivityRunning), current.Activity.MessageCode)
}

func TestRunGate_Case04_SameOperationJoins(t *testing.T) {
	svc := NewActivityService(setupActivityServiceTestDBInternal(t), nil)
	first := matrixGate(t, svc)
	second, err := svc.StartGatedActivity(t.Context(), matrixGateRequest())
	require.NoError(t, err)
	require.Equal(t, string(activity.CodeRunJoined), second.Code)
	require.Equal(t, first.Run.ID, second.Run.ID)
	require.Nil(t, second.WorkContext)
}

func TestRunGate_Case05_DifferentOperationBusy(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	first := matrixGate(t, svc)
	request := matrixGateRequest()
	request.Operation = "remove"
	_, err := svc.StartGatedActivity(t.Context(), request)
	var conflict *GateConflict
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, string(activity.CodeRunBusy), conflict.Code)
	require.Equal(t, first.Run.ID, conflict.RunID)
	require.Equal(t, activity.StatusRunning, matrixRow(t, db.DB, first.Run.ID).Status)
}

func TestRunGate_Case06_CancellingOperationConflicts(t *testing.T) {
	svc := NewActivityService(setupActivityServiceTestDBInternal(t), nil)
	first := matrixGate(t, svc)
	_, err := svc.CancelActivity(t.Context(), "0", first.Run.ID, "operator")
	require.NoError(t, err)
	request := matrixGateRequest()
	request.Check = true
	_, err = svc.StartGatedActivity(t.Context(), request)
	var conflict *GateConflict
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, string(activity.CodeRunCancelling), conflict.Code)
	require.Equal(t, first.Run.ID, conflict.RunID)
}

func TestRunGate_Case07_QueuedCancellationInMemory(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	run, work, err := svc.StartTrackedActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy, Queue: true, DeferSlot: true})
	require.NoError(t, err)
	requested, err := svc.CancelActivity(t.Context(), "0", run.ID, "operator")
	require.NoError(t, err)
	require.Equal(t, activity.StatusQueued, requested.Status)
	require.ErrorIs(t, context.Cause(work), activitylib.ErrCanceled)
	require.Error(t, work.Err())
	require.Empty(t, matrixMessages(t, db.DB, run.ID, "activity.cancel_requested"))
}

func TestRunGate_Case08_QueuedCancelStopsSlotStart(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	run, work, err := svc.StartTrackedActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy, Queue: true, DeferSlot: true})
	require.NoError(t, err)
	_, err = svc.CancelActivity(t.Context(), "0", run.ID, "operator")
	require.NoError(t, err)
	require.Error(t, work.Err())
	require.ErrorIs(t, svc.AwaitActivitySlot(work, run.ID, "0"), activitylib.ErrCanceled)
	finished, err := svc.CompleteActivity(work, run.ID, activity.StatusSuccess, "late", nil)
	require.NoError(t, err)
	require.Equal(t, activity.StatusCancelled, finished.Status)
	require.NotEqual(t, activity.StatusRunning, matrixRow(t, db.DB, run.ID).Status)
}

func TestRunGate_Case09_GatedCancellationKeepsScope(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	started := matrixGate(t, svc)
	requested, err := svc.CancelActivity(t.Context(), "0", started.Run.ID, "operator")
	require.NoError(t, err)
	require.Equal(t, activity.StatusRunning, requested.Status)
	require.ErrorIs(t, context.Cause(started.WorkContext), activitylib.ErrCanceled)
	require.Equal(t, string(activity.CodeRunStarted), requested.MessageCode)
	require.Error(t, started.WorkContext.Err())
	require.Empty(t, matrixMessages(t, db.DB, started.Run.ID, "activity.cancel_requested"))
	request := matrixGateRequest()
	request.Operation = "remove"
	_, err = svc.StartGatedActivity(t.Context(), request)
	var conflict *GateConflict
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, string(activity.CodeRunBusy), conflict.Code)
}

func TestRunGate_Case10_RepeatedCancellationIdempotent(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	started := matrixGate(t, svc)
	first, err := svc.CancelActivity(t.Context(), "0", started.Run.ID, "operator")
	require.NoError(t, err)
	second, err := svc.CancelActivity(t.Context(), "0", started.Run.ID, "operator")
	require.NoError(t, err)
	require.Equal(t, first.Status, second.Status)
	require.Equal(t, activity.StatusRunning, second.Status)
	require.Equal(t, activity.StatusRunning, matrixRow(t, db.DB, started.Run.ID).Status)
	require.Empty(t, matrixMessages(t, db.DB, started.Run.ID, "activity.cancel_requested"))
}

func TestRunGate_Case11_ProgressUpdatesSnapshotAndHistory(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	started := matrixGate(t, svc)
	progress := 42
	_, err := svc.AppendMessage(started.WorkContext, started.Run.ID, AppendActivityMessageRequest{Message: "pulling", Code: string(activity.CodeDeployPull), Step: "pull", Progress: &progress})
	require.NoError(t, err)
	current := matrixRow(t, db.DB, started.Run.ID)
	require.Equal(t, 42, *current.Progress)
	require.Equal(t, "pull", current.Step)
	require.Equal(t, string(activity.CodeDeployPull), current.MessageCode)
	require.Len(t, matrixMessages(t, db.DB, started.Run.ID, string(activity.CodeDeployPull)), 1)
}

func TestRunGate_Case12_ProgressAfterCancelPreservesSnapshot(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	started := matrixGate(t, svc)
	_, err := svc.CancelActivity(t.Context(), "0", started.Run.ID, "operator")
	require.NoError(t, err)
	progress := 70
	_, err = svc.AppendMessage(started.WorkContext, started.Run.ID, AppendActivityMessageRequest{Message: "unwinding", Code: string(activity.CodeDeployPull), Progress: &progress})
	require.NoError(t, err)
	code := string(activity.CodeDeployPull)
	_, err = svc.UpdateActivity(started.WorkContext, started.Run.ID, UpdateActivityRequest{LatestMessage: new("later progress"), MessageCode: &code})
	require.NoError(t, err)
	current := matrixRow(t, db.DB, started.Run.ID)
	require.Equal(t, code, current.MessageCode)
	require.Equal(t, "later progress", current.LatestMessage)
	require.ErrorIs(t, context.Cause(started.WorkContext), activitylib.ErrCanceled)
	require.Equal(t, 70, *current.Progress)
	require.Len(t, matrixMessages(t, db.DB, started.Run.ID, code), 1)
}

func TestRunGate_Case13_CancelledWorkerCanWrite(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	started := matrixGate(t, svc)
	_, err := svc.CancelActivity(t.Context(), "0", started.Run.ID, "operator")
	require.NoError(t, err)
	require.Error(t, started.WorkContext.Err())
	progress := 33
	_, err = svc.UpdateActivity(started.WorkContext, started.Run.ID, UpdateActivityRequest{Progress: &progress})
	require.NoError(t, err)
	_, err = svc.AppendMessages(started.WorkContext, started.Run.ID, []AppendActivityMessageRequest{{Message: "cleanup"}})
	require.NoError(t, err)
	require.NoError(t, svc.PatchActivityMetadata(started.WorkContext, started.Run.ID, map[string]any{"cleanup": true}))
	current := matrixRow(t, db.DB, started.Run.ID)
	require.Equal(t, 33, *current.Progress)
	require.Equal(t, true, current.Metadata["cleanup"])
}

func TestRunGate_Case14_WorkerSuccessOnce(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	started := matrixGate(t, svc)
	finished, err := svc.CompleteActivity(started.WorkContext, started.Run.ID, activity.StatusSuccess, "done", nil)
	require.NoError(t, err)
	require.Equal(t, activity.StatusSuccess, finished.Status)
	require.Equal(t, string(activity.CodeActivitySuccess), finished.MessageCode)
	require.Equal(t, 100, *finished.Progress)
	require.Len(t, matrixMessages(t, db.DB, started.Run.ID, string(activity.CodeActivitySuccess)), 1)
}

func TestRunGate_Case15_WorkerFailureOnce(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	started := matrixGate(t, svc)
	reason := "operation failed"
	finished, err := svc.CompleteActivity(started.WorkContext, started.Run.ID, activity.StatusFailed, reason, &reason)
	require.NoError(t, err)
	require.Equal(t, activity.StatusFailed, finished.Status)
	require.Equal(t, string(activity.CodeActivityFailed), finished.MessageCode)
	require.Equal(t, reason, *finished.Error)
	require.Len(t, matrixMessages(t, db.DB, started.Run.ID, string(activity.CodeActivityFailed)), 1)
}

func TestRunGate_Case16_CancelOverridesWorkerResult(t *testing.T) {
	for _, result := range []activity.Status{activity.StatusSuccess, activity.StatusFailed} {
		t.Run(string(result), func(t *testing.T) {
			db := setupActivityServiceTestDBInternal(t)
			svc := NewActivityService(db, nil)
			started := matrixGate(t, svc)
			_, err := svc.CancelActivity(t.Context(), "0", started.Run.ID, "operator")
			require.NoError(t, err)
			failure := "late failure"
			finished, err := svc.CompleteActivity(context.WithoutCancel(started.WorkContext), started.Run.ID, result, "late", &failure)
			require.NoError(t, err)
			require.Equal(t, activity.StatusCancelled, finished.Status)
			require.Equal(t, string(activity.CodeActivityCancelled), finished.MessageCode)
			require.Nil(t, finished.Error)
			require.Len(t, matrixMessages(t, db.DB, started.Run.ID, string(activity.CodeActivityCancelled)), 1)
		})
	}
}

func TestRunGate_Case17_SlowCancellationRetainsScope(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	started := matrixGate(t, svc)
	_, err := svc.CancelActivity(t.Context(), "0", started.Run.ID, "operator")
	require.NoError(t, err)
	require.NoError(t, db.Model(&Activity{}).Where("id = ?", started.Run.ID).Update("started_at", time.Now().Add(-3*time.Minute)).Error)
	swept, err := NewActivityService(db, nil).FailAbandonedActivities(t.Context())
	require.NoError(t, err)
	require.Zero(t, swept)
	require.Equal(t, activity.StatusRunning, matrixRow(t, db.DB, started.Run.ID).Status)
	request := matrixGateRequest()
	request.Operation = "remove"
	_, err = svc.StartGatedActivity(t.Context(), request)
	var conflict *GateConflict
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, started.Run.ID, conflict.RunID)
}

func TestRunGate_Case18_DuplicateCompletionOneFinalMessage(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	started := matrixGate(t, svc)
	first, err := svc.CompleteActivity(started.WorkContext, started.Run.ID, activity.StatusSuccess, "done", nil)
	require.NoError(t, err)
	second, err := svc.CompleteActivity(started.WorkContext, started.Run.ID, activity.StatusFailed, "late", nil)
	require.NoError(t, err)
	require.Equal(t, first.Status, second.Status)
	require.Equal(t, first.EndedAt, second.EndedAt)
	require.Len(t, matrixMessages(t, db.DB, started.Run.ID, string(activity.CodeActivitySuccess)), 1)
	require.Empty(t, matrixMessages(t, db.DB, started.Run.ID, string(activity.CodeActivityFailed)))
}

func TestRunGate_Case19_CheckFreshSuccess(t *testing.T) {
	svc := NewActivityService(setupActivityServiceTestDBInternal(t), nil)
	started := matrixGate(t, svc)
	_, err := svc.CompleteActivity(started.WorkContext, started.Run.ID, activity.StatusSuccess, "done", nil)
	require.NoError(t, err)
	request := matrixGateRequest()
	request.Check = true
	fresh, err := svc.StartGatedActivity(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, string(activity.CodeRunFresh), fresh.Code)
	require.Equal(t, started.Run.ID, fresh.Run.ID)
	require.Nil(t, fresh.WorkContext)
}

func TestRunGate_Case20_CheckBoundaryExclusive(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	var predicateAtBoundary bool
	require.NoError(t, db.Raw("SELECT (NOW() - INTERVAL '30 seconds') "+checkFreshnessOperator+" NOW() - INTERVAL '30 seconds'").Scan(&predicateAtBoundary).Error)
	require.False(t, predicateAtBoundary)
	svc := NewActivityService(db, nil)
	started := matrixGate(t, svc)
	_, err := svc.CompleteActivity(started.WorkContext, started.Run.ID, activity.StatusSuccess, "done", nil)
	require.NoError(t, err)
	require.NoError(t, db.Model(&Activity{}).Where("id = ?", started.Run.ID).Update("ended_at", gorm.Expr("NOW() - INTERVAL '30 seconds'")).Error)
	request := matrixGateRequest()
	request.Check = true
	next, err := svc.StartGatedActivity(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, string(activity.CodeRunStarted), next.Code)
	require.NotEqual(t, started.Run.ID, next.Run.ID)
}

func TestRunGate_Case21_FailureIsNotFresh(t *testing.T) {
	svc := NewActivityService(setupActivityServiceTestDBInternal(t), nil)
	started := matrixGate(t, svc)
	_, err := svc.CompleteActivity(started.WorkContext, started.Run.ID, activity.StatusFailed, "failed", new("failed"))
	require.NoError(t, err)
	request := matrixGateRequest()
	request.Check = true
	next, err := svc.StartGatedActivity(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, string(activity.CodeRunStarted), next.Code)
	require.NotEqual(t, started.Run.ID, next.Run.ID)
}

func TestRunGate_Case22_RecheckBypassesFreshness(t *testing.T) {
	svc := NewActivityService(setupActivityServiceTestDBInternal(t), nil)
	started := matrixGate(t, svc)
	_, err := svc.CompleteActivity(started.WorkContext, started.Run.ID, activity.StatusSuccess, "done", nil)
	require.NoError(t, err)
	request := matrixGateRequest()
	request.Check, request.Recheck = true, true
	next, err := svc.StartGatedActivity(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, string(activity.CodeRunStarted), next.Code)
	require.NotEqual(t, started.Run.ID, next.Run.ID)
}

func TestRunGate_Case23_ActiveBeforeRecheck(t *testing.T) {
	svc := NewActivityService(setupActivityServiceTestDBInternal(t), nil)
	started := matrixGate(t, svc)
	request := matrixGateRequest()
	request.Check, request.Recheck = true, true
	joined, err := svc.StartGatedActivity(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, string(activity.CodeRunJoined), joined.Code)
	require.Equal(t, started.Run.ID, joined.Run.ID)
	request.Operation = "remove"
	_, err = svc.StartGatedActivity(t.Context(), request)
	var conflict *GateConflict
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, string(activity.CodeRunBusy), conflict.Code)
	_, err = svc.CancelActivity(t.Context(), "0", started.Run.ID, "operator")
	require.NoError(t, err)
	request.Operation = "deploy"
	_, err = svc.StartGatedActivity(t.Context(), request)
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, string(activity.CodeRunCancelling), conflict.Code)
}

func TestRunGate_Case24_ConcurrentScopeAdmission(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	tx := db.Begin()
	require.NoError(t, tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "1:0stack:example").Error)
	defer tx.Rollback()
	services := [2]*ActivityService{NewActivityService(db, nil), NewActivityService(db, nil)}
	var answers [2]GateAnswer
	var failures [2]error
	var workers sync.WaitGroup
	workers.Add(2)
	for i := range services {
		go func(i int) {
			defer workers.Done()
			answers[i], failures[i] = services[i].StartGatedActivity(context.Background(), matrixGateRequest())
		}(i)
	}
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	select {
	case <-time.After(100 * time.Millisecond):
	case <-done:
		t.Fatal("admission bypassed the scope transaction lock")
	}
	require.NoError(t, tx.Commit().Error)
	<-done
	require.NoError(t, failures[0])
	require.NoError(t, failures[1])
	require.Equal(t, answers[0].Run.ID, answers[1].Run.ID)
	require.ElementsMatch(t, []string{string(activity.CodeRunStarted), string(activity.CodeRunJoined)}, []string{answers[0].Code, answers[1].Code})
	require.True(t, (answers[0].WorkContext == nil) != (answers[1].WorkContext == nil))
}
