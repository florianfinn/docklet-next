// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Controlled executor barriers with real ActivityService, capture and migrated SQLite.

package activity

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRuntimeWorkerOneOwnerRetryJoinAndRequestDisconnect(t *testing.T) {
	type traceKey struct{}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var calls atomic.Int64
	observed := make(chan context.Context, 1)
	commands := make(chan RuntimeCommand, 1)
	f := b2Fixture(t, t.Context(), func(ctx context.Context, c RuntimeCommand, w io.Writer) error {
		calls.Add(1)
		observed <- ctx
		commands <- c
		close(entered)
		<-release
		_, err := io.WriteString(w, "complete\nlast partial")
		return err
	})
	c := f.command()
	request, disconnect := context.WithCancel(context.WithValue(t.Context(), traceKey{}, "example-trace"))
	defer disconnect()
	admitted, err := f.service.Admit(request, c)
	require.NoError(t, err)
	require.Equal(t, activitytypes.CodeRunStarted, admitted.Code)
	owner := b2Owner(t, f, admitted.ActivityID)
	b1Await(t, entered)
	work := <-observed
	canonical := <-commands
	require.Equal(t, admitted.ActivityID, work.Value(gateRunContextKey{}))
	require.Equal(t, "example-trace", work.Value(traceKey{}))
	require.Equal(t, c.Actor, canonical.Actor)
	require.Equal(t, c.Target, canonical.Target)
	require.True(t, utils.IsAppLifecycleContext(work))
	deadline, ok := work.Deadline()
	require.True(t, ok)
	require.LessOrEqual(t, time.Until(deadline), 30*time.Minute)
	require.Greater(t, time.Until(deadline), 29*time.Minute)
	disconnect()
	require.NoError(t, work.Err(), "request disconnect is viewer-only")
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Go(func() {
			joined := c
			joined.Key = uuid.NewString()
			joined.Actor.ID = "example-joiner"
			r, e := f.service.Admit(t.Context(), joined)
			require.NoError(t, e)
			require.Equal(t, admitted.ActivityID, r.ActivityID)
			require.Equal(t, activitytypes.CodeRunJoined, r.Code)
			r, e = f.service.Admit(t.Context(), c)
			require.NoError(t, e)
			require.Equal(t, admitted.ActivityID, r.ActivityID)
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, calls.Load())
	b2Held(t, f, c, admitted.ActivityID)
	before := runtimeBoundarySnapshot(t, f.store)
	require.Equal(t, new(c.Actor.ID), before.Receipts[0].CallerID)
	require.Equal(t, new(c.Actor.ID), before.Activities[0].StartedByUserID)
	require.Equal(t, "executing", *before.Receipts[0].ExecutorState)
	require.Nil(t, before.Receipts[0].DispatchedAt)
	require.Equal(t, "pending", *before.Receipts[0].DispatchState)
	wait, stop := context.WithCancel(t.Context())
	stop()
	_, err = f.service.WaitLocal(wait, b2Ref(c, admitted.ActivityID))
	require.ErrorIs(t, err, context.Canceled)
	select {
	case <-owner.attemptDone:
		t.Fatal("viewer cancellation released owner")
	default:
	}
	unblock()
	settled, err := b2Wait(t, f, c, admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, settled.Status)
	b2FinalCount(t, f, admitted.ActivityID, 1)
	after := runtimeBoundarySnapshot(t, f.store)
	require.Len(t, after.Activities[0].Output, 2)
	require.Equal(t, "last partial", after.Activities[0].Output[1].Text)
	// ACTUAL composed settlement precedes history clearing and offline compact retry.
	_, err = f.store.svc.DeleteHistory(t.Context(), "0")
	require.NoError(t, err)
	f.store.now = f.store.now.Add(11 * time.Minute)
	f.resolver.resolve = func(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error) {
		return RuntimeResolvedTarget{}, errors.New("fixture offline")
	}
	compact := runtimeBoundarySnapshot(t, f.store)
	retry, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	require.Equal(t, admitted.ActivityID, retry.ActivityID)
	require.Equal(t, settled.Status, retry.Activity.Status)
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, compact, runtimeBoundarySnapshot(t, f.store))
	require.Empty(t, compact.Activities)
}

func TestRuntimeWorkerAdmissionFailuresNeverRegister(t *testing.T) {
	for _, fault := range []string{"receipt", "activity", "binding", "message", "commit", "foreign-local", "invalid-config"} {
		t.Run(fault, func(t *testing.T) {
			var calls atomic.Int64
			f := b2Fixture(t, t.Context(), func(context.Context, RuntimeCommand, io.Writer) error { calls.Add(1); return nil })
			c := f.command()
			before := runtimeBoundarySnapshot(t, f.store)
			table := map[string]string{"receipt": "runtime_receipts", "activity": "activities", "binding": "runtime_key_bindings", "message": "activity_messages"}[fault]
			if table != "" {
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_b2_admission BEFORE INSERT ON "+table+" BEGIN SELECT RAISE(ABORT, 'fixture admission failure'); END").Error)
			}
			if fault == "commit" {
				b2Deferred(t, f, "NEW.code = 'run.started'")
			}
			if fault == "foreign-local" {
				c.Store.ExecutorGeneration = uuid.NewString()
			}
			if fault == "invalid-config" {
				f.service = NewRuntimeService(f.store.svc, f.resolver, RuntimeLocalConfig{Store: c.Store, Executor: b2ExecutorFunc(func(context.Context, RuntimeCommand, io.Writer) error { calls.Add(1); return nil })})
			}
			result, err := f.service.Admit(t.Context(), c)
			require.Error(t, err)
			b1Zero(t, result)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			require.Empty(t, f.service.owners)
			require.Zero(t, calls.Load())
		})
	}
}

func TestRuntimeWorkerDurableCancelBeforeRegistrationAndExecution(t *testing.T) {
	for _, phase := range []string{"registration", "executing"} {
		t.Run(phase, func(t *testing.T) {
			var calls atomic.Int64
			f := b2Fixture(t, t.Context(), func(context.Context, RuntimeCommand, io.Writer) error { calls.Add(1); return nil })
			c := f.command()
			// A SQLite trigger models an already durable concurrent cancellation at the
			// last write before registration or the accepted-to-executing decision.
			predicate := "NEW.code = 'run.started'"
			statement := "AFTER INSERT ON activity_messages WHEN " + predicate
			idColumn := "NEW.activity_id"
			if phase == "executing" {
				statement = "AFTER UPDATE OF executor_state ON runtime_receipts WHEN NEW.executor_state = 'accepted'"
				idColumn = "NEW.run_id"
			}
			require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_b2_precancel "+statement+" BEGIN UPDATE runtime_receipts SET cancel_requested = 1, cancel_requested_at = admitted_at WHERE run_id = "+idColumn+"; END").Error)
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			settled, err := b2Wait(t, f, c, admitted.ActivityID)
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusCancelled, settled.Status)
			require.Equal(t, "runtime.cancelled", settled.MessageCode)
			require.True(t, settled.CancelRequested)
			require.Zero(t, calls.Load(), "durable intent must prevent CONTROLLED EXECUTOR entry")
			after := runtimeBoundarySnapshot(t, f.store)
			require.Nil(t, after.Receipts[0].ExecutorStartedAt)
			require.Nil(t, after.Receipts[0].DispatchedAt)
			b2FinalCount(t, f, admitted.ActivityID, 1)
		})
	}
}

func TestRuntimeWorkerCancelIntentAtomicBeforeSignal(t *testing.T) {
	for _, fault := range []string{"intent", "message", "commit", "none"} {
		t.Run(fault, func(t *testing.T) {
			entered, seen, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			contexts := make(chan context.Context, 1)
			f := b2Fixture(t, t.Context(), func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error {
				contexts <- ctx
				close(entered)
				select {
				case <-ctx.Done():
					close(seen)
				case <-release:
					return nil
				}
				<-release
				return ctx.Err()
			})
			c := f.command()
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			b1Await(t, entered)
			work := <-contexts
			before := runtimeBoundarySnapshot(t, f.store)
			switch fault {
			case "intent":
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_b2_fault BEFORE UPDATE OF cancel_requested ON runtime_receipts BEGIN SELECT RAISE(ABORT, 'fixture cancel intent'); END").Error)
			case "message":
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_b2_fault BEFORE INSERT ON activity_messages WHEN NEW.code = 'run.cancelling' BEGIN SELECT RAISE(ABORT, 'fixture cancel audit'); END").Error)
			case "commit":
				b2Deferred(t, f, "NEW.code = 'run.cancelling'")
			}
			result, err := f.service.Cancel(t.Context(), b2Ref(c, admitted.ActivityID), c.Actor)
			if fault != "none" {
				require.Error(t, err)
				require.Equal(t, activitytypes.RuntimeSnapshot{}, result)
				require.NoError(t, work.Err(), "failed intent must never signal owned context")
				require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
				select {
				case <-seen:
					t.Fatal("signaled before commit")
				default:
				}
				require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER fixture_b2_fault").Error)
				result, err = f.service.Cancel(t.Context(), b2Ref(c, admitted.ActivityID), c.Actor)
			}
			require.NoError(t, err)
			require.True(t, result.CancelRequested)
			b1Await(t, seen)
			intent := runtimeBoundarySnapshot(t, f.store)
			require.True(t, intent.Receipts[0].CancelRequested)
			require.NotNil(t, intent.Receipts[0].CancelRequestedAt)
			repeated, err := f.service.Cancel(t.Context(), b2Ref(c, admitted.ActivityID), c.Actor)
			require.NoError(t, err)
			require.Equal(t, result, repeated)
			require.Equal(t, intent, runtimeBoundarySnapshot(t, f.store))
			b2Held(t, f, c, admitted.ActivityID)
			unblock()
			settled, err := b2Wait(t, f, c, admitted.ActivityID)
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusCancelled, settled.Status)
			b2FinalCount(t, f, admitted.ActivityID, 1)
			terminal := runtimeBoundarySnapshot(t, f.store)
			late, err := f.service.Cancel(t.Context(), b2Ref(c, admitted.ActivityID), c.Actor)
			require.NoError(t, err)
			require.Equal(t, settled, late)
			require.Equal(t, terminal, runtimeBoundarySnapshot(t, f.store))
		})
	}
}

func TestRuntimeWorkerInterruptionHoldsUntilKnownChildJoins(t *testing.T) {
	for _, mode := range []string{"user", "application", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			app, stop := context.WithCancel(t.Context())
			defer stop()
			var appDeadline *runtimeDeadlineFixture
			if mode == "deadline" {
				appDeadline = newRuntimeDeadlineFixture(t)
				app = appDeadline
			}
			entered, childStopped, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			deadlines := make(chan time.Time, 1)
			f := b2Fixture(t, app, func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error {
				deadline, _ := ctx.Deadline()
				deadlines <- deadline
				child := make(chan struct{})
				go func() { <-ctx.Done(); close(childStopped); <-release; close(child) }()
				close(entered)
				<-ctx.Done()
				<-child // Controlled executor's explicit child join.
				return ctx.Err()
			})
			c := f.command()
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			b1Await(t, entered)
			deadline := <-deadlines
			if mode == "deadline" {
				earlier, _ := app.Deadline()
				require.Equal(t, earlier, deadline)
			}
			if mode == "user" {
				_, err = f.service.Cancel(t.Context(), b2Ref(c, admitted.ActivityID), c.Actor)
				require.NoError(t, err)
			} else if mode == "application" {
				stop()
			} else if appDeadline != nil {
				appDeadline.expire(t, b2Owner(t, f, admitted.ActivityID).ctx)
			}
			b1Await(t, childStopped)
			b2Held(t, f, c, admitted.ActivityID)
			b2FinalCount(t, f, admitted.ActivityID, 0)
			unblock()
			settled, err := b2Wait(t, f, c, admitted.ActivityID)
			require.NoError(t, err)
			expected := activitytypes.StatusFailed
			code := "runtime.interrupted"
			if mode == "user" {
				expected = activitytypes.StatusCancelled
				code = "runtime.cancelled"
			}
			require.Equal(t, expected, settled.Status)
			require.Equal(t, code, settled.MessageCode)
			after := runtimeBoundarySnapshot(t, f.store)
			require.Equal(t, new(code), after.Receipts[0].ExecutorResultCode)
			require.Equal(t, code, after.Activities[0].MessageCode)
			require.Equal(t, activitytypes.RuntimeOutcomeKnown, settled.OutcomeState)
			b2FinalCount(t, f, admitted.ActivityID, 1)
		})
	}
}

// The capture calls this decorator at its actual AppendMessages boundary. It
// delegates to the real service; barriers never replace the whole workflow.
type b2OutputAppender struct {
	*ActivityService
	before func(context.Context, string, []activitylib.AppendMessageRequest) error
}

func (a *b2OutputAppender) AppendMessages(ctx context.Context, id string, req []activitylib.AppendMessageRequest) ([]activitytypes.Message, error) {
	if a.before != nil {
		if err := a.before(ctx, id, req); err != nil {
			return nil, err
		}
	}
	return a.ActivityService.AppendMessages(ctx, id, req)
}

func TestRuntimeWorkerOutputDrainBeforeTerminal(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	returned := make(chan struct{})
	f := b2Fixture(t, t.Context(), func(_ context.Context, _ RuntimeCommand, w io.Writer) error {
		_, err := io.WriteString(w, "first\npartial")
		close(returned)
		return err
	})
	observed := make(chan context.Context, 8)
	var started sync.Once
	f.service.outputAppender = &b2OutputAppender{ActivityService: f.store.svc, before: func(ctx context.Context, id string, req []activitylib.AppendMessageRequest) error {
		observed <- ctx
		started.Do(func() { close(entered) })
		<-release
		return nil
	}}
	c := f.command()
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	b1Await(t, entered)
	b1Await(t, returned)
	appendCtx := <-observed
	require.Equal(t, admitted.ActivityID, appendCtx.Value(gateRunContextKey{}))
	deadline, ok := appendCtx.Deadline()
	require.True(t, ok)
	require.LessOrEqual(t, time.Until(deadline), 10*time.Second)
	b2Held(t, f, c, admitted.ActivityID)
	b2FinalCount(t, f, admitted.ActivityID, 0)
	owner := b2Owner(t, f, admitted.ActivityID)
	select {
	case <-owner.attemptDone:
		t.Fatal("terminalized before actual output join")
	default:
	}
	unblock()
	settled, err := b2Wait(t, f, c, admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, settled.Status)
	snap := runtimeBoundarySnapshot(t, f.store)
	require.Len(t, snap.Activities[0].Output, 2)
	require.Equal(t, "partial", snap.Activities[0].Output[1].Text)
	b2FinalCount(t, f, admitted.ActivityID, 1)
	// Database insertion order proves output precedes the sole terminal message.
	var codes []string
	require.NoError(t, f.store.svc.db.Raw("SELECT code FROM activity_messages WHERE activity_id = ? ORDER BY rowid", admitted.ActivityID).Scan(&codes).Error)
	require.Equal(t, []string{"run.started", "activity.message", "activity.message", "activity.success"}, codes)
	_, err = f.store.svc.AppendMessages(context.WithValue(t.Context(), gateRunContextKey{}, admitted.ActivityID), admitted.ActivityID, []AppendActivityMessageRequest{{Message: "too late"}})
	require.Error(t, err)
	require.Equal(t, snap, runtimeBoundarySnapshot(t, f.store))
}

func TestRuntimeWorkerRealAppendErrorAndOutputCap(t *testing.T) {
	for _, mode := range []string{"append-error", "cap", "workflow-error"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int64
			f := b2Fixture(t, t.Context(), func(_ context.Context, _ RuntimeCommand, w io.Writer) error {
				calls.Add(1)
				if mode == "workflow-error" {
					return errors.New("fixture workflow failure")
				}
				if mode == "cap" {
					for i := 0; i < 400; i++ {
						if _, err := fmt.Fprintf(w, "%04d %s\n", i, strings.Repeat("x", 1024)); err != nil {
							return err
						}
					}
					return nil
				}
				_, err := io.WriteString(w, "fails\n")
				if err != nil {
					return err
				}
				w.(*activitylib.Writer).Flush()
				return nil
			})
			if mode == "append-error" {
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_output_failure BEFORE INSERT ON activity_messages WHEN NEW.code = 'activity.message' BEGIN SELECT RAISE(ABORT, 'fixture real output error'); END").Error)
			}
			c := f.command()
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			settled, err := b2Wait(t, f, c, admitted.ActivityID)
			require.NoError(t, err)
			if mode == "cap" {
				require.Equal(t, activitytypes.StatusSuccess, settled.Status)
				snap := runtimeBoundarySnapshot(t, f.store)
				require.True(t, snap.Activities[0].OutputTruncated)
				require.Less(t, len(snap.Activities[0].Output), 400)
				encoded, err := snap.Activities[0].Output.Value()
				require.NoError(t, err)
				require.LessOrEqual(t, len(encoded.(string)), 256*1024)
			} else {
				require.Equal(t, activitytypes.StatusFailed, settled.Status)
				require.Equal(t, "runtime.failed", settled.MessageCode)
			}
			require.EqualValues(t, 1, calls.Load())
			b2FinalCount(t, f, admitted.ActivityID, 1)
		})
	}
}

func TestRuntimeWorkerUntrackedCancelAndLookupContinuity(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(fmt.Sprint(remote), func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			if remote {
				c.Store.ExecutorGeneration = uuid.NewString()
				c.Store.EnvironmentID = "example-remote"
			}
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			cancelled, err := f.service.Cancel(t.Context(), b2Ref(c, admitted.ActivityID), c.Actor)
			require.NoError(t, err)
			require.True(t, cancelled.CancelRequested)
			require.Equal(t, activitytypes.StatusRunning, cancelled.Status)
			b2FinalCount(t, f, admitted.ActivityID, 0)
			before := runtimeBoundarySnapshot(t, f.store)
			_, err = f.service.RetrySettlement(t.Context(), b2Ref(c, admitted.ActivityID))
			require.ErrorIs(t, err, ErrRuntimeNotSettled)
			foreign := b2Ref(c, admitted.ActivityID)
			foreign.Store.RegistrationID = "example-other"
			result, err := f.service.Cancel(t.Context(), foreign, c.Actor)
			require.Equal(t, activitytypes.RuntimeSnapshot{}, result)
			var mismatch *runtimeConflictInternal
			require.ErrorAs(t, err, &mismatch)
			require.NotContains(t, err.Error(), admitted.ActivityID)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err = f.service.Cancel(ctx, b2Ref(c, admitted.ActivityID), c.Actor)
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
		})
	}
}

func TestRuntimeWorkerRegistrationAndAdmissionCommitShareMutex(t *testing.T) {
	// The create callback is inside admission's transaction. Failed COMMIT tests
	// separately prove no registered owner escapes this boundary.
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	f := b2Fixture(t, t.Context(), func(context.Context, RuntimeCommand, io.Writer) error { close(entered); <-release; return nil })
	c := f.command()
	c.ProposedRunID = uuid.NewString()
	var checked atomic.Bool
	name := "fixture_b2_registration"
	require.NoError(t, f.store.svc.db.Callback().Create().After("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "activity_messages" && tx.Error == nil && !checked.Swap(true) {
			require.Empty(t, f.service.owners, "owner must not exist before admission commit")
		}
	}))
	defer func() { require.NoError(t, f.store.svc.db.Callback().Create().Remove(name)) }()
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	require.True(t, checked.Load())
	owner := b2Owner(t, f, admitted.ActivityID)
	require.Equal(t, c.ProposedRunID, owner.id.RunID)
	b1Await(t, entered)
	unblock()
	_, err = b2Wait(t, f, c, admitted.ActivityID)
	require.NoError(t, err)
}

func TestRuntimeWorkerSevenCanonicalCommands(t *testing.T) {
	for _, op := range []RuntimeOperation{RuntimeDeployProject, RuntimeDownProject, RuntimeRestartProject, RuntimeRedeployProject, RuntimeStartContainer, RuntimeStopContainer, RuntimeRestartContainer} {
		t.Run(string(op), func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			seen := make(chan RuntimeCommand, 1)
			var calls atomic.Int64
			f := b2Fixture(t, t.Context(), func(_ context.Context, c RuntimeCommand, _ io.Writer) error {
				calls.Add(1)
				seen <- c
				close(entered)
				<-release
				return nil
			})
			c := f.command()
			c.Operation = op
			if op == RuntimeStartContainer || op == RuntimeStopContainer || op == RuntimeRestartContainer {
				c.Target = RuntimeTarget{Kind: "container", ID: "example-container", ParentProjectID: new("example-parent")}
			}
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			b1Await(t, entered)
			got := <-seen
			require.Equal(t, c.Operation, got.Operation)
			require.Equal(t, c.Target, got.Target)
			require.Equal(t, c.Actor, got.Actor)
			retry, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			require.Equal(t, admitted.ActivityID, retry.ActivityID)
			unblock()
			settled, err := b2Wait(t, f, c, admitted.ActivityID)
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusSuccess, settled.Status)
			require.EqualValues(t, 1, calls.Load())
			b2FinalCount(t, f, admitted.ActivityID, 1)
		})
	}
}

func TestRuntimeWorkerViewerCauseDoesNotBecomeUserCancel(t *testing.T) {
	app, stop := context.WithCancel(t.Context())
	defer stop()
	request, disconnect := context.WithCancelCause(t.Context())
	defer disconnect(nil)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	seen := make(chan struct{})
	f := b2Fixture(t, app, func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error {
		close(entered)
		<-ctx.Done()
		close(seen)
		<-release
		return ctx.Err()
	})
	c := f.command()
	admitted, err := f.service.Admit(request, c)
	require.NoError(t, err)
	b1Await(t, entered)
	disconnect(activitylib.ErrCanceled)
	stop()
	b1Await(t, seen)
	b2Held(t, f, c, admitted.ActivityID)
	unblock()
	settled, err := b2Wait(t, f, c, admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusFailed, settled.Status)
	require.Equal(t, "runtime.interrupted", settled.MessageCode)
	require.False(t, settled.CancelRequested)
}
