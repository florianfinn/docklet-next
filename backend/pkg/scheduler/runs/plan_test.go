// Source: docklet-next/issues/626
// Revision: 6dc802124ce20a9c53693493ca66a85d1820ef19
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original M41 coverage for persisted plans and predecessor deferral.

package runs

import (
	"testing"
	"time"

	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
)

func TestSubmitRecordsTheActivePredecessorAsDeferral(t *testing.T) {
	q := newResolutionQueueInternal(t)
	first, err := q.Submit(t.Context(), st.Request{JobID: "deliver", Trigger: "manual"})
	require.NoError(t, err)
	require.Empty(t, first.DeferredBy)

	second, err := q.Submit(t.Context(), st.Request{JobID: "deliver", Trigger: "manual"})
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, first.ID, second.DeferredBy)

	stored, err := q.Get(t.Context(), "0", "deliver", second.ID)
	require.NoError(t, err)
	require.Equal(t, first.ID, stored.DeferredBy)
}

func TestSubmitAfterTheLastRunFinishedHasNoDeferral(t *testing.T) {
	q := newResolutionQueueInternal(t)
	first, err := q.Submit(t.Context(), st.Request{JobID: "deliver", Trigger: "manual"})
	require.NoError(t, err)
	require.NoError(t, q.UpdateRun(t.Context(), first, func(current *st.Run) error {
		now := time.Now().UTC()
		current.Status, current.FinishedAt, current.UpdatedAt = st.Succeeded, &now, now
		return nil
	}))
	second, err := q.Submit(t.Context(), st.Request{JobID: "deliver", Trigger: "manual"})
	require.NoError(t, err)
	require.Empty(t, second.DeferredBy)
}

func TestPlanPersistsUnderTheExecutionFenceAndSurvivesSettlement(t *testing.T) {
	q := newResolutionQueueInternal(t)
	run, err := q.Submit(t.Context(), st.Request{JobID: "deliver", Trigger: "manual"})
	require.NoError(t, err)
	require.NoError(t, q.UpdateRun(t.Context(), run, func(current *st.Run) error {
		current.Status, current.Owner, current.AttemptCount = st.Running, "example-owner/1", 1
		current.UpdatedAt = time.Now().UTC()
		*(&run) = *current
		return nil
	}))
	run.Owner = "example-owner/1"

	stranger := run
	stranger.Owner = "other-owner/1"
	require.ErrorIs(t, q.Plan(t.Context(), stranger, []string{"x"}, true), ErrRunConflict)

	require.NoError(t, q.Plan(t.Context(), run, []string{"a", "b"}, true))
	require.NoError(t, q.Progress(t.Context(), run, st.TargetOutcome{ID: "a", Status: st.Succeeded}))
	stored, err := q.Get(t.Context(), "0", "deliver", run.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, stored.Outcome.PlannedIDs)
	require.True(t, stored.Outcome.PlannedKnown)

	// The job's returned outcome carries targets but no plan; settlement keeps the persisted plan.
	q.persistOutcomeInternal(t.Context(), run, st.Outcome{Status: st.Partial, Targets: []st.TargetOutcome{{ID: "b", Status: st.Failed}}})
	settled, err := q.Get(t.Context(), "0", "deliver", run.ID)
	require.NoError(t, err)
	require.Equal(t, st.Partial, settled.Status)
	require.Equal(t, []string{"a", "b"}, settled.Outcome.PlannedIDs)
	counts, ok := settled.Outcome.Counts()
	require.True(t, ok)
	require.Equal(t, st.BulkCounts{Planned: 2, PlannedKnown: true, Completed: 2, Succeeded: 1, Failed: 1}, counts)
}

func TestCountsIgnoreTargetsOutsideTheCurrentPlan(t *testing.T) {
	// A retry keeps the earlier attempt's targets; a removed target must not count against the new plan.
	outcome := st.Outcome{PlannedIDs: []string{"kept", "new"}, PlannedKnown: true, Targets: []st.TargetOutcome{
		{ID: "removed", Status: st.Failed}, {ID: "kept", Status: st.Succeeded, UpdateAvailable: true},
	}}
	counts, ok := outcome.Counts()
	require.True(t, ok)
	require.Equal(t, st.BulkCounts{Planned: 2, PlannedKnown: true, Completed: 1, Succeeded: 1, Updates: 1}, counts)

	_, legacy := st.Outcome{Targets: []st.TargetOutcome{{ID: "a", Status: st.Succeeded}}}.Counts()
	require.False(t, legacy)
}

func TestSubmitDeferralFollowsTheDispatchRule(t *testing.T) {
	future, past := time.Now().Add(time.Hour).UTC(), time.Now().Add(-time.Minute).UTC()
	for _, tc := range []struct {
		name     string
		status   st.RunStatus
		next     *time.Time
		deferred bool
	}{
		{"future retry is skipped by dispatch", st.Retrying, &future, false},
		{"future waiting run is skipped by dispatch", st.Waiting, &future, false},
		{"due retry runs first", st.Retrying, &past, true},
		{"queued predecessor runs first", st.Queued, nil, true},
		{"needs attention does not block", st.NeedsAttention, nil, false},
		{"finished run does not block", st.Failed, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := newResolutionQueueInternal(t)
			first, err := q.Submit(t.Context(), st.Request{JobID: "deliver", Trigger: "manual"})
			require.NoError(t, err)
			require.NoError(t, q.UpdateRun(t.Context(), first, func(current *st.Run) error {
				current.Status, current.NextAttempt, current.AttemptCount, current.UpdatedAt = tc.status, tc.next, 1, time.Now().UTC()
				if tc.status.Terminal() {
					current.FinishedAt = new(time.Now().UTC())
				}
				return nil
			}))
			second, err := q.Submit(t.Context(), st.Request{JobID: "deliver", Trigger: "manual"})
			require.NoError(t, err)
			require.Equal(t, tc.deferred, second.DeferredBy == first.ID)
			if !tc.deferred {
				require.Empty(t, second.DeferredBy)
			}
		})
	}
}
