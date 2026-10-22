// Source: docklet-next/docs/design/runtime-control.md
// Revision: c0c290cb4ce7bbba37fdbd6d4821692df0312738
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Admission-before-dispatch refresh on real Direct and Edge transports.

package activity_test

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRemoteRefreshBeforeDispatch(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			f := newRemoteIntegration(t, isEdge)
			command := f.command(activity.RuntimeStartContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
			admitted, err := f.hub.Admit(t.Context(), command)
			require.NoError(t, err)
			ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
			snapshot, err := f.control.Refresh(t.Context(), ref)
			require.NoError(t, err)
			require.Equal(t, activitytypes.RuntimeOutcomeUnknown, snapshot.OutcomeState)
			require.Zero(t, f.accessCalls.Load(), "an ordinary pending read must not query the executor")
			var row activity.RuntimeReceipt
			require.NoError(t, f.db.First(&row, "run_id = ?", ref.RunID).Error)
			require.Equal(t, "pending", *row.DispatchState)
			require.Equal(t, "pending", *row.ExecutorState)
			require.Nil(t, row.UnknownReason)
			_, err = f.control.Start(t.Context(), ref, command.Options)
			require.NoError(t, err, "the pending command must remain dispatchable")
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			require.NoError(t, f.control.WaitDispatch(ctx, ref.RunID))
			_, err = f.agent.waitLocal(ref.RunID)
			require.NoError(t, err)
			require.Len(t, runtimeEffectEvents(f.agent.http.snapshot()), 1)
			require.EqualValues(t, 1, f.executeCalls.Load())
			require.NoError(t, f.db.First(&row, "run_id = ?", ref.RunID).Error)
			require.Nil(t, row.HubDisposition, "no premature refusal fence")
		})
	}
}
