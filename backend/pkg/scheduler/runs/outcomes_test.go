// Source: docklet-next/issues/675
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original scheduler regression coverage.

package runs

import (
	"cmp"
	"context"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/italypaleale/francis/actor"
	"github.com/stretchr/testify/require"
	kit "go.getarcane.app/kit/pkg"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type executionEnvelopeInternal struct{ command st.ExecutionCommand }

func (e executionEnvelopeInternal) Decode(v any) error {
	*v.(*st.ExecutionCommand) = e.command
	return nil
}

func TestSuccessfulCanceledWorkPersistsAndTerminalRedeliveryDoesNothing(t *testing.T) {
	for _, mode := range []string{"shutdown", "drain"} {
		t.Run(mode, func(t *testing.T) {
			q := newResolutionQueueInternal(t)
			var count atomic.Int32
			entered := make(chan struct{})
			finish := make(chan struct{})
			q.SetExecutor(func(ctx context.Context, _ st.Run) (st.Outcome, error) {
				count.Add(1)
				close(entered)
				<-ctx.Done()
				<-finish
				return st.Outcome{Status: st.Succeeded}, nil
			}, nil)
			run, err := q.Submit(t.Context(), st.Request{JobID: "shutdown-result"})
			require.NoError(t, err)
			command := st.ExecutionCommand{EnvironmentID: "0", JobID: run.JobID, RunID: run.ID, Attempt: 1}
			require.NoError(t, q.Start(t.Context(), t.Context()))
			q.Activate()
			<-entered
			drained := make(chan error, 1)
			if mode == "drain" {
				go func() { drained <- q.PauseAndDrain(t.Context()) }()
			} else {
				go func() {
					stopCtx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
					defer cancel()
					drained <- q.Stop(stopCtx)
				}()
			}
			require.Eventually(t, func() bool { q.mu.Lock(); defer q.mu.Unlock(); return !q.enabled.Load() }, time.Second, time.Millisecond)
			close(finish)
			require.NoError(t, <-drained)
			if mode == "drain" {
				t.Cleanup(func() { require.NoError(t, q.Stop(context.WithoutCancel(t.Context()))) })
			}

			stored, err := q.Get(t.Context(), "0", run.JobID, run.ID)
			require.NoError(t, err)
			require.Equal(t, st.Succeeded, stored.Status)
			redelivery := New(q.service, time.UTC)
			redelivery.runtime = q.runtime
			redelivery.execute = q.execute
			redelivery.enabled.Store(true)
			command.Attempt = 2
			require.NoError(t, (&executorActorInternal{q: redelivery}).Job(t.Context(), "execute", executionEnvelopeInternal{command}))
			require.Equal(t, int32(1), count.Load())
		})
	}
}

// After the shutdown cancel a determined nil-error result persists whatever its status; a cancellation
// error leaves the claim for startup reconciliation.
func TestCanceledOutcomeKeepsDeterminedResultsAndLeavesInterruptions(t *testing.T) {
	for _, status := range []st.RunStatus{st.Partial, st.Failed, ""} {
		t.Run(cmp.Or(string(status), "interrupted"), func(t *testing.T) {
			q := newResolutionQueueInternal(t)
			entered := make(chan struct{})
			q.SetExecutor(func(ctx context.Context, _ st.Run) (st.Outcome, error) {
				close(entered)
				<-ctx.Done()
				if status == "" {
					return st.Outcome{Status: st.Partial}, ctx.Err()
				}
				return st.Outcome{Status: status}, nil
			}, nil)
			run, err := q.Submit(t.Context(), st.Request{JobID: "shutdown-determined"})
			require.NoError(t, err)
			require.NoError(t, q.Start(t.Context(), t.Context()))
			q.Activate()
			<-entered
			stopCtx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			require.NoError(t, q.Stop(stopCtx))
			stored, err := q.Get(t.Context(), "0", run.JobID, run.ID)
			require.NoError(t, err)
			require.Equal(t, cmp.Or(status, st.Running), stored.Status)
		})
	}
}

func TestOutcomeContextRespectsStopBudgetAndDrain(t *testing.T) {
	q := newResolutionQueueInternal(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	q.stopDeadline = time.Now().Add(2200 * time.Millisecond)
	write, release := q.outcomeContextInternal(ctx)
	defer release()
	deadline, ok := write.Deadline()
	require.True(t, ok)
	require.NoError(t, write.Err())
	require.WithinDuration(t, q.stopDeadline.Add(-time.Second), deadline, 20*time.Millisecond)
	q.stopDeadline = time.Time{}
	write, release = q.outcomeContextInternal(ctx)
	defer release()
	deadline, ok = write.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(30*time.Second), deadline, 50*time.Millisecond)
}

func TestOrphanDispatchBothPathsAndNegativeControls(t *testing.T) {
	for _, path := range []string{"reconcile", "dispatch"} {
		for _, age := range []time.Duration{-time.Hour, 14 * time.Minute, 16 * time.Minute} {
			t.Run(path+age.String(), func(t *testing.T) {
				q := newResolutionQueueInternal(t)
				run, err := q.Submit(t.Context(), st.Request{JobID: "orphan"})
				require.NoError(t, err)
				id := kit.SHA256Hex("0\x00" + run.JobID)
				command := st.ExecutionCommand{EnvironmentID: "0", JobID: run.JobID, RunID: run.ID, Attempt: 1}
				// A future job keeps the real runtime from leasing it before the actor method.
				due := time.Now().Add(time.Hour)
				jobID, _, err := q.service.Dispatch(t.Context(), executorTypeInternal, id, "execute", command, actor.WithJobDueTime(due))
				require.NoError(t, err)
				q.now = func() time.Time { return due.Add(age) }
				var state st.CoordinatorState
				require.NoError(t, q.service.GetState(t.Context(), coordinatorTypeInternal, id, &state))
				run = state.Record.Runs[0]
				run.Status = st.Running
				run.AttemptCount = 1
				state.Record.Runs[0] = run
				state.Dispatches = map[string]st.DispatchIntent{run.ID + "-1": {RunID: run.ID, Attempt: 1, JobID: jobID}}
				require.NoError(t, q.service.SetState(t.Context(), coordinatorTypeInternal, id, state, nil))
				a := &coordinatorActorInternal{id: id, q: q}
				if path == "reconcile" {
					active, e := a.reconcileDispatchInternal(t.Context(), &state, run)
					require.NoError(t, e)
					require.Equal(t, age < 15*time.Minute, active)
				} else {
					require.NoError(t, a.dispatchRunInternal(t.Context(), &state, run, 1))
				}
				_, err = q.service.GetJob(t.Context(), jobID)
				if age >= 15*time.Minute {
					require.ErrorIs(t, err, actor.ErrJobNotFound)
					if path == "dispatch" {
						jobs, e := q.service.ListJobs(t.Context(), executorTypeInternal, id)
						require.NoError(t, e)
						require.Len(t, jobs, 1)
						require.NotEqual(t, jobID, jobs[0].JobID)
					}
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
	require.False(t, orphanedDispatchInternal(actor.JobInfo{Status: actor.JobStatusActive, DueTime: time.Now().Add(-time.Hour)}, time.Now()))
}

func TestTerminalActivityReceiptSurvivesCoordinatorRestart(t *testing.T) {
	databaseURL := sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))
	runtime := francistest.New(t, databaseURL)
	q := New(runtime.Service(), time.UTC)
	require.NoError(t, q.Register(runtime))
	francistest.Start(t, runtime)
	observer := &terminalReceiptObserverInternal{}
	observer.unavailable.Store(true)
	q.SetObserver(observer)
	run, err := q.Submit(t.Context(), st.Request{JobID: "receipt"})
	require.NoError(t, err)
	require.NoError(t, q.UpdateRun(t.Context(), run, func(r *st.Run) error { r.Status = st.Running; r.Owner = "worker"; return nil }))
	run.Owner = "worker"
	q.persistOutcomeInternal(t.Context(), run, st.Outcome{Status: st.Succeeded})
	require.True(t, q.hasPendingActivitySyncInternal(t.Context()))
	require.NoError(t, runtime.Stop(t.Context()))
	nextRuntime := francistest.New(t, databaseURL)
	restarted := New(nextRuntime.Service(), time.UTC)
	require.NoError(t, restarted.Register(nextRuntime))
	francistest.Start(t, nextRuntime)

	restarted.SetObserver(observer)
	observer.unavailable.Store(false)
	restarted.retryActivitySyncInternal(t.Context())
	require.False(t, restarted.hasPendingActivitySyncInternal(t.Context()))
	require.Positive(t, observer.observed.Load())
	require.True(t, observer.terminal.Load(), "restart must project the persisted terminal status")
	stored, err := restarted.Get(t.Context(), "0", run.JobID, run.ID)
	require.NoError(t, err)
	require.Equal(t, st.Succeeded, stored.Status)

}

func TestOrphanCompletionBeforeLiveDeleteRetainsOneExecution(t *testing.T) {
	q := newResolutionQueueInternal(t)
	var executions atomic.Int32
	q.SetExecutor(func(context.Context, st.Run) (st.Outcome, error) {
		executions.Add(1)
		return st.Outcome{Status: st.Succeeded}, nil
	}, nil)
	run, err := q.Submit(t.Context(), st.Request{JobID: "completion-race"})
	require.NoError(t, err)
	require.NoError(t, q.Start(t.Context(), t.Context()))
	q.Activate()
	defer func() { require.NoError(t, q.Stop(context.WithoutCancel(t.Context()))) }()
	id := kit.SHA256Hex("0\x00" + run.JobID)
	var job actor.JobInfo
	require.Eventually(t, func() bool {
		jobs, e := q.service.ListJobs(t.Context(), executorTypeInternal, id)
		if e != nil || len(jobs) != 1 {
			return false
		}
		job = jobs[0]
		return job.Status.IsTerminal()
	}, 5*time.Second, time.Millisecond)
	var state st.CoordinatorState
	require.NoError(t, q.service.GetState(t.Context(), coordinatorTypeInternal, id, &state))
	a := &coordinatorActorInternal{id: id, q: q}
	require.NoError(t, a.deleteOrphanInternal(t.Context(), &state, job))
	retained, err := q.service.GetJob(t.Context(), job.JobID)
	require.NoError(t, err)
	require.True(t, retained.Status.IsTerminal())
	require.NoError(t, a.dispatchRunInternal(t.Context(), &state, run, 2))
	require.Equal(t, int32(1), executions.Load())
}

type blockingResultActorInternal struct {
	*coordinatorActorInternal
	writing chan context.Context
	unblock chan struct{}
}

func (a *blockingResultActorInternal) Invoke(ctx context.Context, method string, data actor.Envelope) (any, error) {
	if method == "replace" {
		var state st.CoordinatorState
		if err := data.Decode(&state); err != nil {
			return nil, err
		}
		for _, run := range state.Record.Runs {
			if run.Status == st.Succeeded {
				a.writing <- ctx
				select {
				case <-a.unblock:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				break
			}
		}
	}
	return a.coordinatorActorInternal.Invoke(ctx, method, data)
}
func TestStopAndDrainTrackBlockedOutcomeWrite(t *testing.T) {
	for _, mode := range []string{"stop", "drain"} {
		t.Run(mode, func(t *testing.T) {
			runtime := francistest.New(t)
			q := New(runtime.Service(), time.UTC)
			q.runtime = runtime
			writing := make(chan context.Context, 1)
			unblock := make(chan struct{})
			require.NoError(t, runtime.RegisterActor(coordinatorTypeInternal, func(id string, _ *actor.Service) actor.Actor {
				return &blockingResultActorInternal{&coordinatorActorInternal{id: id, q: q}, writing, unblock}
			}))
			require.NoError(t, runtime.RegisterActor(executorTypeInternal, func(string, *actor.Service) actor.Actor { return &executorActorInternal{q: q} }))
			francistest.Start(t, runtime)
			entered := make(chan struct{})
			q.SetExecutor(func(ctx context.Context, _ st.Run) (st.Outcome, error) {
				close(entered)
				<-ctx.Done()
				return st.Outcome{Status: st.Succeeded}, nil
			}, nil)
			_, err := q.Submit(t.Context(), st.Request{JobID: "blocked-result"})
			require.NoError(t, err)
			require.NoError(t, q.Start(t.Context(), t.Context()))
			q.Activate()
			<-entered
			budget, cancel := context.WithTimeout(t.Context(), 2200*time.Millisecond)
			defer cancel()
			deadline, _ := budget.Deadline()
			finished := make(chan error, 1)
			go func() {
				if mode == "stop" {
					finished <- q.Stop(budget)
				} else {
					finished <- q.PauseAndDrain(budget)
				}
			}()
			write := <-writing
			observed, ok := write.Deadline()
			require.True(t, ok)
			if mode == "stop" {
				require.False(t, observed.After(deadline.Add(-time.Second+25*time.Millisecond)))
			} else {
				require.Greater(t, time.Until(observed), 29*time.Second)
			}
			select {
			case <-finished:
				t.Fatal("drain returned before the write settled")
			default:
			}
			close(unblock)
			require.NoError(t, <-finished)
			require.True(t, time.Now().Before(deadline))
			if mode == "drain" {
				require.NoError(t, q.Stop(t.Context()))
			}
		})
	}
}

type terminalReceiptObserverInternal struct {
	activityObserverFakeInternal
	terminal atomic.Bool
}

func (o *terminalReceiptObserverInternal) SyncRunActivity(ctx context.Context, run st.Run) error {
	if err := o.activityObserverFakeInternal.SyncRunActivity(ctx, run); err != nil {
		return err
	}
	if run.Status == st.Succeeded {
		o.terminal.Store(true)
	}
	return nil
}
