//go:build postgres_integration

// Source: florianfinn/docklet-next/backend/internal/activity
// Revision: 65c9e0dceb6f994ac884904e2a6b7348f75580fa
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: New PostgreSQL run-core regression matrix.

package activity

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

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
	deleted, err := svc.PruneHistory(ctx, 0, 10)
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
	deleted, err = svc.PruneHistory(ctx, 0, 10)
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
	completed, err := svc.CompleteActivity(ctx, started.Run.ID, activity.StatusSuccess, "checked", nil)
	require.NoError(t, err)
	fresh, err := svc.startGatedActivityAtInternal(ctx, request, completed.EndedAt.Add(CheckFreshness-time.Nanosecond))
	require.NoError(t, err)
	require.Equal(t, "run.fresh", fresh.Code)
	require.Equal(t, started.Run.ID, fresh.Run.ID)
	atBoundary, err := svc.startGatedActivityAtInternal(ctx, request, completed.EndedAt.Add(CheckFreshness))
	require.NoError(t, err)
	require.Equal(t, "run.started", atBoundary.Code)
	require.NotEqual(t, started.Run.ID, atBoundary.Run.ID)
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
	deleted, err := svc.PruneHistory(ctx, 0, 100)
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

func TestFinishedRetentionLimitAcrossEnvironments(t *testing.T) {
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
	deleted, err := svc.PruneHistory(t.Context(), 0, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	var finished int64
	require.NoError(t, db.Model(&Activity{}).Where("status IN ?", terminalActivityStatusesInternal()).Count(&finished).Error)
	require.EqualValues(t, 1, finished)
	var running int64
	require.NoError(t, db.Model(&Activity{}).Where("id = ?", active.ID).Count(&running).Error)
	require.EqualValues(t, 1, running)
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
	require.Equal(t, "pull", detail.StepTrace[0].Step)
	require.Equal(t, "deploy.pull", detail.StepTrace[0].Code)
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
	deleted, err := svc.PruneHistory(t.Context(), 0, 1)
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
