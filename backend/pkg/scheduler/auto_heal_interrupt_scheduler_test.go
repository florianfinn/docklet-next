// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M13 closing-verification regressions.

package scheduler

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAutoHealRecoveredPanicFailsThePersistedRun(t *testing.T) {
	j, _ := autoHealFixture(t)
	j.restartContainer = func(context.Context, *client.Client, string) error { panic("controlled restart panic") }
	runtime := francistest.New(t)
	q := runs.New(runtime.Service(), time.UTC)
	require.NoError(t, q.Register(runtime))
	francistest.Start(t, runtime)
	q.SetExecutor(func(ctx context.Context, _ st.Run) (st.Outcome, error) { return j.Run(ctx) }, func(ctx context.Context, r st.Run) (st.Outcome, error) { return j.Reconcile(ctx, r) })
	require.NoError(t, q.Start(t.Context(), t.Context()))
	q.Activate()
	defer q.Stop(context.WithoutCancel(t.Context()))
	run, err := q.Submit(t.Context(), st.Request{JobID: j.Name(), Trigger: "scheduled"})
	require.NoError(t, err)
	var settled st.Run
	require.Eventually(t, func() bool {
		settled, err = q.Get(t.Context(), run.EnvironmentID, run.JobID, run.ID)
		return err == nil && settled.Status.Terminal()
	}, 5*time.Second, 20*time.Millisecond)
	t.Logf("settled status=%s outcome=%s message=%s targets=%v", settled.Status, settled.Outcome.Status, settled.Outcome.Message, settled.Outcome.Targets)
	require.Equal(t, st.Failed, settled.Status, "a recovered panic must fail the persisted run")
}
