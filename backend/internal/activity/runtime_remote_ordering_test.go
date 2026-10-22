// Source: docklet-next/docs/design/runtime-control.md
// Revision: c0c290cb4ce7bbba37fdbd6d4821692df0312738
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Deterministic admission and follower departure interleavings.

package activity

import (
	"context"
	"sync"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRemoteRecoveryHandoffWhileFollowerDeparts(t *testing.T) {
	for _, mode := range []string{"refresh", "target"} {
		t.Run(mode, func(t *testing.T) {
			schedule, _ := followSchedule()
			p := &remoteTransportProbe{}
			var f *remoteFixtureInternal
			p.execute = activeAcceptance(&f)
			f = remoteFixture(t, p, schedule)
			// Hold the follower at its departure boundary rather than launching its timer.
			// Dispatch still performs its real active-result handoff.
			f.control.followEvery = time.Hour
			f.start(t)
			require.NoError(t, f.control.Close(t.Context()))
			f.control.mu.Lock()
			f.control.paused = false
			f.control.mu.Unlock()
			snapshot, err := f.control.Snapshot(t.Context(), f.ref)
			require.NoError(t, err)
			f.control.mu.Lock()
			old := f.control.registerFollowLockedInternal(f.ref, snapshot, nil)
			f.control.mu.Unlock()
			require.NotNil(t, old)
			finish := sync.OnceFunc(func() { f.control.finishFollowInternal(f.ref, old, true) })
			t.Cleanup(finish)
			// Model the completed unknown Refresh whose stop decision was preempted.
			snapshot, err = f.control.Refresh(old.ctx, f.ref)
			require.NoError(t, err)
			require.Equal(t, activitytypes.RuntimeOutcomeUnknown, snapshot.OutcomeState)
			p.query = func(_ context.Context, id activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
				return remoteAnswer(id, f.store.now, activitytypes.StatusRunning), nil
			}
			if mode == "refresh" {
				_, err = f.control.Refresh(t.Context(), f.ref)
				require.NoError(t, err)
			} else {
				require.NoError(t, f.control.RecoverTarget(t.Context(), f.ref.Store))
			}
			require.Eventually(t, func() bool {
				f.control.mu.Lock()
				defer f.control.mu.Unlock()
				return f.control.queries[f.ref.RunID] == nil
			}, 5*time.Second, time.Millisecond)
			snapshot, err = f.control.Snapshot(t.Context(), f.ref)
			require.NoError(t, err)
			require.Equal(t, activitytypes.RuntimeOutcomeKnown, snapshot.OutcomeState)
			// Resume the exact finalizer used by the departing follower.
			finish()
			f.control.mu.Lock()
			next := f.control.follows[f.ref.RunID]
			f.control.mu.Unlock()
			require.NotNil(t, next, "active recovery must retain a follower after the old one departs")
			require.NotSame(t, old.task, next)
			select {
			case <-old.task.done:
			default:
				t.Fatal("old follower did not end")
			}
			require.EqualValues(t, 1, p.executes.Load())
		})
	}
}

func TestRuntimeRemotePendingRefreshKeepsLifecycle(t *testing.T) {
	for _, mode := range []string{"cancel", "startup", "target"} {
		t.Run(mode, func(t *testing.T) {
			p := &remoteTransportProbe{}
			f := remoteFixture(t, p, RuntimeReconcileSchedule{})
			_, err := f.control.Refresh(t.Context(), f.ref)
			require.NoError(t, err)
			require.Zero(t, p.queries.Load())
			f.control.mu.Lock()
			following := f.control.follows[f.ref.RunID] != nil
			f.control.mu.Unlock()
			require.False(t, following, "pending admission alone must not create a follower")
			switch mode {
			case "cancel":
				_, err = f.control.Cancel(t.Context(), f.ref, f.commandValue.Actor)
				require.NoError(t, err)
				f.start(t)
				require.Equal(t, "cancelled", *f.receipt(t).HubDisposition)
			case "startup":
				require.NoError(t, f.control.Startup(t.Context()))
				require.Equal(t, "interrupted", *f.receipt(t).HubDisposition)
			case "target":
				require.NoError(t, f.control.RecoverTarget(t.Context(), f.ref.Store))
				require.Nil(t, f.receipt(t).SettledAt)
				require.Equal(t, "pending", *f.receipt(t).ExecutorState)
			}
			require.Zero(t, p.queries.Load())
			require.Zero(t, p.executes.Load())
		})
	}
}
