// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original no-effect and after-effect cancellation over Direct and Edge.

package activity_test

import (
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	a "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestObservedCancellationDirectEdge(t *testing.T) {
	for _, edge := range []bool{false, true} {
		for _, outcome := range []string{"not-needed", "interrupted"} {
			t.Run(fmt.Sprint(edge, "/", outcome), func(t *testing.T) {
				worker := &observedRemoteProjects{result: dto.LifecycleResult{Recovery: &dto.RecoveryResult{Outcome: outcome}}, entered: make(chan struct{}), release: make(chan struct{}), seen: make(chan dto.ObservedExecution, 1)}
				released := false
				defer func() {
					if !released {
						close(worker.release)
					}
				}()
				f := newRemoteIntegration(t, edge, func(agent *runtimeServicesFixture) {
					worker.ProjectService = agent.projects
					agent.adapter = activity.NewRuntimeServiceAdapter(worker, agent.containers)
				})
				target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
				f.agent.project(target.ID, target.ID, "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
				command := f.command(activity.RuntimeUpdateProject, target)
				command.Options.Lifecycle = observedRemoteIntent()
				admitted, err := f.hub.Admit(t.Context(), command)
				require.NoError(t, err)
				ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
				_, err = f.control.Start(t.Context(), ref, command.Options)
				require.NoError(t, err)
				require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
				<-worker.entered
				_, err = f.control.Cancel(t.Context(), ref, command.Actor)
				require.NoError(t, err)
				close(worker.release)
				released = true
				local, err := f.agent.waitLocal(ref.RunID)
				require.NoError(t, err)
				require.Equal(t, a.StatusCancelled, local.Status)
				require.Equal(t, a.RuntimeOutcomeKnown, local.OutcomeState)
				snapshot, err := f.control.Refresh(t.Context(), ref)
				require.NoError(t, err)
				require.Equal(t, a.StatusCancelled, snapshot.Status)
				require.Equal(t, a.RuntimeOutcomeKnown, snapshot.OutcomeState)
				require.Equal(t, outcome, snapshot.ImageLifecycle.Recovery.Outcome)
				var row activity.RuntimeReceipt
				require.NoError(t, f.db.First(&row, "run_id = ?", ref.RunID).Error)
				require.NotNil(t, row.SettledAt)
				require.Nil(t, row.UnknownReason)
			})
		}
	}
}
