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
	"os"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestGatedCancelHandlerKeepsScopeUntilOwnerCompletes(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	owner := NewActivityService(db, nil)
	started, err := owner.StartGatedActivity(t.Context(), GateRequest{Operation: "deploy", Scope: "stack:example", Type: activitytypes.TypeProjectDeploy})
	require.NoError(t, err)
	require.NotNil(t, started.WorkContext)

	handler := NewHandler(owner, EnvironmentDependencies{})
	response, err := handler.CancelActivity(t.Context(), &CancelActivityInput{EnvironmentID: "0", ActivityID: started.Run.ID, RequestedBy: "operator"})
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusRunning, response.Body.Data.Status)
	require.Error(t, started.WorkContext.Err())
	var requested Activity
	require.NoError(t, db.First(&requested, "id = ?", started.Run.ID).Error)
	require.NotNil(t, requested.CancelRequestedAt)
	conflict := GateRequest{Operation: "remove", Scope: "stack:example", Type: activitytypes.TypeProjectDeploy}
	_, err = owner.StartGatedActivity(t.Context(), conflict)
	var busy *GateConflict
	require.ErrorAs(t, err, &busy)
	require.Equal(t, string(activitytypes.CodeRunBusy), busy.Code)

	finished, err := owner.CompleteActivity(started.WorkContext, started.Run.ID, activitytypes.StatusSuccess, "late success", nil)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusCancelled, finished.Status)
	var persisted Activity
	require.NoError(t, db.First(&persisted, "id = ?", started.Run.ID).Error)
	require.Equal(t, activitytypes.StatusCancelled, persisted.Status)
	require.Equal(t, string(activitytypes.CodeActivityCancelled), persisted.MessageCode)
	var successes int64
	require.NoError(t, db.Model(&ActivityMessage{}).Where("activity_id = ? AND code = ?", started.Run.ID, activitytypes.CodeActivitySuccess).Count(&successes).Error)
	require.Zero(t, successes)
	admitted, err := owner.StartGatedActivity(t.Context(), conflict)
	require.NoError(t, err)
	require.Equal(t, string(activitytypes.CodeRunStarted), admitted.Code)
	_, err = owner.CompleteActivity(admitted.WorkContext, admitted.Run.ID, activitytypes.StatusSuccess, "done", nil)
	require.NoError(t, err)
}

func TestGatedRemoteCancelKeepsScopeUntilOwnerCompletes(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	owner := NewActivityService(db, nil)
	ticks := make(chan time.Time, 1)
	owner.gateHeartbeatTicks = ticks
	other := NewActivityService(db, nil)
	started, err := owner.StartGatedActivity(t.Context(), GateRequest{Operation: "deploy", Scope: "stack:example", Type: activitytypes.TypeProjectDeploy})
	require.NoError(t, err)
	response, err := other.CancelActivity(t.Context(), "0", started.Run.ID, "operator")
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusRunning, response.Status)
	require.NoError(t, started.WorkContext.Err())
	conflict := GateRequest{Operation: "remove", Scope: "stack:example", Type: activitytypes.TypeProjectDeploy}
	_, err = other.StartGatedActivity(t.Context(), conflict)
	var busy *GateConflict
	require.ErrorAs(t, err, &busy)
	ticks <- time.Now()
	require.Eventually(t, func() bool { return started.WorkContext.Err() != nil }, time.Second, 10*time.Millisecond)
	_, err = other.StartGatedActivity(t.Context(), conflict)
	require.ErrorAs(t, err, &busy)
	finished, err := owner.CompleteActivity(started.WorkContext, started.Run.ID, activitytypes.StatusSuccess, "late success", nil)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusCancelled, finished.Status)
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
	require.ErrorIs(t, owner.PatchActivityMetadata(t.Context(), started.Run.ID, map[string]any{"phase": "unowned"}), ErrLeaseLost)
	_, err = owner.UpdateActivity(started.WorkContext, started.Run.ID, UpdateActivityRequest{Status: activitytypes.StatusSuccess})
	require.ErrorIs(t, err, ErrLeaseLost)
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
		Updates(map[string]any{"started_at": old, "gate_lease_at": old}).Error)
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
	owner.clock = func() time.Time { return time.Now().Add(-10 * time.Minute) }
	ticks := make(chan time.Time, 1)
	owner.gateHeartbeatTicks = ticks
	sweeper := NewActivityService(db, nil)
	sweeper.clock = func() time.Time { return time.Now().Add(10 * time.Minute) }
	request := GateRequest{Operation: "deploy", Scope: "stack:example", Type: activitytypes.TypeProjectDeploy}
	started, err := owner.StartGatedActivity(t.Context(), request)
	require.NoError(t, err)
	var databaseNow time.Time
	require.NoError(t, db.Raw("SELECT NOW()").Scan(&databaseNow).Error)
	if os.Getenv("DOCKLET_TEST_EXPECT_DB_SKEW") == "1" {
		require.Greater(t, databaseNow.Sub(time.Now()), 2*time.Minute)
	}
	require.WithinDuration(t, databaseNow, started.Run.StartedAt, time.Second)
	old := time.Now().Add(-10 * time.Minute)
	require.NoError(t, db.Model(&Activity{}).Where("id = ?", started.Run.ID).
		Updates(map[string]any{"started_at": old, "gate_lease_at": old}).Error)
	ticks <- time.Now()
	require.Eventually(t, func() bool {
		var current Activity
		return db.First(&current, "id = ?", started.Run.ID).Error == nil && current.GateLeaseAt != nil && current.GateLeaseAt.After(old)
	}, time.Second, 10*time.Millisecond)
	_, err = owner.UpdateActivity(started.WorkContext, started.Run.ID, UpdateActivityRequest{LatestMessage: new("progress")})
	require.NoError(t, err)
	_, err = owner.AppendMessage(started.WorkContext, started.Run.ID, AppendActivityMessageRequest{Message: "working"})
	require.NoError(t, err)
	require.NoError(t, owner.PatchActivityMetadata(started.WorkContext, started.Run.ID, map[string]any{"phase": "working"}))
	var afterProgress Activity
	require.NoError(t, db.First(&afterProgress, "id = ?", started.Run.ID).Error)
	require.WithinDuration(t, databaseNow, *afterProgress.GateLeaseAt, 2*time.Second)
	swept, err := sweeper.FailAbandonedActivities(t.Context())
	require.NoError(t, err)
	require.Zero(t, swept)
	_, err = sweeper.StartGatedActivity(t.Context(), GateRequest{Operation: "remove", Scope: request.Scope, Type: request.Type})
	var busy *GateConflict
	require.ErrorAs(t, err, &busy)
	finished, err := owner.CompleteActivity(started.WorkContext, started.Run.ID, activitytypes.StatusSuccess, "done", nil)
	require.NoError(t, err)
	require.WithinDuration(t, databaseNow, finished.EndedAt.UTC(), 2*time.Second)
	request.Check = true
	fresh, err := sweeper.StartGatedActivity(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, string(activitytypes.CodeRunFresh), fresh.Code)
}

func TestStartupRepairsOnlyExpiredGatedLeases(t *testing.T) {
	for _, tc := range []struct {
		name      string
		typeID    activitytypes.Type
		queued    bool
		triggered bool
		want      activitytypes.Status
	}{
		{name: "queued", typeID: activitytypes.TypeProjectDeploy, queued: true, want: activitytypes.StatusFailed},
		{name: "auto-update", typeID: activitytypes.TypeAutoUpdate, want: activitytypes.StatusFailed},
		{name: "triggered auto-update", typeID: activitytypes.TypeAutoUpdate, triggered: true, want: activitytypes.StatusSuccess},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupActivityServiceTestDBInternal(t)
			owner := NewActivityService(db, nil)
			owner.gateHeartbeatTicks = make(chan time.Time)
			repair := NewActivityService(db, nil)
			started, err := owner.StartGatedActivity(t.Context(), GateRequest{Operation: "update", Scope: "stack:example", Type: tc.typeID})
			require.NoError(t, err)
			if tc.queued {
				require.NoError(t, db.Model(&Activity{}).Where("id = ?", started.Run.ID).Update("status", activitytypes.StatusQueued).Error)
			}
			if tc.triggered {
				require.NoError(t, owner.PatchActivityMetadata(started.WorkContext, started.Run.ID, map[string]any{"selfUpdateTriggered": true}))
			}
			runRepair := func() (int64, error) {
				if tc.queued {
					return repair.ResolveOrphanedQueuedActivities(t.Context())
				}
				return repair.ResolveStaleAutoUpdateActivities(t.Context())
			}
			count, err := runRepair()
			require.NoError(t, err)
			require.Zero(t, count)
			_, err = repair.StartGatedActivity(t.Context(), GateRequest{Operation: "remove", Scope: "stack:example", Type: tc.typeID})
			var busy *GateConflict
			require.ErrorAs(t, err, &busy)
			require.NoError(t, db.Model(&Activity{}).Where("id = ?", started.Run.ID).Updates(map[string]any{
				"started_at":    time.Now().Add(-3 * time.Minute),
				"gate_lease_at": time.Now().Add(-3 * time.Minute),
			}).Error)
			count, err = runRepair()
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			var repaired Activity
			require.NoError(t, db.First(&repaired, "id = ?", started.Run.ID).Error)
			require.Equal(t, tc.want, repaired.Status)
		})
	}
}
