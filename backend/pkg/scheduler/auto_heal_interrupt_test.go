// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M13 closing-verification regressions.

package scheduler

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/autoheal"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	notificationDTO "github.com/florianfinn/docklet-next/types/notification"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutoHealRecoveredPanicFailsTheRun(t *testing.T) {
	j, _ := autoHealFixture(t)
	j.restartContainer = func(context.Context, *client.Client, string) error { panic("controlled restart panic") }
	outcome, err := j.Run(t.Context())
	require.ErrorContains(t, err, "auto-heal worker panicked")
	require.Equal(t, st.Failed, outcome.Status, "scheduler trusts a nonempty outcome status even when Run returns error")
}
func TestAutoHealRecoveredPanicKeepsTheAttemptAndDoesNotReplay(t *testing.T) {
	j, db := autoHealFixture(t)
	calls := 0
	j.restartContainer = func(context.Context, *client.Client, string) error { calls++; panic("controlled restart panic") }
	_, err := j.Run(t.Context())
	require.Error(t, err)
	var row autoheal.Restart
	require.NoError(t, db.First(&row).Error)
	require.Equal(t, "attempting", row.Outcome)
	require.Nil(t, row.ProjectedAt)
	j.inspectContainer = func(_ context.Context, _ *client.Client, id string) (container.InspectResponse, error) {
		v := healInspect(id)
		v.State.Health.Status = container.Healthy
		return v, nil
	}
	_, err = j.Run(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.NoError(t, db.First(&row).Error)
	require.Equal(t, "interrupted", row.Outcome)
	require.NotNil(t, row.ProjectedAt)
	var n int64
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.Zero(t, n)
}

func TestAutoHealCancelledRestartIsInterruptedAndSilent(t *testing.T) {
	j, db := autoHealFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	j.restartContainer = func(context.Context, *client.Client, string) error { cancel(); return context.Canceled }
	_, _ = j.Run(ctx)
	var n int64
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.Zero(t, n, "interrupted Auto Heal work must not send a failure notice")
}

func TestAutoHealCancelledRestartOnAnAgentDispatchesNothing(t *testing.T) {
	for _, cancelled := range []bool{true, false} {
		j, db := autoHealFixture(t)
		calls := 0
		j.store.Config = &config.Config{AgentMode: true}
		j.store.Dispatch = func(context.Context, notificationDTO.DispatchRequest) error { calls++; return nil }
		ctx, cancel := context.WithCancel(t.Context())
		j.restartContainer = func(context.Context, *client.Client, string) error {
			if cancelled {
				cancel()
				return context.Canceled
			}
			return errors.New("controlled Docker failure")
		}
		_, _ = j.Run(ctx)
		cancel()
		var row autoheal.Restart
		require.NoError(t, db.First(&row).Error)
		if cancelled {
			require.Equal(t, "interrupted", row.Outcome)
			require.Zero(t, calls, "an interrupted attempt dispatches nothing")
		} else {
			require.Equal(t, "restart-failed", row.Outcome)
			require.Equal(t, 1, calls, "a failed restart dispatches once")
		}
	}
}
