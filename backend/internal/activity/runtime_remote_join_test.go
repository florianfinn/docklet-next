// Source: docklet-next/docs/design/runtime-control.md
// Revision: 3c14715d1bab4ac6e7c7f6731bef68aacfdfbfb2
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original refresh-joins-dispatch tests for issue #155.

package activity

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRemoteRefreshJoinsInFlightDispatch(t *testing.T) {
	schedule, _ := followSchedule()
	p := &remoteTransportProbe{}
	var f *remoteFixtureInternal
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	p.execute = func(_ context.Context, e activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error) {
		close(entered)
		<-release
		return remoteAnswer(e.Identity, f.store.now, activitytypes.StatusSuccess), nil
	}
	p.query = func(_ context.Context, id activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
		return activitytypes.RuntimeExecutionOutcome{Identity: id, State: activitytypes.RuntimeAnswerNotReceived}, nil
	}
	f = remoteFixture(t, p, schedule)
	t.Cleanup(unblock)
	_, err := f.control.Start(t.Context(), f.ref, f.commandValue.Options)
	require.NoError(t, err)
	b1Await(t, entered)
	type result struct {
		snapshot activitytypes.RuntimeSnapshot
		err      error
	}
	refreshed := make(chan result, 1)
	go func() {
		snapshot, err := f.control.Refresh(t.Context(), f.ref)
		refreshed <- result{snapshot, err}
	}()
	// The reader waits for the dispatch instead of querying ahead of Execute.
	require.Eventually(t, func() bool {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		for _, stack := range strings.Split(string(buf[:n]), "\n\n") {
			frames := strings.Split(stack, "\n")
			if len(frames) > 1 && strings.Contains(frames[0], "[select]") && strings.Contains(frames[1], "(*RuntimeRemoteCoordinator).refreshInternal(") && strings.Contains(stack, "TestRuntimeRemoteRefreshJoinsInFlightDispatch.func") {
				return true
			}
		}
		return false
	}, 5*time.Second, time.Millisecond)
	f.control.mu.Lock()
	dispatch, query := f.control.dispatch[f.ref.RunID], f.control.queries[f.ref.RunID]
	f.control.mu.Unlock()
	require.NotNil(t, dispatch, "the reader is waiting while dispatch still owns the run")
	require.Nil(t, query, "the reader must join dispatch, even before a query goroutine starts")
	require.Zero(t, p.queries.Load())
	unblock()
	select {
	case got := <-refreshed:
		require.NoError(t, got.err)
		require.Equal(t, activitytypes.StatusSuccess, got.snapshot.Status)
	case <-time.After(5 * time.Second):
		require.Fail(t, "refresh did not return after the dispatch")
	}
	require.Zero(t, p.queries.Load())
	require.Nil(t, currentReceipt(f).HubDisposition, "no not-received fence was recorded")
}
