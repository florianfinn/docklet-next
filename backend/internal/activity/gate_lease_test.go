//go:build postgres_integration

// Source: florianfinn/docklet-next/backend/internal/activity
// Revision: d4e7845e461f51d56a63d3a43937679d6449cf3c
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: Gated ownership and cancellation regression tests.

package activity

import (
	"context"
	"errors"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestGatedCancelHandlerFencesSuccessfulCompletion(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	owner := NewActivityService(db, nil)
	started, err := owner.StartGatedActivity(t.Context(), GateRequest{Operation: "deploy", Scope: "stack:example", Type: activitytypes.TypeProjectDeploy})
	require.NoError(t, err)
	require.NotNil(t, started.WorkContext)

	handler := NewHandler(owner, EnvironmentDependencies{})
	response, err := handler.CancelActivity(t.Context(), &CancelActivityInput{EnvironmentID: "0", ActivityID: started.Run.ID, RequestedBy: "operator"})
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusCancelled, response.Body.Data.Status)
	require.Error(t, started.WorkContext.Err())

	_, err = owner.CompleteActivity(started.WorkContext, started.Run.ID, activitytypes.StatusSuccess, "late success", nil)
	require.ErrorIs(t, err, ErrLeaseLost)
	var persisted Activity
	require.NoError(t, db.First(&persisted, "id = ?", started.Run.ID).Error)
	require.Equal(t, activitytypes.StatusCancelled, persisted.Status)
	require.Equal(t, string(activitytypes.CodeActivityCancelled), persisted.MessageCode)
	var successes int64
	require.NoError(t, db.Model(&ActivityMessage{}).Where("activity_id = ? AND code = ?", started.Run.ID, activitytypes.CodeActivitySuccess).Count(&successes).Error)
	require.Zero(t, successes)
}

func TestGatedCompletionRequiresOwnerContext(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	owner := NewActivityService(db, nil)
	started, err := owner.StartGatedActivity(t.Context(), GateRequest{Operation: "deploy", Scope: "stack:example", Type: activitytypes.TypeProjectDeploy})
	require.NoError(t, err)
	_, err = owner.CompleteActivity(t.Context(), started.Run.ID, activitytypes.StatusSuccess, "without lease", nil)
	require.ErrorIs(t, err, ErrLeaseLost)
	require.NoError(t, started.WorkContext.Err())
	require.True(t, owner.hasGateHeartbeatInternal(started.Run.ID))
	finished, err := owner.CompleteActivity(started.WorkContext, started.Run.ID, activitytypes.StatusSuccess, "owned", nil)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, finished.Status)
}

func TestGatedLeaseTakeoverFencesOldOwner(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	owner := NewActivityService(db, nil)
	ticks := make(chan time.Time, 1)
	owner.gateHeartbeatTicks = ticks
	other := NewActivityService(db, nil)
	request := GateRequest{Operation: "deploy", Scope: "stack:example", Type: activitytypes.TypeProjectDeploy}
	started, err := owner.StartGatedActivity(t.Context(), request)
	require.NoError(t, err)
	var original Activity
	require.NoError(t, db.First(&original, "id = ?", started.Run.ID).Error)
	require.NotEmpty(t, original.GateOwner)

	conflict := request
	conflict.Operation = "remove"
	_, err = other.StartGatedActivity(t.Context(), conflict)
	var busy *GateConflict
	require.ErrorAs(t, err, &busy)

	old := time.Now().Add(-3 * time.Minute)
	require.NoError(t, db.Model(&Activity{}).Where("id = ?", started.Run.ID).
		Updates(map[string]any{"started_at": old, "updated_at": old}).Error)
	swept, err := other.FailAbandonedActivities(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, swept)
	var fenced Activity
	require.NoError(t, db.First(&fenced, "id = ?", started.Run.ID).Error)
	require.NotEqual(t, original.GateOwner, fenced.GateOwner)
	require.Equal(t, activitytypes.StatusFailed, fenced.Status)

	takeover, err := other.StartGatedActivity(t.Context(), conflict)
	require.NoError(t, err)
	require.Equal(t, string(activitytypes.CodeRunStarted), takeover.Code)
	require.NotEqual(t, started.Run.ID, takeover.Run.ID)
	var active int64
	require.NoError(t, db.Model(&Activity{}).Where("gate_scope = ? AND status IN ?", request.Scope, []activitytypes.Status{activitytypes.StatusQueued, activitytypes.StatusRunning}).Count(&active).Error)
	require.EqualValues(t, 1, active)

	require.ErrorIs(t, owner.heartbeatGatedActivityInternal(t.Context(), started.Run.ID, original.GateOwner), ErrLeaseLost)
	ticks <- time.Now()
	require.Eventually(t, func() bool { return errors.Is(context.Cause(started.WorkContext), ErrLeaseLost) }, time.Second, 10*time.Millisecond)
	_, err = owner.CompleteActivity(context.WithoutCancel(started.WorkContext), started.Run.ID, activitytypes.StatusSuccess, "late success", nil)
	require.ErrorIs(t, err, ErrLeaseLost)
	_, err = owner.UpdateActivity(context.WithoutCancel(started.WorkContext), started.Run.ID, UpdateActivityRequest{LatestMessage: new("late")})
	require.ErrorIs(t, err, ErrLeaseLost)
	_, err = owner.AppendMessage(context.WithoutCancel(started.WorkContext), started.Run.ID, AppendActivityMessageRequest{Message: "late"})
	require.ErrorIs(t, err, ErrActivityNotCancelable)
	_, err = other.CompleteActivity(takeover.WorkContext, takeover.Run.ID, activitytypes.StatusSuccess, "done", nil)
	require.NoError(t, err)
}

func TestGatedHeartbeatUsesDatabaseClockAcrossSkewedServices(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	owner := NewActivityService(db, nil)
	owner.clock = func() time.Time { return time.Now().Add(10 * time.Minute) }
	ticks := make(chan time.Time, 1)
	owner.gateHeartbeatTicks = ticks
	sweeper := NewActivityService(db, nil)
	sweeper.clock = func() time.Time { return time.Now().Add(10 * time.Minute) }
	request := GateRequest{Operation: "deploy", Scope: "stack:example", Type: activitytypes.TypeProjectDeploy}
	started, err := owner.StartGatedActivity(t.Context(), request)
	require.NoError(t, err)
	var databaseNow time.Time
	require.NoError(t, db.Raw("SELECT NOW()").Scan(&databaseNow).Error)
	require.WithinDuration(t, databaseNow, started.Run.StartedAt, time.Second)
	old := time.Now().Add(-10 * time.Minute)
	require.NoError(t, db.Model(&Activity{}).Where("id = ?", started.Run.ID).
		Updates(map[string]any{"started_at": old, "updated_at": old}).Error)
	ticks <- time.Now()
	require.Eventually(t, func() bool {
		var current Activity
		return db.First(&current, "id = ?", started.Run.ID).Error == nil && current.UpdatedAt.After(old)
	}, time.Second, 10*time.Millisecond)
	swept, err := sweeper.FailAbandonedActivities(t.Context())
	require.NoError(t, err)
	require.Zero(t, swept)
	_, err = sweeper.StartGatedActivity(t.Context(), GateRequest{Operation: "remove", Scope: request.Scope, Type: request.Type})
	var busy *GateConflict
	require.ErrorAs(t, err, &busy)
	_, err = owner.CompleteActivity(started.WorkContext, started.Run.ID, activitytypes.StatusSuccess, "done", nil)
	require.NoError(t, err)
}
