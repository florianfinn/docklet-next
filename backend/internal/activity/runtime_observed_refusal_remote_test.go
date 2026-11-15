// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original Direct and Edge proof that observed zero-effect refusals settle known.

package activity_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	a "github.com/florianfinn/docklet-next/types/activity"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

// refusingExecution fails target identity only inside an observed execution, so
// admission and acceptance resolve normally and the refusal comes from dispatch.
type refusingExecution struct{ *project.ProjectService }

func (p *refusingExecution) ResolveRuntimeProjectIdentity(ctx context.Context, id, name string) (projectDTO.Reference, bool, error) {
	if dto.ObservedExecutionFromContext(ctx).Mode != "" {
		return projectDTO.Reference{}, false, errors.New("docker unavailable")
	}
	return p.ProjectService.ResolveRuntimeProjectIdentity(ctx, id, name)
}

// R2S1-3: an observed adapter refusal completes as a known failed run with
// "not-needed" evidence on the agent and on the hub, without an exclusion or hold.
func TestObservedRefusalSettlesKnownDirectEdge(t *testing.T) {
	for _, edge := range []bool{false, true} {
		t.Run(fmt.Sprint(edge), func(t *testing.T) {
			f := newRemoteIntegration(t, edge, func(agent *runtimeServicesFixture) {
				agent.adapter = activity.NewRuntimeServiceAdapter(&refusingExecution{ProjectService: agent.projects}, agent.containers)
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
			local, err := f.agent.waitLocal(ref.RunID)
			require.NoError(t, err)
			require.Equal(t, a.StatusFailed, local.Status)
			require.Equal(t, a.RuntimeOutcomeKnown, local.OutcomeState)
			require.Equal(t, "runtime.target-unavailable", local.ImageLifecycle.Code)
			require.Equal(t, "not-needed", local.ImageLifecycle.Recovery.Outcome)
			snapshot, err := f.control.Refresh(t.Context(), ref)
			require.NoError(t, err)
			require.Equal(t, a.RuntimeOutcomeKnown, snapshot.OutcomeState)
			require.Equal(t, local.ImageLifecycle, snapshot.ImageLifecycle)
			var receipt activity.RuntimeReceipt
			require.NoError(t, f.db.First(&receipt, "run_id = ?", ref.RunID).Error)
			require.NotNil(t, receipt.SettledAt)
			require.Nil(t, receipt.UnknownReason, "a known refusal must not hold the host")
			require.EqualValues(t, 1, f.executeCalls.Load(), "one dispatch; the refusal happens on the agent")
		})
	}
}
