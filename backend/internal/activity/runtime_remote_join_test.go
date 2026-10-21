// Source: docklet-next/docs/design/runtime-control.md
// Revision: 3c14715d1bab4ac6e7c7f6731bef68aacfdfbfb2
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original refresh-joins-dispatch tests for issue #155.

package activity

import (
	"context"
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
	p.execute = func(_ context.Context, e activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error) {
		close(entered)
		<-release
		return remoteAnswer(e.Identity, f.store.now, activitytypes.StatusSuccess), nil
	}
	p.query = func(_ context.Context, id activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
		return activitytypes.RuntimeExecutionOutcome{Identity: id, State: activitytypes.RuntimeAnswerNotReceived}, nil
	}
	f = remoteFixture(t, p, schedule)
	_, err := f.control.Start(t.Context(), f.ref, f.commandValue.Options)
	require.NoError(t, err)
	<-entered
	refreshed := make(chan activitytypes.RuntimeSnapshot, 1)
	go func() {
		snapshot, _ := f.control.Refresh(t.Context(), f.ref)
		refreshed <- snapshot
	}()
	// The reader waits for the dispatch instead of querying ahead of Execute.
	time.Sleep(50 * time.Millisecond)
	require.Zero(t, p.queries.Load())
	close(release)
	select {
	case snapshot := <-refreshed:
		require.Equal(t, activitytypes.StatusSuccess, snapshot.Status)
	case <-time.After(5 * time.Second):
		require.Fail(t, "refresh did not return after the dispatch")
	}
	require.Zero(t, p.queries.Load())
	require.Nil(t, currentReceipt(f).HubDisposition, "no not-received fence was recorded")
}
