// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original scheduled hold convergence over real Direct and Edge transports.

package activity_test

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	a "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

// heldScheduledRun leaves a scheduled run held as transport unknown on the hub
// while the executor lost its terminal commit, as after an agent process kill.
func heldScheduledRun(t *testing.T, edge bool) (*remoteIntegration, activity.RuntimeRunReference, http.Handler) {
	t.Helper()
	worker := &observedRemoteProjects{result: dto.LifecycleResult{Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: "restored"}}, entered: make(chan struct{}), release: make(chan struct{}), seen: make(chan dto.ObservedExecution, 1)}
	f := newRemoteIntegration(t, edge, func(agent *runtimeServicesFixture) {
		worker.ProjectService = agent.projects
		agent.adapter = activity.NewRuntimeServiceAdapter(worker, agent.containers)
	})
	target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
	f.agent.project(target.ID, target.ID, "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
	command := bindNoticeRemote(t, f, f.command(activity.RuntimeUpdateProject, target), "scheduled")
	admitted, err := f.hub.Admit(t.Context(), command)
	require.NoError(t, err)
	ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
	_, err = f.control.Start(t.Context(), ref, command.Options)
	require.NoError(t, err)
	require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
	<-worker.entered
	require.NoError(t, f.agent.db.Exec("CREATE TRIGGER lost_terminal BEFORE UPDATE OF executor_state ON runtime_receipts WHEN NEW.executor_state='failed' BEGIN SELECT RAISE(ABORT,'synthetic lost terminal commit'); END").Error)
	close(worker.release)
	_, err = f.agent.waitLocal(ref.RunID)
	require.Error(t, err)
	require.NoError(t, f.agent.db.Exec("DROP TRIGGER lost_terminal").Error)
	f.handlerMu.Lock()
	original := f.receiverHandler
	f.receiverHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	f.handlerMu.Unlock()
	snapshot, err := f.control.Refresh(t.Context(), ref)
	require.NoError(t, err)
	require.Equal(t, a.RuntimeOutcomeUnknown, snapshot.OutcomeState)
	_, err = activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, GuardContainer: func(context.Context, string, string) error { return nil }, PrepareTarget: func(_ context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
		return activity.RuntimeResolvedTarget{Target: target}, nil
	}})
	require.NoError(t, err)
	return f, ref, original
}

// AR5: the hub stays up, the agent restarts and settles the run interrupted; the
// scheduled wait alone converges, with exactly one interrupted notice.
func TestScheduledHoldConvergesWithoutReaderDirectEdge(t *testing.T) {
	for _, edge := range []bool{false, true} {
		t.Run(fmt.Sprint(edge), func(t *testing.T) {
			f, ref, original := heldScheduledRun(t, edge)
			restarted := activity.NewRuntimeService(activity.NewActivityService(f.agent.db, nil), f.agent.adapter)
			count, err := restarted.SettleLocalStartup(t.Context(), f.agent.store)
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			f.handlerMu.Lock()
			f.receiverHandler = original
			f.handlerMu.Unlock()
			snapshot, err := f.hubActivity.WaitPolicyUpdate(t.Context(), f.store.EnvironmentID, ref.RunID)
			require.NoError(t, err)
			require.Equal(t, a.RuntimeOutcomeKnown, snapshot.OutcomeState)
			require.Equal(t, "runtime.interrupted", snapshot.MessageCode)
			require.False(t, snapshot.ImageLifecycle.Verified)
			require.Equal(t, "interrupted", snapshot.ImageLifecycle.Recovery.Outcome)
			var rows []notificationoutbox.Row
			require.NoError(t, f.db.Find(&rows).Error)
			require.Len(t, rows, 1)
			require.Equal(t, "update-interrupted", rows[0].Payload["reason"])
			require.EqualValues(t, 1, f.executeCalls.Load(), "the effect is never resent")
		})
	}
}

// An unreachable executor is re-queried only until the hold cap, then the
// result stays unknown as before.
func TestScheduledHoldStopsAtCapDirectEdge(t *testing.T) {
	for _, edge := range []bool{false, true} {
		t.Run(fmt.Sprint(edge), func(t *testing.T) {
			f, ref, _ := heldScheduledRun(t, edge)
			var queries atomic.Int32
			f.handlerMu.Lock()
			f.receiverHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				queries.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			})
			f.handlerMu.Unlock()
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			snapshot, err := f.hubActivity.WaitPolicyUpdate(ctx, f.store.EnvironmentID, ref.RunID)
			// 10 min / 30 s spacing = 20 refresh cycles of at most 3 queries each.
			require.LessOrEqual(t, queries.Load(), int32(3*20))
			require.Positive(t, queries.Load())
			require.NoError(t, err, "the cap ends the wait, not the context")
			require.Equal(t, a.RuntimeOutcomeUnknown, snapshot.OutcomeState)
			var rows []notificationoutbox.Row
			require.NoError(t, f.db.Find(&rows).Error)
			require.Empty(t, rows)
			require.EqualValues(t, 1, f.executeCalls.Load())
		})
	}
}

// A connection that returns while the executor is still running proves an active
// run; the scheduled wait resumes and settles the terminal result.
func TestScheduledHoldResumesActiveRunDirectEdge(t *testing.T) {
	for _, edge := range []bool{false, true} {
		t.Run(fmt.Sprint(edge), func(t *testing.T) {
			worker := &observedRemoteProjects{result: dto.LifecycleResult{Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: "restored"}}, entered: make(chan struct{}), release: make(chan struct{}), seen: make(chan dto.ObservedExecution, 1)}
			f := newRemoteIntegration(t, edge, func(agent *runtimeServicesFixture) {
				worker.ProjectService = agent.projects
				agent.adapter = activity.NewRuntimeServiceAdapter(worker, agent.containers)
			})
			target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
			f.agent.project(target.ID, target.ID, "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
			command := bindNoticeRemote(t, f, f.command(activity.RuntimeUpdateProject, target), "scheduled")
			admitted, err := f.hub.Admit(t.Context(), command)
			require.NoError(t, err)
			ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
			_, err = f.control.Start(t.Context(), ref, command.Options)
			require.NoError(t, err)
			require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
			<-worker.entered
			f.handlerMu.Lock()
			original := f.receiverHandler
			f.receiverHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
			f.handlerMu.Unlock()
			snapshot, err := f.control.Refresh(t.Context(), ref)
			require.NoError(t, err)
			require.Equal(t, a.RuntimeOutcomeUnknown, snapshot.OutcomeState)
			_, err = activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, GuardContainer: func(context.Context, string, string) error { return nil }, PrepareTarget: func(_ context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
				return activity.RuntimeResolvedTarget{Target: target}, nil
			}})
			require.NoError(t, err)
			f.handlerMu.Lock()
			f.receiverHandler = original
			f.handlerMu.Unlock()
			type waited struct {
				snapshot a.RuntimeSnapshot
				err      error
			}
			done := make(chan waited, 1)
			go func() {
				snapshot, err := f.hubActivity.WaitPolicyUpdate(t.Context(), f.store.EnvironmentID, ref.RunID)
				done <- waited{snapshot, err}
			}()
			// The executor stays blocked until the wait has seen the run active again.
			require.Eventually(t, func() bool {
				current, err := f.control.Snapshot(t.Context(), ref)
				return err == nil && current.OutcomeState == a.RuntimeOutcomeKnown
			}, 10*time.Second, 10*time.Millisecond)
			select {
			case early := <-done:
				t.Fatalf("wait returned while the executor was still running: status=%s outcome=%s err=%v", early.snapshot.Status, early.snapshot.OutcomeState, early.err)
			case <-time.After(200 * time.Millisecond):
			}
			close(worker.release)
			result := <-done
			require.NoError(t, result.err)
			require.Equal(t, a.RuntimeOutcomeKnown, result.snapshot.OutcomeState)
			require.Equal(t, a.StatusFailed, result.snapshot.Status)
			require.Equal(t, "restored", result.snapshot.ImageLifecycle.Recovery.Outcome)
			var rows []notificationoutbox.Row
			require.NoError(t, f.db.Find(&rows).Error)
			require.Len(t, rows, 1)
			require.Equal(t, "auto-restored", rows[0].Payload["reason"])
			require.EqualValues(t, 1, f.executeCalls.Load())
		})
	}
}

// Expired, invalid or storage holds keep today's explicit recovery: no re-query.
func TestScheduledHoldReasonFilterDirectEdge(t *testing.T) {
	for _, edge := range []bool{false, true} {
		for _, reason := range []string{"invalid-evidence", "evidence-expired", "storage"} {
			t.Run(fmt.Sprintf("%t/%s", edge, reason), func(t *testing.T) {
				f, ref, _ := heldScheduledRun(t, edge)
				require.NoError(t, f.db.Model(&activity.RuntimeReceipt{}).Where("run_id = ?", ref.RunID).Update("unknown_reason", reason).Error)
				var queries atomic.Int32
				f.handlerMu.Lock()
				f.receiverHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					queries.Add(1)
					w.WriteHeader(http.StatusServiceUnavailable)
				})
				f.handlerMu.Unlock()
				snapshot, err := f.hubActivity.WaitPolicyUpdate(t.Context(), f.store.EnvironmentID, ref.RunID)
				require.NoError(t, err)
				require.Equal(t, a.RuntimeOutcomeUnknown, snapshot.OutcomeState)
				require.Zero(t, queries.Load())
			})
		}
	}
}
