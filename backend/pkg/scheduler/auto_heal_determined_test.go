// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M13 closing-verification regressions.

package scheduler

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/autoheal"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	nt "github.com/florianfinn/docklet-next/types/notification"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestAutoHealDeterminedOutcomeNotifiesWhenCancelledWhileSaved(t *testing.T) {
	for _, stage := range []string{"settle", "projection"} {
		for _, agent := range []bool{false, true} {
			for _, failed := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/agent=%t/failed=%t", stage, agent, failed), func(t *testing.T) {
					j, db := autoHealFixture(t)
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					calls := 0
					j.store.Config = &config.Config{AgentMode: agent}
					j.store.Dispatch = func(context.Context, nt.DispatchRequest) error { calls++; return nil }
					j.restartContainer = func(context.Context, *client.Client, string) error {
						if failed {
							return errors.New("controlled Docker failure")
						}
						return nil
					}
					injected := false
					if stage == "settle" {
						require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:cancel-settle", func(tx *gorm.DB) {
							if tx.Statement.Table == "auto_heal_restarts" {
								if m, ok := tx.Statement.Dest.(map[string]any); ok && m["outcome"] != nil {
									injected = true
									cancel()
								}
							}
						}))
					} else {
						require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:cancel-project", func(tx *gorm.DB) {
							if _, ok := tx.Statement.Dest.(*activity.Activity); ok {
								injected = true
								cancel()
							}
						}))
					}
					_, _ = j.Run(ctx)
					require.True(t, injected)
					require.ErrorIs(t, ctx.Err(), context.Canceled)
					var row autoheal.Restart
					require.NoError(t, db.First(&row).Error)
					var notices, activities, events int64
					require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&notices).Error)
					require.NoError(t, db.Model(&activity.Activity{}).Count(&activities).Error)
					require.NoError(t, db.Model(&event.Event{}).Count(&events).Error)
					t.Logf("outcome=%s projected=%t notices=%d dispatches=%d activities=%d events=%d", row.Outcome, row.ProjectedAt != nil, notices, calls, activities, events)
					require.NotNil(t, row.ProjectedAt)
					require.EqualValues(t, 1, activities)
					require.EqualValues(t, 1, events)
					// The Docker call already determined the outcome; saving it is not an interruption.
					if agent {
						require.Zero(t, notices)
						require.Equal(t, 1, calls, "a determined outcome dispatches once")
					} else {
						require.EqualValues(t, 1, notices, "a determined outcome notifies once")
						require.Zero(t, calls)
					}
				})
			}
		}
	}
}
func TestAutoHealProgressErrorFailsThePersistedRun(t *testing.T) {
	j, _ := autoHealFixture(t)
	runtime := francistest.New(t)
	q := runs.New(runtime.Service(), time.UTC)
	require.NoError(t, q.Register(runtime))
	francistest.Start(t, runtime)
	q.SetExecutor(func(ctx context.Context, r st.Run) (st.Outcome, error) {
		ctx = jobcontext.WithExecution(ctx, r, func(target st.TargetOutcome) error {
			if target.ResourceType == "container" {
				return errors.New("controlled progress failure")
			}
			return nil
		})
		return j.Run(ctx)
	}, func(ctx context.Context, r st.Run) (st.Outcome, error) { return j.Reconcile(ctx, r) })
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
	t.Logf("status=%s message=%s", settled.Status, settled.Outcome.Message)
	require.Equal(t, st.Failed, settled.Status)
	require.Contains(t, settled.Outcome.Message, "controlled progress failure")
}

func TestAutoHealSchedulerShutdownKeepsTheAttemptInterrupted(t *testing.T) {
	j, db := autoHealFixture(t)
	entered := make(chan struct{})
	returned := make(chan struct{})
	j.restartContainer = func(ctx context.Context, _ *client.Client, _ string) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}
	runtime := francistest.New(t)
	q := runs.New(runtime.Service(), time.UTC)
	require.NoError(t, q.Register(runtime))
	francistest.Start(t, runtime)
	q.SetExecutor(func(ctx context.Context, _ st.Run) (st.Outcome, error) { defer close(returned); return j.Run(ctx) }, func(ctx context.Context, r st.Run) (st.Outcome, error) { return j.Reconcile(ctx, r) })
	require.NoError(t, q.Start(t.Context(), t.Context()))
	q.Activate()
	run, err := q.Submit(t.Context(), st.Request{JobID: j.Name(), Trigger: "scheduled"})
	require.NoError(t, err)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("restart never entered")
	}
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
	defer cancel()
	require.NoError(t, q.Stop(stopCtx))
	select {
	case <-returned:
	case <-stopCtx.Done():
		t.Fatal("worker did not return")
	}
	settled, err := q.Get(t.Context(), run.EnvironmentID, run.JobID, run.ID)
	require.NoError(t, err)
	require.Equal(t, st.Running, settled.Status, "coordinator leaves cancelled claim for startup reconciliation")
	var row autoheal.Restart
	require.NoError(t, db.First(&row).Error)
	require.Equal(t, "interrupted", row.Outcome)
	require.NotNil(t, row.ProjectedAt)
	var n int64
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.Zero(t, n)
	t.Logf("run=%s durable=%s projected=%t notices=%d", settled.Status, row.Outcome, row.ProjectedAt != nil, n)
}
