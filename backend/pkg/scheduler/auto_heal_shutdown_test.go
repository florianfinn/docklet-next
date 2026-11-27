// Source: docklet-next/issues/675
// Revision: ab66e6581dbb9abf94b1962ec4c73a701f1e7e1d
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original shutdown result guard for Auto Heal worker failures.

package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

// A determined worker failure keeps its Partial result through shutdown; a worker
// cancelled before its reservation leaves the claim for startup reconciliation.
func TestAutoHealShutdownSeparatesDeterminedFailuresFromInterruptions(t *testing.T) {
	for _, determined := range []bool{true, false} {
		t.Run(map[bool]string{true: "determined", false: "cancelled"}[determined], func(t *testing.T) {
			j, _ := autoHealFixture(t)
			reached := make(chan struct{})
			j.inspectContainer = func(ctx context.Context, _ *client.Client, _ string) (container.InspectResponse, error) {
				if determined {
					return container.InspectResponse{}, errors.New("controlled inspect failure")
				}
				close(reached)
				<-ctx.Done()
				return container.InspectResponse{}, ctx.Err()
			}
			runtime := francistest.New(t)
			q := runs.New(runtime.Service(), time.UTC)
			require.NoError(t, q.Register(runtime))
			francistest.Start(t, runtime)
			q.SetExecutor(func(ctx context.Context, r st.Run) (st.Outcome, error) {
				// The production checkpoint adapter; shutdown arrives after the target checkpoint commits.
				ctx = jobcontext.WithExecution(ctx, r, func(target st.TargetOutcome) error {
					err := q.Progress(ctx, r, target)
					if err == nil && determined && target.ResourceType == "container" {
						close(reached)
						<-ctx.Done()
					}
					return err
				})
				return j.Run(ctx)
			}, func(ctx context.Context, r st.Run) (st.Outcome, error) { return j.Reconcile(ctx, r) })
			require.NoError(t, q.Start(t.Context(), t.Context()))
			q.Activate()
			run, err := q.Submit(t.Context(), st.Request{JobID: j.Name(), Trigger: "scheduled"})
			require.NoError(t, err)
			select {
			case <-reached:
			case <-time.After(5 * time.Second):
				t.Fatal("worker never reached the shutdown point")
			}
			stopCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
			defer cancel()
			require.NoError(t, q.Stop(stopCtx))
			stored, err := q.Get(t.Context(), run.EnvironmentID, run.JobID, run.ID)
			require.NoError(t, err)
			if determined {
				require.Equal(t, st.Partial, stored.Status, "a determined worker failure survives shutdown")
			} else {
				require.Equal(t, st.Running, stored.Status, "an interrupted worker stays claimed for reconciliation")
			}
		})
	}
}
