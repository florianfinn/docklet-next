// Source: florianfinn/docklet-next/backend/pkg/scheduler/runs
// Revision: 65c9e0dceb6f994ac884904e2a6b7348f75580fa
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: New completion projection retention regression.

package runs

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
)

type receiptPruningObserverInternal struct {
	activityObserverFakeInternal
	retained []string
}

func (o *receiptPruningObserverInternal) PruneRunActivityReceipts(_ context.Context, retained []string) error {
	o.retained = append([]string(nil), retained...)
	return nil
}

func TestCoordinatorPruneWiresRetainedActivityReceipts(t *testing.T) {
	q := newResolutionQueueInternal(t)
	observer := &receiptPruningObserverInternal{}
	initial, err := q.Submit(t.Context(), st.Request{JobID: "cleanup", Trigger: "scheduled"})
	require.NoError(t, err)
	old := time.Now().Add(-time.Hour)
	require.NoError(t, q.mutateInternal(t.Context(), initial.EnvironmentID, initial.JobID, func(record *st.QueueRecord) error {
		record.Runs = []st.Run{{ID: "retired", Status: st.Succeeded, Trigger: "scheduled", UpdatedAt: old}}
		for i := range 100 {
			record.Runs = append(record.Runs, st.Run{ID: fmt.Sprintf("kept-%03d", i), Status: st.Succeeded, Trigger: "scheduled", UpdatedAt: old.Add(time.Duration(i+1) * time.Minute)})
		}
		return nil
	}))
	q.SetObserver(observer)
	require.NoError(t, q.pruneInternal(t.Context()))
	require.Len(t, observer.retained, 100)
	require.NotContains(t, observer.retained, "retired")
	require.Contains(t, observer.retained, "kept-099")
}

func TestPendingCompletionSurvivesPruningUntilAcknowledged(t *testing.T) {
	now := time.Now().UTC()
	old := st.Run{ID: "old", ActivityID: "activity-old", Status: st.Succeeded, UpdatedAt: now.Add(-time.Hour)}
	state := st.CoordinatorState{Record: st.QueueRecord{Runs: []st.Run{old}}, PendingActivitySync: map[string]time.Time{old.ID: old.UpdatedAt}}
	for i := range 100 {
		state.Record.Runs = append(state.Record.Runs, st.Run{ID: fmt.Sprintf("new-%03d", i), Status: st.Succeeded, UpdatedAt: now.Add(time.Duration(i) * time.Second)})
	}
	previous := slices.Clone(state.Record.Runs)
	require.True(t, pruneRunsInternal(&state.Record, now))
	require.Len(t, state.Record.Runs, 100)
	preserveUnsyncedRunsInternal(&state, previous)
	require.Len(t, state.Record.Runs, 101)
	require.Contains(t, state.PendingActivitySync, old.ID)
	require.Equal(t, old.ID, state.Record.Runs[0].ID)
	delete(state.PendingActivitySync, old.ID)
	require.True(t, pruneRunsInternal(&state.Record, now))
	require.Len(t, state.Record.Runs, 100)
	require.NotContains(t, state.Record.Runs, old)
}

func TestPrunedManualRunKeepsIdempotencyReceipt(t *testing.T) {
	now := time.Now().UTC()
	old := st.Run{ID: "manual-old", Trigger: "manual", Status: st.Succeeded, UpdatedAt: now.Add(-time.Hour)}
	record := st.QueueRecord{Runs: []st.Run{old}}
	for i := range 100 {
		record.Runs = append(record.Runs, st.Run{ID: fmt.Sprintf("new-%03d", i), Status: st.Succeeded, UpdatedAt: now.Add(time.Duration(i) * time.Second)})
	}
	require.True(t, pruneRunsInternal(&record, now))
	require.Len(t, record.Runs, 100)
	require.NotContains(t, record.Runs, old)
	require.Equal(t, old.ID, record.Receipts[old.ID].ID)
	require.False(t, pruneRunsInternal(&record, now))
	require.Equal(t, old.ID, record.Receipts[old.ID].ID)
}

func TestJobRunsPruneAfterSevenDaysWithinCount(t *testing.T) {
	now := time.Now().UTC()
	record := st.QueueRecord{Runs: []st.Run{
		{ID: "expired", Status: st.Succeeded, UpdatedAt: now.Add(-8 * 24 * time.Hour)},
		{ID: "recent", Status: st.Succeeded, UpdatedAt: now.Add(-time.Hour)},
	}}
	require.True(t, pruneRunsInternal(&record, now))
	require.Equal(t, []string{"recent"}, []string{record.Runs[0].ID})
	require.False(t, pruneRunsInternal(&record, now))
}
