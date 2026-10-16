// Source: florianfinn/docklet-next/backend/internal/activity
// Revision: 65c9e0dceb6f994ac884904e2a6b7348f75580fa
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: SQLite run-core regression matrix.

package activity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
)

func TestCompletionEffectOnceAfterRetention(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	ctx := t.Context()
	old, err := svc.StartActivity(ctx, StartActivityRequest{Type: activity.TypeProjectDeploy})
	require.NoError(t, err)
	require.NoError(t, db.Model(&Activity{}).Where("id = ?", old.ID).Update("started_at", time.Now().Add(-time.Hour)).Error)
	for i := range 10 {
		finished, startErr := svc.StartActivity(ctx, StartActivityRequest{Type: activity.TypeProjectDeploy, ResourceID: new(fmt.Sprintf("new-%d", i))})
		require.NoError(t, startErr)
		_, finishErr := svc.CompleteActivity(ctx, finished.ID, activity.StatusSuccess, "done", nil)
		require.NoError(t, finishErr)
	}
	deleted, err := svc.PruneHistory(ctx, 10)
	require.NoError(t, err)
	require.Zero(t, deleted)
	first, err := svc.CompleteActivity(ctx, old.ID, activity.StatusSuccess, "completed once", nil, "deploy")
	require.NoError(t, err)
	second, err := svc.CompleteActivity(ctx, old.ID, activity.StatusFailed, "must not appear", nil)
	require.NoError(t, err)
	require.Equal(t, first.Status, second.Status)
	require.Equal(t, first.EndedAt, second.EndedAt)
	var effects int64
	require.NoError(t, db.Model(&ActivityMessage{}).Where("activity_id = ? AND code = ?", old.ID, "activity.success").Count(&effects).Error)
	require.EqualValues(t, 1, effects)
	deleted, err = svc.PruneHistory(ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
}

func TestRunGateJoinConflictAndFreshnessBoundary(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	ctx := t.Context()
	request := GateRequest{Operation: "check-updates", Scope: "stack:example", Type: activity.TypeImageUpdateCheck, Check: true}
	started, err := svc.StartGatedActivity(ctx, request)
	require.NoError(t, err)
	require.Equal(t, "run.started", started.Code)
	joined, err := svc.StartGatedActivity(ctx, request)
	require.NoError(t, err)
	require.Equal(t, "run.joined", joined.Code)
	require.Equal(t, started.Run.ID, joined.Run.ID)
	conflict := request
	conflict.Operation = "deploy"
	_, err = svc.StartGatedActivity(ctx, conflict)
	var busy *GateConflict
	require.True(t, errors.As(err, &busy))
	require.Equal(t, "run.busy", busy.Code)
	require.Equal(t, started.Run.ID, busy.RunID)
	_, err = svc.CompleteActivity(started.WorkContext, started.Run.ID, activity.StatusSuccess, "checked", nil)
	require.NoError(t, err)
	fresh, err := svc.StartGatedActivity(ctx, request)
	require.NoError(t, err)
	require.Equal(t, "run.fresh", fresh.Code)
	require.Equal(t, started.Run.ID, fresh.Run.ID)
	recheck := request
	recheck.Recheck = true
	rechecked, err := svc.StartGatedActivity(ctx, recheck)
	require.NoError(t, err)
	require.Equal(t, "run.started", rechecked.Code)
	require.NotEqual(t, started.Run.ID, rechecked.Run.ID)
	_, err = svc.CompleteActivity(rechecked.WorkContext, rechecked.Run.ID, activity.StatusSuccess, "checked again", nil)
	require.NoError(t, err)
	require.NoError(t, db.Model(&Activity{}).Where("id IN ?", []string{started.Run.ID, rechecked.Run.ID}).Update("ended_at", time.Now().UTC().Add(-30*time.Second)).Error)
	atBoundary, err := svc.StartGatedActivity(ctx, request)
	require.NoError(t, err)
	require.Equal(t, "run.started", atBoundary.Code)
	require.NotEqual(t, started.Run.ID, atBoundary.Run.ID)
}

func TestActivityWritesRejectUnknownMessageCodes(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	_, err := svc.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy, MessageCode: "unlisted.code"})
	require.Error(t, err)
	run, err := svc.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy})
	require.NoError(t, err)
	invalid := "unlisted.code"
	_, err = svc.UpdateActivity(t.Context(), run.ID, UpdateActivityRequest{MessageCode: &invalid})
	require.Error(t, err)
	_, err = svc.AppendMessages(t.Context(), run.ID, []AppendActivityMessageRequest{{Message: "valid"}, {Message: "invalid", Code: invalid}})
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&ActivityMessage{}).Where("activity_id = ?", run.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestQueuedToRunningSetsRunningCode(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	run, err := svc.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy, Queue: true, DeferSlot: true})
	require.NoError(t, err)
	require.Equal(t, string(activity.CodeActivityQueued), run.MessageCode)
	require.NoError(t, svc.AwaitActivitySlot(t.Context(), run.ID, "0"))
	var running Activity
	require.NoError(t, db.First(&running, "id = ?", run.ID).Error)
	require.Equal(t, activity.StatusRunning, running.Status)
	require.Equal(t, string(activity.CodeActivityRunning), running.MessageCode)
	_, err = svc.CompleteActivity(t.Context(), run.ID, activity.StatusSuccess, "done", nil)
	require.NoError(t, err)
}

func TestRunDetailBoundsDistinctStepTransitions(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	run, err := svc.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy})
	require.NoError(t, err)
	messages := make([]AppendActivityMessageRequest, 0, 600)
	for i := range 300 {
		step := fmt.Sprintf("step-%03d", i)
		messages = append(messages, AppendActivityMessageRequest{Message: "begin", Step: step}, AppendActivityMessageRequest{Message: "progress", Step: step})
	}
	_, err = svc.AppendMessages(t.Context(), run.ID, messages)
	require.NoError(t, err)
	detail, err := svc.GetActivityDetail(t.Context(), "0", run.ID, 10)
	require.NoError(t, err)
	require.Len(t, detail.StepTrace, maxStepTransitions)
	require.Equal(t, "step-044", detail.StepTrace[0].Name)
	require.Equal(t, "step-299", detail.StepTrace[len(detail.StepTrace)-1].Name)
}

func TestRunGateSingleConnectionPoolBlocksAnotherService(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	svc := NewActivityService(db, nil)
	type outcome struct {
		answer GateAnswer
		err    error
	}
	result := make(chan outcome, 1)
	go func() {
		answer, err := svc.StartGatedActivity(context.Background(), GateRequest{
			Operation: "deploy", Scope: "stack:example", Type: activity.TypeProjectDeploy,
		})
		result <- outcome{answer, err}
	}()
	select {
	case got := <-result:
		require.Failf(t, "occupied connection pool did not block", "start returned: %v", got.err)
	case <-time.After(100 * time.Millisecond):
	}
	require.NoError(t, tx.Commit().Error)
	select {
	case got := <-result:
		require.NoError(t, got.err)
		_, err := svc.CompleteActivity(got.answer.WorkContext, got.answer.Run.ID, activity.StatusSuccess, "done", nil)
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("start remained blocked after pool connection released")
	}
}

func TestGatedRunSurvivesThreeMinuteUngatedSweep(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	owner := NewActivityService(db, nil)
	sweeper := NewActivityService(db, nil)
	request := GateRequest{Operation: "deploy", Scope: "stack:example", Type: activity.TypeProjectDeploy}
	started, err := owner.StartGatedActivity(t.Context(), request)
	require.NoError(t, err)
	require.True(t, owner.isTrackedInternal(started.Run.ID))
	old := time.Now().Add(-3 * time.Minute)
	require.NoError(t, db.Model(&Activity{}).Where("id = ?", started.Run.ID).
		Update("started_at", old).Error)
	swept, err := sweeper.FailAbandonedActivities(t.Context())
	require.NoError(t, err)
	require.Zero(t, swept)
	conflict := request
	conflict.Operation = "remove"
	_, err = sweeper.StartGatedActivity(t.Context(), conflict)
	var busy *GateConflict
	require.ErrorAs(t, err, &busy)
	require.Equal(t, "run.busy", busy.Code)
	finished, err := owner.CompleteActivity(started.WorkContext, started.Run.ID, activity.StatusSuccess, "finished", nil)
	require.NoError(t, err)
	require.Equal(t, activity.StatusSuccess, finished.Status)
}

func TestFailedCompletionWriteReleasesRegistrationAndSlot(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	run, err := svc.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy, Queue: true})
	require.NoError(t, err)
	_ = svc.Track(t.Context(), run.ID)
	svc.slotMu.Lock()
	_, held := svc.slotReleases[run.ID]
	svc.slotMu.Unlock()
	require.True(t, held)
	require.NoError(t, db.Migrator().DropTable(&ActivityMessage{}))
	_, err = svc.CompleteActivity(t.Context(), run.ID, activity.StatusSuccess, "final", nil)
	require.Error(t, err)
	require.False(t, svc.isTrackedInternal(run.ID))
	svc.slotMu.Lock()
	_, held = svc.slotReleases[run.ID]
	svc.slotMu.Unlock()
	require.False(t, held)
}

func TestEveryTerminalPathSetsMatchingMessageCode(t *testing.T) {
	cases := []struct {
		name string
		want activity.Status
		run  func(*testing.T, *ActivityService, *database.DB) string
	}{
		{"complete success", activity.StatusSuccess, func(t *testing.T, s *ActivityService, _ *database.DB) string {
			run, err := s.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy})
			require.NoError(t, err)
			_, err = s.CompleteActivity(t.Context(), run.ID, activity.StatusSuccess, "done", nil)
			require.NoError(t, err)
			return run.ID
		}},
		{"complete failed", activity.StatusFailed, func(t *testing.T, s *ActivityService, _ *database.DB) string {
			run, err := s.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy})
			require.NoError(t, err)
			_, err = s.CompleteActivity(t.Context(), run.ID, activity.StatusFailed, "failed", nil)
			require.NoError(t, err)
			return run.ID
		}},
		{"complete cancelled", activity.StatusCancelled, func(t *testing.T, s *ActivityService, _ *database.DB) string {
			run, err := s.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy})
			require.NoError(t, err)
			_, err = s.CompleteActivity(t.Context(), run.ID, activity.StatusCancelled, "cancelled", nil)
			require.NoError(t, err)
			return run.ID
		}},
		{"cancel fallback", activity.StatusCancelled, func(t *testing.T, s *ActivityService, _ *database.DB) string {
			run, err := s.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy})
			require.NoError(t, err)
			_, err = s.CancelActivity(t.Context(), "0", run.ID, "operator")
			require.NoError(t, err)
			_, err = s.CompleteActivity(t.Context(), run.ID, activity.StatusSuccess, "done", nil)
			require.NoError(t, err)
			return run.ID
		}},
		{"abandoned sweep", activity.StatusFailed, func(t *testing.T, s *ActivityService, db *database.DB) string {
			run, err := s.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy})
			require.NoError(t, err)
			require.NoError(t, db.Model(&Activity{}).Where("id = ?", run.ID).Update("started_at", time.Now().Add(-3*time.Minute)).Error)
			count, err := s.FailAbandonedActivities(t.Context())
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			return run.ID
		}},
		{"orphaned queued", activity.StatusFailed, func(t *testing.T, s *ActivityService, _ *database.DB) string {
			run, err := s.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy, Queue: true, DeferSlot: true})
			require.NoError(t, err)
			count, err := s.ResolveOrphanedQueuedActivities(t.Context())
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			return run.ID
		}},
		{"stale auto update failed", activity.StatusFailed, func(t *testing.T, s *ActivityService, _ *database.DB) string {
			run, err := s.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeAutoUpdate})
			require.NoError(t, err)
			count, err := s.ResolveStaleAutoUpdateActivities(t.Context())
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			return run.ID
		}},
		{"stale auto update success", activity.StatusSuccess, func(t *testing.T, s *ActivityService, _ *database.DB) string {
			run, err := s.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeAutoUpdate})
			require.NoError(t, err)
			require.NoError(t, s.PatchActivityMetadata(t.Context(), run.ID, database.JSON{"selfUpdateTriggered": true}))
			count, err := s.ResolveStaleAutoUpdateActivities(t.Context())
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			return run.ID
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupActivityServiceTestDBInternal(t)
			id := tc.run(t, NewActivityService(db, nil), db)
			var row Activity
			require.NoError(t, db.First(&row, "id = ?", id).Error)
			require.Equal(t, tc.want, row.Status)
			require.Equal(t, string(activityCodeInternal(tc.want)), row.MessageCode)
		})
	}
	t.Run("gate start", func(t *testing.T) {
		svc := NewActivityService(setupActivityServiceTestDBInternal(t), nil)
		answer, err := svc.StartGatedActivity(t.Context(), GateRequest{Operation: "deploy", Scope: "stack:example", Type: activity.TypeProjectDeploy})
		require.NoError(t, err)
		require.Equal(t, activity.StatusRunning, answer.Run.Status)
		require.Equal(t, "run.started", answer.Run.MessageCode)
		_, err = svc.CompleteActivity(answer.WorkContext, answer.Run.ID, activity.StatusSuccess, "done", nil)
		require.NoError(t, err)
	})
}

func TestRunGateConcurrentStartsAcrossServices(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	services := [2]*ActivityService{NewActivityService(db, nil), NewActivityService(db, nil)}
	request := GateRequest{Operation: "deploy", Scope: "stack:example", Type: activity.TypeProjectDeploy}
	var answers [2]GateAnswer
	var errs [2]error
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := range services {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			answers[i], errs[i] = services[i].StartGatedActivity(t.Context(), request)
		}(i)
	}
	close(start)
	workers.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, answers[0].Run.ID, answers[1].Run.ID)
	require.ElementsMatch(t, []string{"run.started", "run.joined"}, []string{answers[0].Code, answers[1].Code})
}

func TestConcurrentCompletionHasOneEffect(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	created, err := NewActivityService(db, nil).StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy})
	require.NoError(t, err)
	services := [2]*ActivityService{NewActivityService(db, nil), NewActivityService(db, nil)}
	var outcomes [2]*activity.Activity
	var errs [2]error
	var workers sync.WaitGroup
	for i := range services {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			outcomes[i], errs[i] = services[i].CompleteActivity(t.Context(), created.ID, activity.StatusSuccess, "once", nil)
		}(i)
	}
	workers.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, outcomes[0].EndedAt, outcomes[1].EndedAt)
	var messages int64
	require.NoError(t, db.Model(&ActivityMessage{}).Where("activity_id = ? AND code = ?", created.ID, "activity.success").Count(&messages).Error)
	require.EqualValues(t, 1, messages)
}

func TestRunningRunsSurviveCountRetentionAndReloadDoesNotRepeatCompletion(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	ctx := t.Context()
	var running []string
	for range 3 {
		run, err := svc.StartActivity(ctx, StartActivityRequest{Type: activity.TypeProjectDeploy})
		require.NoError(t, err)
		running = append(running, run.ID)
	}
	for i := range 101 {
		run, err := svc.StartActivity(ctx, StartActivityRequest{Type: activity.TypeProjectDeploy})
		require.NoError(t, err)
		_, err = svc.CompleteActivity(ctx, run.ID, activity.StatusSuccess, "done", nil)
		require.NoError(t, err)
		_ = i
	}
	deleted, err := svc.PruneHistory(ctx, 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	var count int64
	require.NoError(t, db.Model(&Activity{}).Count(&count).Error)
	require.EqualValues(t, 103, count)
	for _, id := range running {
		reloaded := NewActivityService(db, nil)
		_, err := reloaded.CompleteActivity(ctx, id, activity.StatusSuccess, "once", nil)
		require.NoError(t, err)
		_, err = NewActivityService(db, nil).CompleteActivity(ctx, id, activity.StatusSuccess, "again", nil)
		require.NoError(t, err)
		var messages int64
		require.NoError(t, db.Model(&ActivityMessage{}).Where("activity_id = ? AND code = ?", id, "activity.success").Count(&messages).Error)
		require.EqualValues(t, 1, messages)
	}
}

func TestFinishedRetentionLimitPerEnvironment(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	for _, environment := range []string{"0", "remote-example"} {
		run, err := svc.StartActivity(t.Context(), StartActivityRequest{EnvironmentID: environment, Type: activity.TypeProjectDeploy})
		require.NoError(t, err)
		_, err = svc.CompleteActivity(t.Context(), run.ID, activity.StatusSuccess, "done", nil)
		require.NoError(t, err)
	}
	active, err := svc.StartActivity(t.Context(), StartActivityRequest{EnvironmentID: "remote-example", Type: activity.TypeProjectDeploy})
	require.NoError(t, err)
	deleted, err := svc.PruneHistory(t.Context(), 1)
	require.NoError(t, err)
	require.Zero(t, deleted)
	var finished int64
	require.NoError(t, db.Model(&Activity{}).Where("status IN ?", terminalActivityStatusesInternal()).Count(&finished).Error)
	require.EqualValues(t, 2, finished)
	var running int64
	require.NoError(t, db.Model(&Activity{}).Where("id = ?", active.ID).Count(&running).Error)
	require.EqualValues(t, 1, running)
}

func TestActivityRetentionKeepsNewestFinishedRunByEndTime(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	ids := make([]string, 0, 3)
	for i := range 3 {
		run, err := svc.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy})
		require.NoError(t, err)
		_, err = svc.CompleteActivity(t.Context(), run.ID, activity.StatusSuccess, "done", nil)
		require.NoError(t, err)
		ended := time.Now().Add(time.Duration(i-3) * time.Hour)
		require.NoError(t, db.Model(&Activity{}).Where("id = ?", run.ID).Update("ended_at", ended).Error)
		ids = append(ids, run.ID)
	}
	deleted, err := svc.PruneHistory(t.Context(), 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, deleted)
	var kept []string
	require.NoError(t, db.Model(&Activity{}).Pluck("id", &kept).Error)
	require.Equal(t, []string{ids[2]}, kept)
}

func TestRunTraceCodesAndAffectedServices(t *testing.T) {
	svc := NewActivityService(setupActivityServiceTestDBInternal(t), nil)
	run, err := svc.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy, AffectedServices: []string{"web", "api", "web"}})
	require.NoError(t, err)
	_, err = svc.AppendMessage(t.Context(), run.ID, AppendActivityMessageRequest{Message: "Pulling", Code: "deploy.pull", Step: "pull"})
	require.NoError(t, err)
	detail, err := svc.GetActivityDetail(t.Context(), "0", run.ID, 10)
	require.NoError(t, err)
	require.Equal(t, []string{"web", "api"}, detail.Activity.AffectedServices)
	require.Equal(t, "deploy.pull", detail.Activity.MessageCode)
	require.Len(t, detail.StepTrace, 1)
	require.Equal(t, "pull", detail.StepTrace[0].Name)
	require.Equal(t, "deploy.pull", detail.StepTrace[0].Code)
}

func TestStepTraceKeepsEarlyStepsAndDeduplicatesProgress(t *testing.T) {
	svc := NewActivityService(setupActivityServiceTestDBInternal(t), nil)
	run, err := svc.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy})
	require.NoError(t, err)
	_, err = svc.AppendMessage(t.Context(), run.ID, AppendActivityMessageRequest{Message: "prepare", Step: "Prepare"})
	require.NoError(t, err)
	for i := range 510 {
		_, err = svc.AppendMessage(t.Context(), run.ID, AppendActivityMessageRequest{Message: fmt.Sprintf("line %d", i), Step: "Apply"})
		require.NoError(t, err)
	}
	_, err = svc.CompleteActivity(t.Context(), run.ID, activity.StatusSuccess, "done", nil, "Verify")
	require.NoError(t, err)
	detail, err := svc.GetActivityDetail(t.Context(), "0", run.ID, 10)
	require.NoError(t, err)
	require.Len(t, detail.Messages, 10)
	require.Len(t, detail.StepTrace, 3)
	require.Equal(t, []string{"Prepare", "Apply", "Verify"}, []string{detail.StepTrace[0].Name, detail.StepTrace[1].Name, detail.StepTrace[2].Name})
	for _, step := range detail.StepTrace {
		require.NotEmpty(t, step.ID)
		require.NotNil(t, step.EndedAt)
		require.Equal(t, activity.StatusSuccess, step.Status)
	}
}

func TestJobCompletionReplayAfterPruningKeepsOneEffect(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	now := time.Now().UTC()
	run := scheduler.Run{ID: "run-old", ActivityID: "activity-old", JobID: "check", EnvironmentID: "0", Status: scheduler.Succeeded, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Minute)}
	require.NoError(t, svc.SyncJobRun(t.Context(), run, "Check"))
	newer, err := svc.StartActivity(t.Context(), StartActivityRequest{Type: activity.TypeProjectDeploy})
	require.NoError(t, err)
	_, err = svc.CompleteActivity(t.Context(), newer.ID, activity.StatusSuccess, "newer", nil)
	require.NoError(t, err)
	deleted, err := svc.PruneHistory(t.Context(), 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	require.NoError(t, NewActivityService(db, nil).SyncJobRun(t.Context(), run, "Check"))
	var activities int64
	require.NoError(t, db.Model(&Activity{}).Where("id = ?", run.ActivityID).Count(&activities).Error)
	require.Zero(t, activities)
	var receipts int64
	require.NoError(t, db.Model(&ActivitySyncReceipt{}).Where("run_id = ?", run.ID).Count(&receipts).Error)
	require.EqualValues(t, 1, receipts)
}

func TestPruneSyncReceiptsKeepsCoordinatorRunsAndRecentSyncs(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	svc := NewActivityService(db, nil)
	old := time.Now().UTC().Add(-time.Hour)
	recent := time.Now().UTC()
	for _, row := range []ActivitySyncReceipt{
		{RunID: "retained", UpdatedAt: old, SyncedAt: &old},
		{RunID: "retired", UpdatedAt: old, SyncedAt: &old},
		{RunID: "concurrent", UpdatedAt: old, SyncedAt: &recent},
	} {
		require.NoError(t, db.Create(&row).Error)
	}
	deleted, err := svc.PruneSyncReceipts(t.Context(), []string{"retained"})
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	var kept []string
	require.NoError(t, db.Model(&ActivitySyncReceipt{}).Order("run_id").Pluck("run_id", &kept).Error)
	require.Equal(t, []string{"concurrent", "retained"}, kept)
}
