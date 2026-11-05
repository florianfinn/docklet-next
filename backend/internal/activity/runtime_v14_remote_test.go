// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original real Direct/Edge v14 operation allowlist and temporary zero-effect result proofs.

package activity_test

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	dto "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestV14ContainerOperationsDirectAndEdgeUnavailableUntilS3(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, operation := range []activity.RuntimeOperation{activity.RuntimeUpdateContainer, activity.RuntimeRestoreContainerImage} {
			name := "direct/"
			if isEdge {
				name = "edge/"
			}
			t.Run(name+string(operation), func(t *testing.T) {
				f := newRemoteIntegration(t, isEdge)
				intake, e := activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, PrepareTarget: func(ctx context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
					return f.agent.adapter.ResolveRuntimeTarget(ctx, target)
				}, GuardContainer: func(context.Context, string, string) error { return nil }})
				require.NoError(t, e)
				intake.SetReady(true)
				command := f.command(operation, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
				accepted, e := f.hub.Admit(t.Context(), command)
				require.NoError(t, e)
				ref := activity.RuntimeRunReference{Store: f.store, RunID: accepted.ActivityID}
				_, e = f.control.Start(t.Context(), ref, command.Options)
				require.NoError(t, e)
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				require.NoError(t, f.control.WaitDispatch(ctx, ref.RunID))
				_, e = f.agent.waitLocal(ref.RunID)
				require.NoError(t, e)
				snapshot, e := f.control.Wait(ctx, ref, nil)
				require.NoError(t, e)
				require.Equal(t, dto.StatusFailed, snapshot.Status)
				require.Equal(t, dto.RuntimeOutcomeKnown, snapshot.OutcomeState)
				require.NotNil(t, snapshot.ImageLifecycle)
				require.Equal(t, "runtime.target-unavailable", snapshot.ImageLifecycle.Code)
				require.Equal(t, 503, snapshot.ImageLifecycle.HTTPStatus)
				require.Empty(t, runtimeEffectEvents(f.agent.http.snapshot()))
				require.EqualValues(t, 1, f.executeCalls.Load())
			})
		}
	}
}
