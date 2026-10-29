// Source: florianfinn/docklet-next/backend/pkg/scheduler/runs
// Revision: 68ed0f5d915cc4047a2eeedc2cd54e0570390620
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: New admission retry and duplicate repair regressions.

package runs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
)

// claimDuringAdmissionObserverInternal claims the pending run while Submit prepares
// its write, so the admission loses its compare-and-swap and reruns.
type claimDuringAdmissionObserverInternal struct {
	ctx    context.Context
	q      *Coordinator
	target atomic.Pointer[st.Run]
	err    error
}

func (o *claimDuringAdmissionObserverInternal) ActivityID(run st.Run) string {
	target := o.target.Load()
	if target == nil || run.ID != target.ID || !o.target.CompareAndSwap(target, nil) {
		return ""
	}
	o.err = o.q.UpdateRun(o.ctx, *target, func(current *st.Run) error {
		current.Status = st.Running
		current.AttemptCount = 1
		current.UpdatedAt = time.Now().UTC()
		return nil
	})
	return ""
}

func (*claimDuringAdmissionObserverInternal) SyncRunActivity(context.Context, st.Run) error {
	return nil
}

func TestSubmitRetryAfterLostClaimRaceAppendsOnlyTheNewRun(t *testing.T) {
	q := newResolutionQueueInternal(t)
	observer := &claimDuringAdmissionObserverInternal{ctx: t.Context(), q: q}
	q.SetObserver(observer)
	pending, err := q.Submit(t.Context(), st.Request{JobID: "deliver", Trigger: "scheduled"})
	require.NoError(t, err)
	observer.target.Store(&pending)

	woken, err := q.Submit(t.Context(), st.Request{JobID: "deliver", Trigger: "event"})
	require.NoError(t, err)
	require.NoError(t, observer.err)
	require.NotEqual(t, pending.ID, woken.ID)
	require.Equal(t, st.Queued, woken.Status)

	record, err := q.ScheduleState(t.Context(), "deliver")
	require.NoError(t, err)
	require.Len(t, record.Runs, 2)
	require.Equal(t, pending.ID, record.Runs[0].ID)
	require.Equal(t, st.Running, record.Runs[0].Status)
	require.Equal(t, woken.ID, record.Runs[1].ID)
}

func TestStartupRepairsDuplicatedRunAndResumesDispatch(t *testing.T) {
	q := newResolutionQueueInternal(t)
	var executions atomic.Int32
	q.SetExecutor(func(context.Context, st.Run) (st.Outcome, error) {
		executions.Add(1)
		return st.Outcome{Status: st.Succeeded}, nil
	}, nil)
	stale, err := q.Submit(t.Context(), st.Request{JobID: "deliver", Trigger: "scheduled"})
	require.NoError(t, err)
	finished := stale
	finished.Status = st.Succeeded
	finished.AttemptCount = 1
	finished.UpdatedAt = stale.UpdatedAt.Add(time.Second)
	finished.FinishedAt = &finished.UpdatedAt
	require.NoError(t, q.mutateInternal(t.Context(), "0", "deliver", func(record *st.QueueRecord) error {
		record.Runs = []st.Run{finished, stale}
		return nil
	}))

	require.NoError(t, q.Start(t.Context(), t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		require.NoError(t, q.Stop(ctx))
	})
	record, err := q.ScheduleState(t.Context(), "deliver")
	require.NoError(t, err)
	require.Len(t, record.Runs, 1)
	require.Equal(t, stale.ID, record.Runs[0].ID)
	require.Equal(t, st.Succeeded, record.Runs[0].Status)

	q.Activate()
	next, err := q.Submit(t.Context(), st.Request{JobID: "deliver", Trigger: "event"})
	require.NoError(t, err)
	require.NotEqual(t, stale.ID, next.ID)
	require.Eventually(t, func() bool {
		persisted, getErr := q.Get(t.Context(), "0", "deliver", next.ID)
		return getErr == nil && persisted.Status == st.Succeeded
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, int32(1), executions.Load())
}
