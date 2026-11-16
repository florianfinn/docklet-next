// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Filtered and coalesced recovery of transport/continuity holds.

package activity

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	a "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRecoverHeldTargetFiltersEvidenceAndIdentity(t *testing.T) {
	for _, reason := range []string{"transport", "process-continuity", "invalid-evidence", "evidence-expired", "storage", "administrator", "known", "new-registration", "new-generation"} {
		t.Run(reason, func(t *testing.T) {
			p := &remoteTransportProbe{}
			f := remoteFixture(t, p, RuntimeReconcileSchedule{})
			f.start(t)
			store := f.ref.Store
			if reason == "administrator" {
				_, err := f.service.remoteDispositionInternal(t.Context(), f.ref, runtimeDispositionInternal{Disposition: "released", By: "example-owner", Reason: "Administrator hold", ReleaseAcknowledged: true})
				require.NoError(t, err)
			} else if reason == "known" {
				require.NoError(t, f.store.svc.db.Model(&RuntimeReceipt{}).Where("run_id = ?", f.ref.RunID).Updates(map[string]any{"executor_state": "accepted", "unknown_reason": nil}).Error)
			} else if reason == "new-registration" {
				store.RegistrationID = uuid.NewString()
			} else if reason == "new-generation" {
				store.ExecutorGeneration = uuid.NewString()
			} else {
				_, err := f.service.remoteUnknownInternal(t.Context(), f.ref, reason)
				require.NoError(t, err)
			}
			held, err := f.control.HasHeldTarget(t.Context(), store.EnvironmentID)
			require.NoError(t, err)
			require.Equal(t, reason != "invalid-evidence" && reason != "evidence-expired" && reason != "storage" && reason != "administrator" && reason != "known", held,
				"the precheck selects held runs of the environment independently of the executor identity")
			before := f.receipt(t)
			queries := p.queries.Load()
			p.query = func(_ context.Context, id a.RuntimeRunIdentity) (a.RuntimeExecutionOutcome, error) {
				return remoteAnswer(id, f.store.now, a.StatusSuccess), nil
			}
			require.NoError(t, f.control.RecoverHeldTarget(t.Context(), store))
			require.NoError(t, WaitRuntimeRemoteIdleForTest(t.Context(), f.control, f.ref.RunID))
			if reason == "transport" || reason == "process-continuity" {
				require.NotNil(t, f.receipt(t).SettledAt)
				require.Equal(t, queries+1, p.queries.Load())
			} else {
				require.Equal(t, before, f.receipt(t))
				require.Equal(t, queries, p.queries.Load())
			}
			require.EqualValues(t, 1, p.executes.Load())
		})
	}
}

// A held run that cannot be prepared does not keep the other held runs of the host waiting.
func TestRuntimeRecoverHeldTargetContinuesAfterRowError(t *testing.T) {
	p := &remoteTransportProbe{}
	f := remoteFixture(t, p, RuntimeReconcileSchedule{})
	f.start(t)
	_, err := f.service.remoteUnknownInternal(t.Context(), f.ref, "transport")
	require.NoError(t, err)
	broken := f.receipt(t)
	broken.RunID = "00000000-0000-4000-8000-000000000000"
	broken.GateScope = new("stack:example-other")
	require.NoError(t, f.store.svc.db.Create(&broken).Error)
	f.control.mu.Lock()
	f.control.recovering[broken.RunID] = true
	f.control.mu.Unlock()
	p.query = func(_ context.Context, id a.RuntimeRunIdentity) (a.RuntimeExecutionOutcome, error) {
		return remoteAnswer(id, f.store.now, a.StatusSuccess), nil
	}
	require.ErrorIs(t, f.control.RecoverHeldTarget(t.Context(), f.ref.Store), ErrRuntimeRecoveryInProgress)
	require.NoError(t, WaitRuntimeRemoteIdleForTest(t.Context(), f.control, f.ref.RunID))
	require.NotNil(t, f.receipt(t).SettledAt, "the healthy held run must still settle")
	require.EqualValues(t, 1, p.executes.Load())
}

func TestRuntimeRecoverHeldTargetCoalescesAndDebounces(t *testing.T) {
	p := &remoteTransportProbe{}
	var elapsed atomic.Int64
	base := time.Now()
	f := remoteFixture(t, p, RuntimeReconcileSchedule{Now: func() time.Time { return base.Add(time.Duration(elapsed.Load())) }})
	f.start(t)
	before := p.queries.Load()
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	p.query = func(ctx context.Context, _ a.RuntimeRunIdentity) (a.RuntimeExecutionOutcome, error) {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
		return a.RuntimeExecutionOutcome{}, ErrRuntimeTransport
	}
	require.NoError(t, f.control.RecoverHeldTarget(t.Context(), f.ref.Store))
	b1Await(t, entered)
	for range 10 {
		require.NoError(t, f.control.RecoverHeldTarget(t.Context(), f.ref.Store))
	}
	close(release)
	require.NoError(t, WaitRuntimeRemoteIdleForTest(t.Context(), f.control, f.ref.RunID))
	require.EqualValues(t, 3, p.queries.Load()-before)
	for range 10 {
		require.NoError(t, f.control.RecoverHeldTarget(t.Context(), f.ref.Store))
	}
	require.NoError(t, WaitRuntimeRemoteIdleForTest(t.Context(), f.control, f.ref.RunID))
	require.EqualValues(t, 3, p.queries.Load()-before)
	elapsed.Store(int64(31 * time.Second))
	require.NoError(t, f.control.RecoverHeldTarget(t.Context(), f.ref.Store))
	require.NoError(t, WaitRuntimeRemoteIdleForTest(t.Context(), f.control, f.ref.RunID))
	require.EqualValues(t, 6, p.queries.Load()-before)
	require.EqualValues(t, 1, p.executes.Load())
}

// WaitRuntimeRemoteIdleForTest joins the actual dispatch/query/follow effects,
// including a replacement registered by a departing task.
func WaitRuntimeRemoteIdleForTest(ctx context.Context, c *RuntimeRemoteCoordinator, run string) error {
	for {
		c.mu.Lock()
		tasks := []*runtimeRemoteTaskInternal{c.dispatch[run], c.queries[run], c.follows[run]}
		c.mu.Unlock()
		idle := true
		for _, task := range tasks {
			if task != nil {
				idle = false
				select {
				case <-task.done:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		if idle {
			return nil
		}
	}
}
