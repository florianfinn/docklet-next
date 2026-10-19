// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Real SQLite admission, identity, scope and conditional transition proofs.

package activity

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRuntimeAdmissionRetainedRetryAndJoin(t *testing.T) {
	f := b1Fixture(t)
	command := f.command()
	command.Key = "6ba7b810-9dad-11d1-80b4-00c04fd430c8" // Any UUID version is opaque.
	first, err := f.service.Admit(t.Context(), command)
	require.NoError(t, err)
	require.Equal(t, activitytypes.CodeRunStarted, first.Code)
	require.Equal(t, first.ActivityID, first.Activity.ID)
	require.Equal(t, activitytypes.RuntimeOutcomeUnknown, first.Activity.OutcomeState)
	require.Equal(t, "Original operator", first.Activity.StartedBy.DisplayName)
	before := runtimeBoundarySnapshot(t, f.store)
	require.Len(t, before.Messages, 1)
	require.Equal(t, string(activitytypes.CodeRunStarted), before.Messages[0].Code)
	require.Equal(t, command.Actor.ID, before.Messages[0].Payload["actorId"])
	require.Equal(t, before.Receipts[0].AdmittedAt, new(before.Messages[0].CreatedAt))
	require.Equal(t, time.UTC, before.Messages[0].CreatedAt.Location())
	require.Equal(t, before.Messages[0].CreatedAt.Truncate(time.Microsecond), before.Messages[0].CreatedAt)
	f.resolver.resolve = func(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error) {
		return RuntimeResolvedTarget{}, errors.New("fixture offline")
	}
	for _, age := range []time.Duration{10 * time.Minute, 11 * time.Minute, 25 * time.Hour} {
		f.store.now = f.store.now.Add(age)
		command.ProposedRunID = uuid.NewString()
		command.Target.Name = new("Changed display")
		retry, err := f.service.Admit(t.Context(), command)
		require.NoError(t, err)
		require.Equal(t, activitytypes.CodeRunJoined, retry.Code)
		require.Equal(t, first.Activity, retry.Activity)
		require.Equal(t, first.ActivityID, retry.ActivityID)
		require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	}
	require.EqualValues(t, 1, f.resolver.calls.Load())
	joinedCommand := command
	joinedCommand.Key = uuid.NewString()
	joinedCommand.Actor.ID, joinedCommand.Actor.Username = "example-joiner", "Joining operator"
	joined, err := f.service.Admit(t.Context(), joinedCommand)
	require.NoError(t, err)
	require.Equal(t, first.ActivityID, joined.ActivityID)
	require.Equal(t, activitytypes.CodeRunJoined, joined.Code)
	after := runtimeBoundarySnapshot(t, f.store)
	require.Len(t, after.Activities, 1)
	require.Equal(t, before.Activities, after.Activities)
	require.Equal(t, before.Receipts[0].CallerID, after.Receipts[0].CallerID)
	require.Equal(t, runtimeTimeInternal(f.store.now), after.Receipts[0].FinalWriteAt)
	require.Len(t, after.Bindings, 2)
	var ownBinding RuntimeKeyBinding
	require.NoError(t, f.store.svc.db.First(&ownBinding, "idempotency_key = ?", joinedCommand.Key).Error)
	require.Equal(t, joinedCommand.Actor.ID, ownBinding.CallerID)
	require.Equal(t, joinedCommand.Actor.Username, *ownBinding.CallerName)
	require.Len(t, after.Messages, 2)
	var joinMessage ActivityMessage
	require.NoError(t, f.store.svc.db.First(&joinMessage, "code = ?", string(activitytypes.CodeRunJoined)).Error)
	require.Equal(t, joinedCommand.Actor.ID, joinMessage.Payload["actorId"])
	f.store.now = f.store.now.Add(time.Hour)
	_, err = f.service.Admit(t.Context(), joinedCommand)
	require.NoError(t, err)
	require.Equal(t, after, runtimeBoundarySnapshot(t, f.store))
	require.EqualValues(t, 1, f.resolver.calls.Load(), "durable joins require no fresh effect identity read")
}

func TestRuntimeAdmissionIdentityMismatchNonDisclosing(t *testing.T) {
	f := b1Fixture(t)
	command := f.command()
	first, err := f.service.Admit(t.Context(), command)
	require.NoError(t, err)
	before := runtimeBoundarySnapshot(t, f.store)
	for _, change := range []string{"caller", "caller-and-target", "manager", "registration", "executor", "store"} {
		t.Run(change, func(t *testing.T) {
			c := command
			switch change {
			case "caller":
				c.Actor.ID = "example-other-caller"
			case "caller-and-target":
				c.Actor.ID = "example-other-caller"
				c.Target.ID = "example-other-target"
			case "manager":
				c.Store.ManagerID = "example-other-manager"
			case "registration":
				c.Store.RegistrationID = "example-other-registration"
			case "executor":
				c.Store.ExecutorGeneration = uuid.NewString()
			case "store":
				c.Store.StoreGeneration = uuid.NewString()
			}
			result, err := f.service.Admit(t.Context(), c)
			b1Mismatch(t, result, err, first.ActivityID)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			// A different key must not implicitly bind into a foreign continuity tuple.
			if change != "caller" && change != "caller-and-target" {
				c.Key = uuid.NewString()
				result, err = f.service.Admit(t.Context(), c)
				b1Mismatch(t, result, err, first.ActivityID)
				require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
				c.Operation = RuntimeDownProject
				result, err = f.service.Admit(t.Context(), c)
				b1Mismatch(t, result, err, first.ActivityID)
				require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			}
		})
	}
}

func TestRuntimeAdmissionExactBusyCancellingAndUnknown(t *testing.T) {
	f := b1Fixture(t)
	command := f.command()
	first, err := f.service.Admit(t.Context(), command)
	require.NoError(t, err)
	id := b1Identity(t, command, first.ActivityID)
	for _, change := range []string{"operation", "options", "unknown"} {
		c := command
		c.Key = uuid.NewString()
		switch change {
		case "operation":
			c.Operation = RuntimeDownProject
		case "options":
			c.Options.Deploy = &projecttypes.DeployOptions{RecreateVolumes: true}
		case "unknown":
			b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
				_, err := runtimeEvidenceTxInternal(tx, now, id, runtimeEvidenceInternal{State: "unknown", UnknownReason: new("executor-unavailable")})
				return err
			})
			c.Operation = RuntimeRestartProject
		}
		before := runtimeBoundarySnapshot(t, f.store)
		result, err := f.service.Admit(t.Context(), c)
		b1Zero(t, result)
		var busy *GateConflict
		require.ErrorAs(t, err, &busy)
		require.Equal(t, string(activitytypes.CodeRunBusy), busy.Code)
		require.Equal(t, first.ActivityID, busy.RunID)
		require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	}
	b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
		_, err := runtimeCancelIntentTxInternal(tx, now, id)
		return err
	})
	before := runtimeBoundarySnapshot(t, f.store)
	c := command
	c.Key = uuid.NewString()
	result, err := f.service.Admit(t.Context(), c)
	b1Zero(t, result)
	var cancelling *GateConflict
	require.ErrorAs(t, err, &cancelling)
	require.Equal(t, string(activitytypes.CodeRunCancelling), cancelling.Code)
	retry, err := f.service.Admit(t.Context(), command)
	require.NoError(t, err)
	require.Equal(t, first.ActivityID, retry.ActivityID)
	require.True(t, retry.Activity.CancelRequested)
	require.Equal(t, activitytypes.RuntimeOutcomeUnknown, retry.Activity.OutcomeState)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
}

func TestRuntimeAdmissionParentBothDirections(t *testing.T) {
	for _, firstKind := range []string{"stack", "container"} {
		for _, unknown := range []bool{false, true} {
			name := firstKind
			if unknown {
				name += "-unknown"
			}
			t.Run(name, func(t *testing.T) {
				f := b1Fixture(t)
				stack := f.command()
				child := f.command()
				child.Operation = RuntimeStartContainer
				child.Target = RuntimeTarget{Kind: "container", ID: "example-child", ParentProjectID: new(stack.Target.ID)}
				one, two := stack, child
				if firstKind == "container" {
					one, two = child, stack
				}
				admitted, err := f.service.Admit(t.Context(), one)
				require.NoError(t, err)
				if unknown {
					b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
						_, err := runtimeEvidenceTxInternal(tx, now, b1Identity(t, one, admitted.ActivityID), runtimeEvidenceInternal{State: "unknown", UnknownReason: new("executor-unavailable")})
						return err
					})
				}
				before := runtimeBoundarySnapshot(t, f.store)
				result, err := f.service.Admit(t.Context(), two)
				b1Zero(t, result)
				var busy *GateConflict
				require.ErrorAs(t, err, &busy)
				require.Equal(t, string(activitytypes.CodeRunBusy), busy.Code)
				require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
				require.EqualValues(t, 1, f.resolver.calls.Load(), "held parent conflict is durable-first")
			})
		}
	}
}

func TestRuntimeAdmissionSiblingsAndEnvironmentsIndependent(t *testing.T) {
	f := b1Fixture(t)
	first := f.command()
	first.Operation = RuntimeStartContainer
	first.Target = RuntimeTarget{Kind: "container", ID: "example-child-a", ParentProjectID: new("example-parent")}
	sibling := first
	sibling.Key = uuid.NewString()
	sibling.Target.ID = "example-child-b"
	standalone := first
	standalone.Key = uuid.NewString()
	standalone.Target = RuntimeTarget{Kind: "container", ID: "example-standalone"}
	remote := first
	remote.Key = first.Key
	remote.Store.EnvironmentID = "example-other-environment"
	remote.Store.RegistrationID = "example-other-registration"
	ids := map[string]bool{}
	for _, c := range []RuntimeCommand{first, sibling, standalone, remote} {
		result, err := f.service.Admit(t.Context(), c)
		require.NoError(t, err)
		require.Equal(t, activitytypes.CodeRunStarted, result.Code)
		require.False(t, ids[result.ActivityID])
		ids[result.ActivityID] = true
	}
	snapshot := runtimeBoundarySnapshot(t, f.store)
	require.Len(t, snapshot.Receipts, 4)
	require.Len(t, snapshot.Activities, 4)
	require.Len(t, snapshot.Messages, 4)
	require.Len(t, snapshot.Bindings, 4)
}

func TestRuntimeAdmissionConcurrentPools(t *testing.T) {
	for _, mode := range []string{"same-key", "different-callers-same-key", "new-key-join", "parent-stack-first", "parent-child-first", "shared-service"} {
		t.Run(mode, func(t *testing.T) {
			f := b1Fixture(t)
			ready := make(chan struct{}, 2)
			release := make(chan struct{})
			resolver := &b1ResolverInternal{resolve: func(ctx context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
				ready <- struct{}{}
				select {
				case <-release:
					return RuntimeResolvedTarget{Target: target}, nil
				case <-ctx.Done():
					return RuntimeResolvedTarget{}, ctx.Err()
				}
			}}
			f.service.resolver = resolver
			other := f.contender(t, resolver)
			if mode == "shared-service" {
				other = NewRuntimeService(f.store.svc, resolver)
			}
			one := f.command()
			two := one
			two.ProposedRunID = uuid.NewString()
			switch mode {
			case "different-callers-same-key":
				two.Actor.ID = "example-other-caller"
			case "new-key-join":
				two.Key = uuid.NewString()
				two.Actor.ID = "example-joiner"
			case "parent-stack-first", "parent-child-first":
				two.Key = uuid.NewString()
				two.Operation = RuntimeStartContainer
				two.Target = RuntimeTarget{Kind: "container", ID: "example-child", ParentProjectID: new(one.Target.ID)}
				if mode == "parent-child-first" {
					one, two = two, one
				}
			}
			type answer struct {
				result RuntimeAdmissionResult
				err    error
			}
			answers := make(chan answer, 2)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			go func() { r, e := f.service.Admit(ctx, one); answers <- answer{r, e} }()
			go func() { r, e := other.Admit(ctx, two); answers <- answer{r, e} }()
			b1Await(t, ready)
			b1Await(t, ready)
			close(release)
			a, b := <-answers, <-answers
			if a.err != nil || b.result.Code == activitytypes.CodeRunStarted {
				a, b = b, a
			}
			require.NoError(t, a.err)
			require.Equal(t, activitytypes.CodeRunStarted, a.result.Code)
			if mode == "different-callers-same-key" {
				b1Mismatch(t, b.result, b.err, a.result.ActivityID)
			} else if mode == "parent-stack-first" || mode == "parent-child-first" {
				b1Zero(t, b.result)
				var busy *GateConflict
				require.ErrorAs(t, b.err, &busy)
				require.Equal(t, string(activitytypes.CodeRunBusy), busy.Code)
			} else {
				require.NoError(t, b.err)
				require.Equal(t, a.result.ActivityID, b.result.ActivityID)
				require.Equal(t, activitytypes.CodeRunJoined, b.result.Code)
			}
			snapshot := runtimeBoundarySnapshot(t, f.store)
			require.Len(t, snapshot.Receipts, 1)
			require.Len(t, snapshot.Activities, 1)
			bindings, messages := 1, 1
			if mode == "new-key-join" {
				bindings, messages = 2, 2
			}
			require.Len(t, snapshot.Bindings, bindings)
			require.Len(t, snapshot.Messages, messages)
		})
	}
}

func TestRuntimeAdmissionResolveRaceFinalRecheck(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "join", true: "busy"}[conflict], func(t *testing.T) {
			f := b1Fixture(t)
			reached, release := make(chan struct{}), make(chan struct{})
			stalled := &b1ResolverInternal{resolve: func(ctx context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
				close(reached)
				select {
				case <-release:
					return RuntimeResolvedTarget{Target: target}, nil
				case <-ctx.Done():
					return RuntimeResolvedTarget{}, ctx.Err()
				}
			}}
			f.service.resolver = stalled
			other := f.contender(t, &b1ResolverInternal{})
			one := f.command()
			two := one
			two.Key = uuid.NewString()
			if conflict {
				two.Operation = RuntimeDownProject
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			type answer struct {
				r RuntimeAdmissionResult
				e error
			}
			done := make(chan answer, 1)
			go func() { r, e := f.service.Admit(ctx, one); done <- answer{r, e} }()
			b1Await(t, reached)
			// This must commit while the first resolver is blocked, proving I/O holds
			// neither the shared admission mutex nor a SQLite transaction.
			winner, err := other.Admit(ctx, two)
			require.NoError(t, err)
			before := runtimeBoundarySnapshot(t, f.store)
			close(release)
			loser := <-done
			if conflict {
				b1Zero(t, loser.r)
				var busy *GateConflict
				require.ErrorAs(t, loser.e, &busy)
				require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			} else {
				require.NoError(t, loser.e)
				require.Equal(t, winner.ActivityID, loser.r.ActivityID)
				require.Equal(t, activitytypes.CodeRunJoined, loser.r.Code)
				runtimeCount(t, f.store.svc.db.DB, &RuntimeReceipt{}, 1)
			}
		})
	}
}

func TestRuntimeAdmissionClockOneSamplePerTransaction(t *testing.T) {
	f := b1Fixture(t)
	var samples atomic.Int64
	f.store.svc.clock = func() time.Time { samples.Add(1); return f.store.now }
	c := f.command()
	_, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	require.EqualValues(t, 2, samples.Load(), "one read pass and one final admission transaction")
	_, err = f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	require.EqualValues(t, 3, samples.Load())
}

func TestRuntimeAdmissionConditionalTransitionMarkers(t *testing.T) {
	for _, mode := range []string{"local", "dispatch", "cancel", "foreign-local", "terminal"} {
		t.Run(mode, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			if mode == "foreign-local" {
				c.Store.ExecutorGeneration = uuid.NewString()
			}
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			id := b1Identity(t, c, admitted.ActivityID)
			call := func(fn func(*gorm.DB, time.Time, runtimeIdentityInternal) (*RuntimeReceipt, error)) (*RuntimeReceipt, error) {
				var row *RuntimeReceipt
				err := f.store.tx(func(tx *gorm.DB, now time.Time) error { var e error; row, e = fn(tx, now, id); return e })
				return row, err
			}
			dispatch := func(accepted bool) func(*gorm.DB, time.Time, runtimeIdentityInternal) (*RuntimeReceipt, error) {
				return func(tx *gorm.DB, now time.Time, id runtimeIdentityInternal) (*RuntimeReceipt, error) {
					return runtimeDispatchTxInternal(tx, now, id, accepted)
				}
			}
			before := runtimeBoundarySnapshot(t, f.store)
			switch mode {
			case "local":
				row, err := call(runtimeLocalExecutingTxInternal)
				runtimeMismatch(t, row, err, admitted.ActivityID)
				require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
				row, err = call(runtimeLocalAcceptTxInternal)
				require.NoError(t, err)
				require.Equal(t, "pending", *row.DispatchState)
				require.Nil(t, row.DispatchedAt)
				accepted := runtimeBoundarySnapshot(t, f.store)
				f.store.now = f.store.now.Add(time.Hour)
				_, err = call(runtimeLocalAcceptTxInternal)
				require.NoError(t, err)
				require.Equal(t, accepted, runtimeBoundarySnapshot(t, f.store))
				row, err = call(runtimeLocalExecutingTxInternal)
				require.NoError(t, err)
				require.Equal(t, new(runtimeTimeInternal(f.store.now)), row.ExecutorStartedAt)
				executing := runtimeBoundarySnapshot(t, f.store)
				f.store.now = f.store.now.Add(time.Hour)
				_, err = call(runtimeLocalExecutingTxInternal)
				require.NoError(t, err)
				require.Equal(t, executing, runtimeBoundarySnapshot(t, f.store))
				row, err = call(runtimeLocalAcceptTxInternal)
				runtimeMismatch(t, row, err, admitted.ActivityID)
			case "dispatch":
				row, err := call(dispatch(true))
				runtimeMismatch(t, row, err, admitted.ActivityID)
				require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
				row, err = call(dispatch(false))
				require.NoError(t, err)
				stamp := *row.DispatchedAt
				dispatched := runtimeBoundarySnapshot(t, f.store)
				f.store.now = f.store.now.Add(time.Hour)
				_, err = call(dispatch(false))
				require.NoError(t, err)
				require.Equal(t, dispatched, runtimeBoundarySnapshot(t, f.store))
				row, err = call(dispatch(true))
				require.NoError(t, err)
				require.Equal(t, new(stamp), row.DispatchedAt)
				acknowledged := runtimeBoundarySnapshot(t, f.store)
				f.store.now = f.store.now.Add(time.Hour)
				_, err = call(dispatch(true))
				require.NoError(t, err)
				require.Equal(t, acknowledged, runtimeBoundarySnapshot(t, f.store))
			case "cancel":
				f.store.now = f.store.now.Add(time.Hour)
				row, err := call(runtimeCancelIntentTxInternal)
				require.NoError(t, err)
				require.True(t, row.CancelRequested)
				cancelled := runtimeBoundarySnapshot(t, f.store)
				f.store.now = f.store.now.Add(time.Hour)
				_, err = call(runtimeCancelIntentTxInternal)
				require.NoError(t, err)
				require.Equal(t, cancelled, runtimeBoundarySnapshot(t, f.store))
				for _, fn := range []func(*gorm.DB, time.Time, runtimeIdentityInternal) (*RuntimeReceipt, error){runtimeLocalAcceptTxInternal, runtimeLocalExecutingTxInternal, dispatch(false), dispatch(true)} {
					row, err = call(fn)
					runtimeMismatch(t, row, err, admitted.ActivityID)
					require.Equal(t, cancelled, runtimeBoundarySnapshot(t, f.store))
				}
			case "foreign-local":
				for _, fn := range []func(*gorm.DB, time.Time, runtimeIdentityInternal) (*RuntimeReceipt, error){runtimeLocalAcceptTxInternal, runtimeLocalExecutingTxInternal} {
					row, err := call(fn)
					runtimeMismatch(t, row, err, admitted.ActivityID)
					require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
				}
			case "terminal":
				// Storage evidence only: no worker or composed settlement is claimed here.
				b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
					_, err := runtimeEvidenceTxInternal(tx, now, id, runtimeEvidenceInternal{State: "success", SettledAt: new(now)})
					return err
				})
				terminal := runtimeBoundarySnapshot(t, f.store)
				row, err := call(runtimeCancelIntentTxInternal)
				require.NoError(t, err)
				require.False(t, row.CancelRequested)
				for _, fn := range []func(*gorm.DB, time.Time, runtimeIdentityInternal) (*RuntimeReceipt, error){runtimeLocalAcceptTxInternal, runtimeLocalExecutingTxInternal, dispatch(false), dispatch(true)} {
					row, err = call(fn)
					runtimeMismatch(t, row, err, admitted.ActivityID)
				}
				require.Equal(t, terminal, runtimeBoundarySnapshot(t, f.store))
			}
		})
	}
}
