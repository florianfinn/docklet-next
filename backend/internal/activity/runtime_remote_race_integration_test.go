// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package activity_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func remoteRunIdentity(t *testing.T, f *remoteIntegration, runID string) activitytypes.RuntimeRunIdentity {
	t.Helper()
	var row activity.RuntimeReceipt
	require.NoError(t, f.db.First(&row, "run_id = ?", runID).Error)
	return activitytypes.RuntimeRunIdentity{RuntimeExecutorIdentity: activitytypes.RuntimeExecutorIdentity{ManagerID: row.ManagerID, RegistrationID: row.RegistrationID, ExecutorGeneration: row.ExecutorGeneration}, RunID: row.RunID, RequestDigest: row.RequestDigest}
}
func TestRuntimeRemoteActualQueryFenceWinsDelayedExecute(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			f := newRemoteIntegration(t, isEdge)
			command := f.command(activity.RuntimeStartContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
			admitted, err := f.hub.Admit(t.Context(), command)
			require.NoError(t, err)
			ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
			id := remoteRunIdentity(t, f, ref.RunID)
			entered, release := make(chan struct{}), make(chan struct{})
			var once, unblockOnce sync.Once
			unblock := func() { unblockOnce.Do(func() { close(release) }) }
			defer unblock()
			f.agent.http.mu.Lock()
			f.agent.http.onCall = func(_ http.ResponseWriter, r *http.Request) bool {
				if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/containers/"+runtimeContainerID+"/json") {
					return false
				}
				once.Do(func() { close(entered) })
				<-release
				return false
			}
			f.agent.http.mu.Unlock()
			_, err = f.control.Start(t.Context(), ref, command.Options)
			require.NoError(t, err)
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("actual Execute resolver did not stall")
			}
			outcome, err := f.sender.Query(t.Context(), id)
			require.NoError(t, err)
			require.Equal(t, activitytypes.RuntimeAnswerNotReceived, outcome.State)
			require.Nil(t, outcome.Detail)
			var held activity.RuntimeReceipt
			require.NoError(t, f.db.First(&held, "run_id = ?", ref.RunID).Error)
			require.Nil(t, held.SettledAt)
			unblock()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			require.NoError(t, f.control.WaitDispatch(ctx, ref.RunID))
			require.Empty(t, runtimeEffectEvents(f.agent.http.snapshot()))
			require.EqualValues(t, 1, f.executeCalls.Load())
			var row activity.RuntimeReceipt
			require.NoError(t, f.db.First(&row, "run_id = ?", ref.RunID).Error)
			require.Equal(t, "not_received", *row.HubDisposition)
			require.NotNil(t, row.SettledAt)
		})
	}
}
func TestRuntimeRemoteActualDuplicatePreservesIdentityAndActor(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			f := newRemoteIntegration(t, isEdge)
			command := f.command(activity.RuntimeStartContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
			admitted, err := f.hub.Admit(t.Context(), command)
			require.NoError(t, err)
			ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
			_, err = f.control.Start(t.Context(), ref, command.Options)
			require.NoError(t, err)
			require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
			_, err = f.agent.waitLocal(ref.RunID)
			require.NoError(t, err)
			before := runtimeEffectEvents(f.agent.http.snapshot())
			id := remoteRunIdentity(t, f, ref.RunID)
			duplicate, err := f.sender.Execute(t.Context(), activitytypes.RuntimeExecutionEnvelope{Identity: id, Operation: command.Operation, Target: command.Target, Options: command.Options, Attribution: activitytypes.RuntimeAttribution{ID: "example-retry-attribution", Username: "Retry"}})
			require.NoError(t, err)
			require.Equal(t, id, duplicate.Identity)
			require.Equal(t, command.Actor.ID, duplicate.Detail.Activity.StartedBy.UserID)
			require.Equal(t, before, runtimeEffectEvents(f.agent.http.snapshot()))
			require.Len(t, before, 1)
			var bindings int64
			require.NoError(t, f.agent.db.Table("runtime_key_bindings").Count(&bindings).Error)
			require.Zero(t, bindings)
		})
	}
}
