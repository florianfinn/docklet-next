// Source: docklet-next/docs/design/runtime-control.md
// Revision: 3ccd9e86572ccd7150b2aaf5bf04c555e98a284c
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original reader-independent remote follow tests for issue #146.

package activity

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

// followSchedule lets every rate-limit check pass and keeps reconciliation
// retries instant; follow spacing itself is shortened per fixture.
func followSchedule() (RuntimeReconcileSchedule, *atomic.Int64) {
	base := time.Now()
	var calls atomic.Int64
	return RuntimeReconcileSchedule{
		Now:  func() time.Time { return base.Add(time.Duration(calls.Add(1)) * time.Second) },
		Wait: func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
	}, &calls
}

func fastFollow(f *remoteFixtureInternal) *remoteFixtureInternal {
	f.control.mu.Lock()
	f.control.followEvery = 5 * time.Millisecond
	f.control.mu.Unlock()
	return f
}

// currentReceipt reads without test assertions, so polling conditions stay safe.
func currentReceipt(f *remoteFixtureInternal) RuntimeReceipt {
	var row RuntimeReceipt
	_ = f.store.svc.db.First(&row, "run_id = ?", f.ref.RunID).Error
	return row
}

func activeAcceptance(f **remoteFixtureInternal) func(context.Context, activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error) {
	return func(_ context.Context, e activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error) {
		return remoteAnswer(e.Identity, (*f).store.now, activitytypes.StatusRunning), nil
	}
}

func TestRuntimeRemoteFollowSettlesWithoutReader(t *testing.T) {
	schedule, _ := followSchedule()
	p := &remoteTransportProbe{}
	var f *remoteFixtureInternal
	var inFlight, overlap atomic.Int32
	answers := atomic.Int32{}
	p.execute = activeAcceptance(&f)
	p.query = func(_ context.Context, id activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
		if inFlight.Add(1) > 1 {
			overlap.Add(1)
		}
		defer inFlight.Add(-1)
		if answers.Add(1) < 3 {
			return remoteAnswer(id, f.store.now, activitytypes.StatusRunning), nil
		}
		return remoteAnswer(id, f.store.now, activitytypes.StatusSuccess), nil
	}
	f = fastFollow(remoteFixture(t, p, schedule))
	f.start(t)
	require.Eventually(t, func() bool { return currentReceipt(f).SettledAt != nil }, 5*time.Second, 5*time.Millisecond)
	snapshot, err := f.control.Snapshot(t.Context(), f.ref)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, snapshot.Status)
	require.EqualValues(t, 1, p.executes.Load())
	require.Zero(t, overlap.Load())
	settled := p.queries.Load()
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, settled, p.queries.Load(), "a settled run is not queried again")
}

func TestRuntimeRemoteFollowStopsWhenUnknown(t *testing.T) {
	schedule, _ := followSchedule()
	p := &remoteTransportProbe{}
	var f *remoteFixtureInternal
	p.execute = activeAcceptance(&f)
	p.query = func(context.Context, activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
		return activitytypes.RuntimeExecutionOutcome{}, ErrRuntimeTransport
	}
	f = fastFollow(remoteFixture(t, p, schedule))
	f.start(t)
	require.Eventually(t, func() bool {
		row := currentReceipt(f)
		return row.ExecutorState != nil && *row.ExecutorState == "unknown"
	}, 5*time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool {
		f.control.mu.Lock()
		defer f.control.mu.Unlock()
		return f.control.follows[f.ref.RunID] == nil
	}, 5*time.Second, 5*time.Millisecond)
	stopped := p.queries.Load()
	require.LessOrEqual(t, stopped, int32(3), "one bounded reconciliation cycle")
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, stopped, p.queries.Load())
	require.Nil(t, f.receipt(t).SettledAt, "unknown never settles or releases")
	require.EqualValues(t, 1, p.executes.Load())
}

func TestRuntimeRemoteFollowEndsOnClose(t *testing.T) {
	schedule, _ := followSchedule()
	p := &remoteTransportProbe{}
	var f *remoteFixtureInternal
	p.execute = activeAcceptance(&f)
	p.query = func(_ context.Context, id activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
		return remoteAnswer(id, f.store.now, activitytypes.StatusRunning), nil
	}
	f = fastFollow(remoteFixture(t, p, schedule))
	f.start(t)
	require.Eventually(t, func() bool { return p.queries.Load() >= 2 }, 5*time.Second, 5*time.Millisecond)
	f.control.mu.Lock()
	follow := f.control.follows[f.ref.RunID]
	f.control.mu.Unlock()
	require.NotNil(t, follow)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, f.control.Close(ctx))
	select {
	case <-follow.done:
	default:
		require.Fail(t, "Close returned before the follower ended")
	}
	closed := p.queries.Load()
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, closed, p.queries.Load())
	require.Nil(t, f.receipt(t).SettledAt)
}

func TestRuntimeRemoteFollowResumesAfterStartup(t *testing.T) {
	schedule, _ := followSchedule()
	p := &remoteTransportProbe{}
	var f *remoteFixtureInternal
	var answers atomic.Int32
	resumed := atomic.Bool{}
	p.execute = activeAcceptance(&f)
	p.query = func(_ context.Context, id activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
		if !resumed.Load() || answers.Add(1) < 3 {
			return remoteAnswer(id, f.store.now, activitytypes.StatusRunning), nil
		}
		return remoteAnswer(id, f.store.now, activitytypes.StatusSuccess), nil
	}
	f = fastFollow(remoteFixture(t, p, schedule))
	f.start(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, f.control.Close(ctx))
	require.Nil(t, f.receipt(t).SettledAt)
	resumed.Store(true)
	require.NoError(t, f.control.Startup(ctx))
	// The first restored query still answers active; only following settles it.
	require.Eventually(t, func() bool { return currentReceipt(f).SettledAt != nil }, 5*time.Second, 5*time.Millisecond)
	require.EqualValues(t, 1, p.executes.Load())
}
