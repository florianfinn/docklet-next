// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Receipt-origin identity, local resolution, limiter and cancellation regression evidence.

package activity

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRuntimeReceiverOriginalReceiptBeforeResolver(t *testing.T) {
	f, p := receiverProbeFixture(t)
	e := receiverEnvelope(t, f.command())
	first := receiverExecute(t, f, e)
	require.Equal(t, activitytypes.StatusQueued, first.Snapshot.Status)
	require.Equal(t, activitytypes.RuntimeOutcomeKnown, first.Snapshot.OutcomeState)
	b1Await(t, p.entered)
	before := runtimeBoundarySnapshot(t, f.store)
	require.Equal(t, "executing", *before.Receipts[0].ExecutorState)
	require.Equal(t, "pending", *before.Receipts[0].DispatchState)
	require.Nil(t, before.Receipts[0].DispatchedAt)
	require.Equal(t, "Original operator", *before.Activities[0].StartedByDisplayName)
	require.NotContains(t, before.Messages[0].Payload, "key")
	require.Len(t, before.Messages[0].Payload, 2)
	f.resolver.resolve = func(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error) {
		return RuntimeResolvedTarget{}, errors.New("fixture offline")
	}
	duplicate := e
	duplicate.Target.Name = new("Changed display")
	duplicate.Attribution = activitytypes.RuntimeAttribution{ID: "example-other-actor", Username: "Other", DisplayName: new("Other display")}
	retry := receiverExecute(t, f, duplicate)
	require.Equal(t, "example-owner", retry.Snapshot.StartedBy.UserID)
	require.Equal(t, "Original operator", retry.Snapshot.StartedBy.DisplayName)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	require.EqualValues(t, 1, f.resolver.calls.Load())
	require.EqualValues(t, 1, p.calls.Load())
	p.unblock()
	settled := receiverWait(t, f, e)
	require.Equal(t, activitytypes.StatusSuccess, settled.Status)
	terminal := runtimeBoundarySnapshot(t, f.store)
	retry = receiverExecute(t, f, duplicate)
	require.Equal(t, activitytypes.RuntimeAnswerTerminal, retry.State)
	require.Equal(t, settled, *retry.Snapshot)
	require.Equal(t, terminal, runtimeBoundarySnapshot(t, f.store))
	receiverCounts(t, f, 1, 1, 1)
	// Compact evidence survives full history clear and remains receipt-authoritative.
	_, err := f.store.svc.DeleteHistory(t.Context(), "0")
	require.NoError(t, err)
	retry = receiverExecute(t, f, duplicate)
	require.Equal(t, e.Identity, retry.Identity)
	require.Equal(t, activitytypes.StatusSuccess, retry.Snapshot.Status)
	receiverCounts(t, f, 1, 0, 0)
	require.EqualValues(t, 1, p.calls.Load())
}

func TestRuntimeReceiverQueuedDuplicateAndDurableCancel(t *testing.T) {
	f, p := receiverProbeFixture(t)
	legacy := slotsLegacy(t, f.b1FixtureInternal, "0")
	e := receiverEnvelope(t, f.command())
	receiverExecute(t, f, e)
	// The initial accepted state is atomic; no polling a worker transition is needed.
	before := runtimeBoundarySnapshot(t, f.store)
	require.Equal(t, "accepted", *before.Receipts[0].ExecutorState)
	require.Nil(t, before.Receipts[0].ExecutorStartedAt)
	retry := receiverExecute(t, f, e)
	require.Equal(t, activitytypes.StatusQueued, retry.Snapshot.Status)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	require.Zero(t, p.calls.Load())
	for i := 0; i < 2; i++ {
		result, err := f.receiver.Cancel(t.Context(), e.Identity, activitytypes.RuntimeAttribution{ID: "example-canceller"})
		require.NoError(t, err)
		require.True(t, result.Snapshot.CancelRequested)
	}
	snapshot := receiverWait(t, f, e)
	require.Equal(t, activitytypes.StatusCancelled, snapshot.Status)
	row := b3Receipt(t, f.b1FixtureInternal, e.Identity.RunID)
	require.Nil(t, row.ExecutorStartedAt)
	require.Zero(t, p.calls.Load())
	slotsHeld(t, f.b1FixtureInternal, 1) // Only the legacy holder remains.
	slotsCompleteLegacy(t, f.b1FixtureInternal, legacy)
	terminal := runtimeBoundarySnapshot(t, f.store)
	result, err := f.receiver.Cancel(t.Context(), e.Identity, activitytypes.RuntimeAttribution{ID: "example-canceller"})
	require.NoError(t, err)
	require.Equal(t, activitytypes.RuntimeAnswerTerminal, result.State)
	require.Equal(t, terminal, runtimeBoundarySnapshot(t, f.store))
	var cancels int64
	require.NoError(t, f.store.svc.db.Model(&ActivityMessage{}).Where("code = 'run.cancelling'").Count(&cancels).Error)
	require.EqualValues(t, 1, cancels)
	receiverCounts(t, f, 1, 2, 1)
}

func TestRuntimeReceiverDifferentUUIDNeverJoins(t *testing.T) {
	f, p := receiverProbeFixture(t)
	e := receiverEnvelope(t, f.command())
	receiverExecute(t, f, e)
	b1Await(t, p.entered)
	for _, cancelling := range []bool{false, true} {
		if cancelling {
			_, err := f.receiver.Cancel(t.Context(), e.Identity, e.Attribution)
			require.NoError(t, err)
		}
		before := runtimeBoundarySnapshot(t, f.store)
		other := e
		other.Identity.RunID = uuid.NewString()
		result, err := f.receiver.Execute(t.Context(), other)
		receiverZero(t, result)
		var busy *GateConflict
		require.ErrorAs(t, err, &busy)
		want := activitytypes.CodeRunBusy
		if cancelling {
			want = activitytypes.CodeRunCancelling
		}
		require.Equal(t, string(want), busy.Code)
		require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	}
	p.unblock()
	require.Equal(t, activitytypes.StatusCancelled, receiverWait(t, f, e).Status)
	require.EqualValues(t, 1, p.calls.Load())
}

func TestRuntimeReceiverParentsAndSiblings(t *testing.T) {
	for _, firstStack := range []bool{true, false} {
		t.Run(map[bool]string{true: "stack-first", false: "child-first"}[firstStack], func(t *testing.T) {
			var calls atomic.Int64
			entered := make(chan struct{}, 2)
			f := receiverNew(t, func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error {
				calls.Add(1)
				entered <- struct{}{}
				<-ctx.Done()
				return ctx.Err()
			})
			c := f.command()
			c.Operation = RuntimeDownProject
			if !firstStack {
				c.Operation = RuntimeStartContainer
				c.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64), ParentProjectID: new("example-stack")}
			}
			e := receiverEnvelope(t, c)
			receiverExecute(t, f, e)
			b1Await(t, entered)
			next := f.command()
			next.Operation = RuntimeStartContainer
			next.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("b", 64), ParentProjectID: new("example-stack")}
			if !firstStack {
				next.Operation = RuntimeDownProject
				next.Target = RuntimeTarget{Kind: "stack", ID: "example-stack"}
			}
			before := runtimeBoundarySnapshot(t, f.store)
			result, err := f.receiver.Execute(t.Context(), receiverEnvelope(t, next))
			receiverZero(t, result)
			var busy *GateConflict
			require.ErrorAs(t, err, &busy)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			if !firstStack {
				next.Operation = RuntimeStartContainer
				next.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("b", 64), ParentProjectID: new("example-stack")}
				receiverExecute(t, f, receiverEnvelope(t, next))
				receiverCounts(t, f, 2, 2, 2)
			}
		})
	}
}

func TestRuntimeReceiverFreshTargetRefusals(t *testing.T) {
	for _, mode := range []string{"offline", "unresolved", "parent", "kind", "id", "invalid-name", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			f, p := receiverProbeFixture(t)
			c := f.command()
			c.Operation = RuntimeStartContainer
			c.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64)}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			f.resolver.resolve = func(_ context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
				managed := false
				switch mode {
				case "offline":
					return RuntimeResolvedTarget{}, errors.New("fixture offline")
				case "unresolved":
					managed = true
				case "parent":
					target.ParentProjectID = new("example-parent")
				case "kind":
					target.Kind = "stack"
				case "id":
					target.ID = strings.Repeat("b", 64)
				case "invalid-name":
					target.Name = new("bad\nname")
				case "cancelled":
					cancel()
				}
				return RuntimeResolvedTarget{Target: target, ComposeManaged: managed}, nil
			}
			before := runtimeBoundarySnapshot(t, f.store)
			result, err := f.receiver.Execute(ctx, receiverEnvelope(t, c))
			receiverZero(t, result)
			switch mode {
			case "offline", "unresolved":
				var target *RuntimeTargetError
				require.ErrorAs(t, err, &target)
				if mode == "unresolved" {
					require.Equal(t, string(activitytypes.CodeRuntimeParentUnresolved), target.Code)
				}
			case "cancelled":
				require.ErrorIs(t, err, context.Canceled)
			default:
				receiverMismatch(t, result, err, "unassigned")
			}
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			require.Zero(t, p.calls.Load())
		})
	}
}

func TestRuntimeReceiverSevenOperationsLocalNameAndCopies(t *testing.T) {
	for _, op := range []RuntimeOperation{RuntimeDeployProject, RuntimeRedeployProject, RuntimeDownProject, RuntimeRestartProject, RuntimeStartContainer, RuntimeStopContainer, RuntimeRestartContainer} {
		t.Run(string(op), func(t *testing.T) {
			commands := make(chan RuntimeCommand, 1)
			f := receiverNew(t, func(_ context.Context, c RuntimeCommand, _ io.Writer) error { commands <- c; return nil })
			c := f.command()
			c.Operation = op
			c.Actor.Preferences = usertypes.Preferences{ThemeMode: new("dark")}
			if strings.HasSuffix(string(op), "container") {
				c.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64), ParentProjectID: new("example-parent")}
			}
			f.resolver.resolve = func(_ context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
				// Mutation of resolver inputs cannot alter the retained canonical parent.
				parent := copyPtrInternal(target.ParentProjectID)
				if target.ParentProjectID != nil {
					*target.ParentProjectID = "resolver-input-mutation"
				}
				target.ParentProjectID = parent
				target.Name = new("Local effective name")
				return RuntimeResolvedTarget{Target: target, ComposeManaged: parent != nil}, nil
			}
			e := receiverEnvelope(t, c)
			receiverExecute(t, f, e)
			require.Equal(t, activitytypes.StatusSuccess, receiverWait(t, f, e).Status)
			select {
			case got := <-commands:
				require.Empty(t, got.Key)
				require.Equal(t, e.Identity.RunID, got.ProposedRunID)
				require.Equal(t, "Local effective name", *got.Target.Name)
				require.Equal(t, c.Target.ParentProjectID, got.Target.ParentProjectID)
				require.Equal(t, usertypes.Preferences{}, got.Actor.Preferences)
			case <-time.After(5 * time.Second):
				t.Fatal("executor command missing")
			}
			receiverCounts(t, f, 1, 1, 1)
		})
	}
}

func TestRuntimeReceiverTupleDigestAndShape(t *testing.T) {
	f, p := receiverProbeFixture(t)
	e := receiverEnvelope(t, f.command())
	receiverExecute(t, f, e)
	b1Await(t, p.entered)
	before := runtimeBoundarySnapshot(t, f.store)
	for _, field := range []string{"manager", "registration", "generation", "digest", "semantic-target"} {
		t.Run(field, func(t *testing.T) {
			changed := e
			switch field {
			case "manager":
				changed.Identity.ManagerID = "example-other-manager"
				changed.Operation = "unsupported"
			case "registration":
				changed.Identity.RegistrationID = "example-other-registration"
			case "generation":
				changed.Identity.ExecutorGeneration = uuid.NewString()
			case "digest":
				changed.Identity.RequestDigest = strings.Repeat("a", 64)
			case "semantic-target":
				changed.Target.ID = "example-other-stack"
				changed.Identity.RequestDigest = receiverEnvelope(t, RuntimeCommand{Store: f.receiver.store, Actor: f.command().Actor, Operation: e.Operation, Target: changed.Target}).Identity.RequestDigest
			}
			result, err := f.receiver.Execute(t.Context(), changed)
			receiverMismatch(t, result, err, e.Identity.RunID)
			if field != "semantic-target" {
				result, err = f.receiver.Query(t.Context(), changed.Identity)
				receiverMismatch(t, result, err, e.Identity.RunID)
				result, err = f.receiver.Cancel(t.Context(), changed.Identity, e.Attribution)
				receiverMismatch(t, result, err, e.Identity.RunID)
			}
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
		})
	}
	for _, bad := range []string{"uuid", "digest", "actor", "operation", "options"} {
		changed := e
		switch bad {
		case "uuid":
			changed.Identity.RunID = "invalid"
		case "digest":
			changed.Identity.RequestDigest = "not-a-digest"
		case "actor":
			changed.Attribution.ID = " spaced "
		case "operation":
			changed.Operation = "arbitrary-route"
		case "options":
			changed.Options.RestartServices = []string{"unsupported"}
		}
		result, err := f.receiver.Execute(t.Context(), changed)
		receiverZero(t, result)
		require.ErrorIs(t, err, errRuntimeInvalidInternal)
		require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	}
	p.unblock()
	receiverWait(t, f, e)
}

func TestRuntimeReceiverRequiresLocalTrustedConfig(t *testing.T) {
	f := b1Fixture(t)
	_, err := NewRuntimeReceiver(f.service)
	require.ErrorIs(t, err, errRuntimeInvalidInternal)
	for _, mode := range []string{"nil-app", "nil-executor", "environment", "generation", "manager", "registration"} {
		local := RuntimeLocalConfig{ApplicationContext: t.Context(), Store: f.command().Store, Executor: b2ExecutorFunc(func(context.Context, RuntimeCommand, io.Writer) error { return nil })}
		switch mode {
		case "nil-app":
			local.ApplicationContext = nil
		case "nil-executor":
			local.Executor = nil
		case "environment":
			local.Store.EnvironmentID = "example-remote"
		case "generation":
			local.Store.ExecutorGeneration = uuid.NewString()
		case "manager":
			local.Store.ManagerID = " "
		case "registration":
			local.Store.RegistrationID = ""
		}
		_, err := NewRuntimeReceiver(NewRuntimeService(f.store.svc, f.resolver, local))
		require.ErrorIs(t, err, errRuntimeInvalidInternal)
	}
}

func TestRuntimeReceiverCompactRetentionUsesFinalWrite(t *testing.T) {
	f, p := receiverProbeFixture(t)
	e := receiverEnvelope(t, f.command())
	receiverExecute(t, f, e)
	b1Await(t, p.entered)
	p.unblock()
	receiverWait(t, f, e)
	absent := e.Identity
	absent.RunID = uuid.NewString()
	negative, err := f.receiver.Query(t.Context(), absent)
	require.NoError(t, err)
	require.Nil(t, negative.Snapshot)
	before := runtimeBoundarySnapshot(t, f.store)
	_, err = f.store.svc.DeleteHistory(t.Context(), "0")
	require.NoError(t, err)
	require.NoError(t, f.store.tx(func(tx *gorm.DB, _ time.Time) error {
		count, err := runtimePruneTxInternal(tx, before.Receipts[0].FinalWriteAt.Add(24*time.Hour-time.Microsecond), 100)
		require.Zero(t, count)
		return err
	}))
	retained := receiverExecute(t, f, e)
	require.Equal(t, activitytypes.RuntimeAnswerTerminal, retained.State)
	// A later compact final write extends the floor independently of admission.
	later := f.store.now.Add(time.Hour)
	require.NoError(t, f.store.tx(func(tx *gorm.DB, _ time.Time) error {
		row, err := runtimeReadTxInternal(tx, runtimeIdentityInternal{RunID: e.Identity.RunID, StoreGeneration: f.receiver.store.StoreGeneration, ManagerID: e.Identity.ManagerID,
			RegistrationID: e.Identity.RegistrationID, EnvironmentID: "0", ExecutorGeneration: e.Identity.ExecutorGeneration, RequestDigest: e.Identity.RequestDigest})
		if err != nil {
			return err
		}
		return runtimeUpdateTxInternal(tx, later, row, map[string]any{})
	}))
	require.NoError(t, f.store.tx(func(tx *gorm.DB, _ time.Time) error {
		count, err := runtimePruneTxInternal(tx, later.Add(24*time.Hour-time.Microsecond), 100)
		require.EqualValues(t, 1, count)
		return err
	}))
	receiverCounts(t, f, 1, 0, 0)
	require.NoError(t, f.store.tx(func(tx *gorm.DB, _ time.Time) error {
		count, err := runtimePruneTxInternal(tx, later.Add(24*time.Hour), 100)
		require.EqualValues(t, 1, count)
		return err
	}))
	receiverCounts(t, f, 0, 0, 0)
	require.EqualValues(t, 1, p.calls.Load())
	// A fresh fence after retention is executor evidence only; C6 owns whether
	// the hub can trust it as recent negative authority. Never execute the old ID.
	result, err := f.receiver.Query(t.Context(), e.Identity)
	require.NoError(t, err)
	require.Equal(t, activitytypes.RuntimeAnswerNotReceived, result.State)
	require.Nil(t, result.Snapshot)
}
