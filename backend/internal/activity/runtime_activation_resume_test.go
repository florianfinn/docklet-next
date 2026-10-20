// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Pause ordering and repeated or failed runtime resumption proofs.

package activity

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRuntimeActivationPausedStartPreservesPending(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	f := remoteFixture(t, &remoteTransportProbe{}, RuntimeReconcileSchedule{})
	require.NoError(t, f.control.Close(ctx))
	_, err := f.control.Start(ctx, f.ref, f.commandValue.Options)
	require.ErrorIs(t, err, ErrRuntimeRecoveryInProgress)
	require.NoError(t, f.control.WaitDispatch(ctx, f.ref.RunID))
	row := f.receipt(t)
	require.Equal(t, new("pending"), row.DispatchState, "closed coordinator must not commit an unsent dispatch")
	require.Nil(t, row.DispatchedAt)
	require.Nil(t, row.UnknownReason)
	require.Zero(t, f.probe.executes.Load())
}

func TestRuntimeActivationDispatchTransactionOrdersClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	f := remoteFixture(t, &remoteTransportProbe{}, RuntimeReconcileSchedule{})
	entered, release := make(chan struct{}), make(chan struct{})
	const name = "fixture-dispatch-transaction"
	var blocked atomic.Bool
	require.NoError(t, f.store.svc.db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "runtime_receipts" && blocked.CompareAndSwap(false, true) {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				tx.AddError(ctx.Err())
			}
		}
	}))
	startDone := make(chan error, 1)
	go func() { _, err := f.control.Start(ctx, f.ref, f.commandValue.Options); startDone <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("dispatch transaction not entered")
	}
	acquired := f.control.mu.TryLock()
	if acquired {
		f.control.mu.Unlock()
	}
	// Release the barrier even when this assertion is mutated red.
	close(release)
	require.False(t, acquired, "pause mutex must cover dispatch transaction and task registration")
	closeDone := make(chan error, 1)
	go func() { closeDone <- f.control.Close(ctx) }()
	select {
	case err := <-startDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("Start did not join")
	}
	select {
	case err := <-closeDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("Close did not join")
	}
	require.NoError(t, f.store.svc.db.Callback().Query().Remove(name))
	require.NoError(t, f.control.WaitDispatch(ctx, f.ref.RunID))
	require.Equal(t, new("dispatching"), f.receipt(t).DispatchState)
	before := f.probe.executes.Load()
	require.NoError(t, f.control.Startup(ctx))
	require.NoError(t, f.control.WaitDispatch(ctx, f.ref.RunID))
	require.Equal(t, before, f.probe.executes.Load(), "recovery never replays a committed dispatch")
}

func TestRuntimeActivationResumeFailureAndRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	f := remoteFixture(t, &remoteTransportProbe{}, RuntimeReconcileSchedule{})
	require.NoError(t, f.service.BindActivityOwner())
	h, err := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{ApplicationContext: ctx,
		Store: func(context.Context, string) (RuntimeStoreBinding, error) { return f.commandValue.Store, nil },
		PrepareTarget: func(_ context.Context, _ string, target RuntimeTarget) (RuntimeResolvedTarget, error) {
			return RuntimeResolvedTarget{Target: target}, nil
		}})
	require.NoError(t, err)
	h.SetReady(true)
	require.NoError(t, f.store.svc.PrepareRuntimeRestore(ctx))
	const name = "fixture-resume-read-fault"
	fault := errors.New("synthetic recovery read failure")
	require.NoError(t, f.store.svc.db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "runtime_receipts" {
			tx.AddError(fault)
		}
	}))
	err = f.store.svc.ResumeRuntimeIntake(ctx)
	require.ErrorIs(t, err, fault)
	require.False(t, h.ready.Load())
	require.True(t, f.service.intakePaused)
	require.True(t, f.service.storagePaused)
	require.True(t, f.control.paused)
	require.NoError(t, f.store.svc.db.Callback().Query().Remove(name))
	require.NoError(t, f.store.svc.ResumeRuntimeIntake(ctx))
	require.True(t, h.ready.Load())
	require.False(t, f.service.intakePaused)
	require.False(t, f.service.storagePaused)
	row := f.receipt(t)
	require.Equal(t, new("interrupted"), row.HubDisposition)
	require.NoError(t, f.store.svc.ResumeRuntimeIntake(ctx))
	require.Equal(t, row, f.receipt(t), "repeated resume does not rewrite or restart recovery")
	require.Zero(t, f.probe.executes.Load())
	require.Equal(t, activitytypes.StatusFailed, matrixRow(t, f.store.svc.db.DB, f.ref.RunID).Status)
}

func TestRuntimeActivationShutdownDuringResumeStaysClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	f := remoteFixture(t, &remoteTransportProbe{}, RuntimeReconcileSchedule{})
	life, stop := context.WithCancel(ctx)
	defer stop()
	f.control.lifetime = life
	h, err := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{ApplicationContext: life,
		Store: func(context.Context, string) (RuntimeStoreBinding, error) { return f.commandValue.Store, nil },
		PrepareTarget: func(_ context.Context, _ string, target RuntimeTarget) (RuntimeResolvedTarget, error) {
			return RuntimeResolvedTarget{Target: target}, nil
		}})
	require.NoError(t, err)
	h.SetReady(true)
	require.NoError(t, f.store.svc.PrepareRuntimeRestore(ctx))
	const name = "fixture-shutdown-during-resume"
	require.NoError(t, f.store.svc.db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "runtime_receipts" {
			stop()
		}
	}))
	defer func() { require.NoError(t, f.store.svc.db.Callback().Query().Remove(name)) }()
	err = f.store.svc.ResumeRuntimeIntake(ctx)
	require.ErrorIs(t, err, context.Canceled, "application shutdown during recovery cannot reopen readiness")
	require.False(t, h.ready.Load())
	require.True(t, f.service.intakePaused)
	require.True(t, f.service.storagePaused)
	require.True(t, f.control.paused)
	require.ErrorIs(t, f.store.svc.ResumeRuntimeIntake(ctx), context.Canceled)
	require.False(t, h.ready.Load())
	require.Zero(t, f.probe.executes.Load())
}
