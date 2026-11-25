// Source: docklet-next/issues/626
// Revision: 6dc802124ce20a9c53693493ca66a85d1820ef19
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original M41 coverage for job activity waiting phases and bulk counts.

package activity

import (
	"encoding/json/v2"
	"fmt"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
)

func exampleJobRun(status scheduler.RunStatus) scheduler.Run {
	at := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	return scheduler.Run{ID: "example-run", ActivityID: "example-activity", JobID: "update-check-host", EnvironmentID: "example-host", Trigger: "manual", Status: status, CreatedAt: at, UpdatedAt: at}
}

func TestImmediateJobRunHasNoWaitingPhase(t *testing.T) {
	model := jobActivityInternal(exampleJobRun(scheduler.Queued), "Update check")
	require.Empty(t, model.Step)
	trace := stepTraceInternal([]ActivityMessage{{ID: "1", Step: model.Step}, {ID: "2", Step: "running"}}, &model)
	require.Len(t, trace, 1)
	require.Equal(t, "running", trace[0].Name)
}

func TestDeferredJobRunShowsItsWaitingPhase(t *testing.T) {
	run := exampleJobRun(scheduler.Queued)
	run.DeferredBy = "example-predecessor"
	model := jobActivityInternal(run, "Update check")
	require.Equal(t, "queued", model.Step)
}

func TestJobActivityCarriesBulkCountsThroughStorage(t *testing.T) {
	run := exampleJobRun(scheduler.Running)
	run.Outcome = scheduler.Outcome{PlannedIDs: []string{"a", "b", "c"}, Targets: []scheduler.TargetOutcome{
		{ID: "a", Status: scheduler.Succeeded, UpdateAvailable: true}, {ID: "b", Status: scheduler.Skipped},
	}}
	model := jobActivityInternal(run, "Update check")
	want := &activitytypes.BulkProgress{Unit: "target", Planned: 3, Completed: 2, Succeeded: 1, Skipped: 1, Updates: 1}
	require.Equal(t, want, activityToDTOInternal(&model).Bulk)

	// Stored metadata comes back as generic JSON; the DTO reads the same counts from it.
	encoded, err := json.Marshal(model.Metadata)
	require.NoError(t, err)
	var stored database.JSON
	require.NoError(t, json.Unmarshal(encoded, &stored))
	model.Metadata = stored
	require.Equal(t, want, activityToDTOInternal(&model).Bulk)
}

func TestLegacyJobActivityHasNoBulkCounts(t *testing.T) {
	run := exampleJobRun(scheduler.Partial)
	run.Outcome = scheduler.Outcome{Targets: []scheduler.TargetOutcome{{ID: "a", Status: scheduler.Failed}}}
	model := jobActivityInternal(run, "Update check")
	require.Nil(t, activityToDTOInternal(&model).Bulk)
}

func TestCountOnlyJobUpdatesAddNoMessages(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	service := NewActivityService(db, nil)
	run := exampleJobRun(scheduler.Running)
	run.EnvironmentID = "0"
	run.Outcome = scheduler.Outcome{PlannedIDs: make([]string, 200), PlannedKnown: true}
	for i := range run.Outcome.PlannedIDs {
		run.Outcome.PlannedIDs[i] = fmt.Sprintf("target-%03d", i)
	}
	require.NoError(t, service.SyncJobRun(t.Context(), run, "Update check"))
	for i := range 200 {
		run.UpdatedAt = run.UpdatedAt.Add(time.Millisecond)
		run.Outcome.Targets = append(run.Outcome.Targets, scheduler.TargetOutcome{ID: run.Outcome.PlannedIDs[i], Status: scheduler.Succeeded})
		require.NoError(t, service.SyncJobRun(t.Context(), run, "Update check"))
	}
	var messages int64
	require.NoError(t, db.Model(&ActivityMessage{}).Where("activity_id = ?", run.ActivityID).Count(&messages).Error)
	require.EqualValues(t, 1, messages)
	detail, err := service.GetActivityDetail(t.Context(), "0", run.ActivityID, 10)
	require.NoError(t, err)
	require.Equal(t, 200, detail.Activity.Bulk.Completed)

	run.Status, run.UpdatedAt = scheduler.Succeeded, run.UpdatedAt.Add(time.Second)
	run.FinishedAt = new(run.UpdatedAt)
	require.NoError(t, service.SyncJobRun(t.Context(), run, "Update check"))
	require.NoError(t, db.Model(&ActivityMessage{}).Where("activity_id = ?", run.ActivityID).Count(&messages).Error)
	require.EqualValues(t, 2, messages)
}
