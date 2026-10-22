// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package activity

import (
	"context"
	"encoding/json/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type remoteTransportProbe struct {
	executes, queries, cancels atomic.Int32
	execute                    func(context.Context, activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error)
	query                      func(context.Context, activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error)
	cancel                     func(context.Context, activitytypes.RuntimeCancellation) (activitytypes.RuntimeExecutionOutcome, error)
}

func (p *remoteTransportProbe) Execute(ctx context.Context, e activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error) {
	p.executes.Add(1)
	if p.execute != nil {
		return p.execute(ctx, e)
	}
	return activitytypes.RuntimeExecutionOutcome{}, ErrRuntimeTransport
}
func (p *remoteTransportProbe) Query(ctx context.Context, id activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
	p.queries.Add(1)
	if p.query != nil {
		return p.query(ctx, id)
	}
	return activitytypes.RuntimeExecutionOutcome{}, ErrRuntimeTransport
}
func (p *remoteTransportProbe) Cancel(ctx context.Context, c activitytypes.RuntimeCancellation) (activitytypes.RuntimeExecutionOutcome, error) {
	p.cancels.Add(1)
	if p.cancel != nil {
		return p.cancel(ctx, c)
	}
	return activitytypes.RuntimeExecutionOutcome{}, ErrRuntimeTransport
}

type remoteFixtureInternal struct {
	*b1FixtureInternal
	control      *RuntimeRemoteCoordinator
	probe        *remoteTransportProbe
	commandValue RuntimeCommand
	ref          RuntimeRunReference
}

func remoteFixture(t *testing.T, p *remoteTransportProbe, schedule RuntimeReconcileSchedule) *remoteFixtureInternal {
	t.Helper()
	f := b1Fixture(t)
	command := f.command()
	command.Store.EnvironmentID = "example-remote"
	command.Store.ExecutorGeneration = uuid.NewString()
	if schedule.Wait == nil {
		schedule.Wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	}
	c, err := NewRuntimeRemoteCoordinator(f.service, t.Context(), func(context.Context, RuntimeStoreBinding) (RuntimeRemoteTransport, error) { return p, nil }, schedule)
	require.NoError(t, err)
	result, err := f.service.Admit(t.Context(), command)
	require.NoError(t, err)
	out := &remoteFixtureInternal{f, c, p, command, RuntimeRunReference{Store: command.Store, RunID: result.ActivityID}}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, c.Close(ctx))
	})
	return out
}
func (f *remoteFixtureInternal) start(t *testing.T) {
	t.Helper()
	_, err := f.control.Start(t.Context(), f.ref, f.commandValue.Options)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, f.control.WaitDispatch(ctx, f.ref.RunID))
}
func (f *remoteFixtureInternal) receipt(t *testing.T) RuntimeReceipt {
	t.Helper()
	var row RuntimeReceipt
	require.NoError(t, f.store.svc.db.First(&row, "run_id = ?", f.ref.RunID).Error)
	return row
}
func remoteAnswer(id activitytypes.RuntimeRunIdentity, now time.Time, status activitytypes.Status, seqs ...uint64) activitytypes.RuntimeExecutionOutcome {
	a := activitytypes.RuntimeSnapshot{Activity: activitytypes.Activity{ID: id.RunID, EnvironmentID: "0", Type: activitytypes.TypeProjectDeploy, Status: status, StartedAt: now, CreatedAt: now, UpdatedAt: new(now), ResourceType: new("stack"), ResourceID: new("example-stack"), StartedBy: &activitytypes.StartedBy{UserID: "example-owner", Username: "Operator"}, MessageCode: "run.started"}, RuntimeKnowledge: activitytypes.RuntimeKnowledge{OutcomeState: activitytypes.RuntimeOutcomeKnown}}
	state := activitytypes.RuntimeAnswerActive
	switch status {
	case activitytypes.StatusSuccess:
		a.EndedAt = new(now)
		a.MessageCode = "activity.success"
		state = activitytypes.RuntimeAnswerTerminal
	case activitytypes.StatusCancelled:
		a.EndedAt = new(now)
		a.MessageCode = string(activitytypes.CodeRuntimeCancelled)
		state = activitytypes.RuntimeAnswerTerminal
	}
	d := &activitytypes.RuntimeExecutionDetail{Activity: a, Messages: []activitytypes.Message{}, StepTrace: []activitytypes.StepTraceEntry{}, Output: []activitytypes.RuntimeExecutionOutputLine{}, RetainedStartSeq: 1}
	for _, seq := range seqs {
		d.Output = append(d.Output, activitytypes.RuntimeExecutionOutputLine{OutputLine: activitytypes.OutputLine{Timestamp: now, Stream: "stdout", Text: "line"}, OutputSeq: seq})
	}
	if len(seqs) > 0 {
		d.RetainedStartSeq = seqs[0]
		d.OutputTruncated = seqs[0] > 1
	}
	return activitytypes.RuntimeExecutionOutcome{Identity: id, State: state, Detail: d}
}
func TestRuntimeRemoteUncertaintyNeverReplaysOrReleases(t *testing.T) {
	for _, mode := range []string{"transport", "generic-conflict", "store", "malformed", "strict-refusal"} {
		t.Run(mode, func(t *testing.T) {
			p := &remoteTransportProbe{}
			f := remoteFixture(t, p, RuntimeReconcileSchedule{})
			p.execute = func(context.Context, activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error) {
				if mode == "strict-refusal" {
					return activitytypes.RuntimeExecutionOutcome{}, &RuntimeExecutionRefused{Code: activitytypes.CodeRunBusy}
				}
				if mode == "generic-conflict" {
					return activitytypes.RuntimeExecutionOutcome{}, &runtimeConflictInternal{}
				}
				return activitytypes.RuntimeExecutionOutcome{}, ErrRuntimeTransport
			}
			f.start(t)
			row := f.receipt(t)
			require.EqualValues(t, 1, p.executes.Load())
			if mode == "strict-refusal" {
				require.NotNil(t, row.SettledAt)
				require.Equal(t, "refused", *row.HubDisposition)
				require.Zero(t, p.queries.Load())
			} else {
				require.Nil(t, row.SettledAt)
				require.Equal(t, "unknown", *row.ExecutorState)
				require.EqualValues(t, 3, p.queries.Load())
			}
			f.start(t)
			require.EqualValues(t, 1, p.executes.Load())
		})
	}
}
func TestRuntimeRemoteNegativeProofContinuityAndQueryBudget(t *testing.T) {
	for _, mode := range []string{"recent", "old", "restart", "query-budget", "forward-clock", "backward-clock", "wrong-generation"} {
		t.Run(mode, func(t *testing.T) {
			base := time.Now()
			var elapsed atomic.Int64
			p := &remoteTransportProbe{}
			f := remoteFixture(t, p, RuntimeReconcileSchedule{Now: func() time.Time { return base.Add(time.Duration(elapsed.Load())) }})
			p.query = func(_ context.Context, id activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
				switch mode {
				case "old":
					f.control.mu.Lock()
					stamp := f.control.stamps[id.RunID]
					stamp.mono = stamp.mono.Add(-2 * time.Hour)
					stamp.wall = stamp.wall.Add(-2 * time.Hour)
					f.control.stamps[id.RunID] = stamp
					f.control.mu.Unlock()
				case "restart":
					f.control.InvalidateNegativeContinuity()
				case "query-budget":
					elapsed.Store(int64(6 * time.Second))
				case "forward-clock":
					f.store.now = f.store.now.Add(time.Hour)
				case "backward-clock":
					f.store.now = f.store.now.Add(-time.Hour)
				case "wrong-generation":
					id.ExecutorGeneration = uuid.NewString()
				}
				return activitytypes.RuntimeExecutionOutcome{Identity: id, State: activitytypes.RuntimeAnswerNotReceived}, nil
			}
			f.start(t)
			row := f.receipt(t)
			if mode == "recent" {
				require.NotNil(t, row.SettledAt)
				require.Equal(t, "not_received", *row.HubDisposition)
			} else {
				require.Nil(t, row.SettledAt)
				require.Equal(t, "unknown", *row.ExecutorState)
			}
			require.EqualValues(t, 1, p.executes.Load())
		})
	}
}
func TestRuntimeRemoteCancellationBeforeSendAndForwardAfterQuery(t *testing.T) {
	t.Run("admitted", func(t *testing.T) {
		p := &remoteTransportProbe{}
		f := remoteFixture(t, p, RuntimeReconcileSchedule{})
		_, err := f.control.Cancel(t.Context(), f.ref, f.commandValue.Actor)
		require.NoError(t, err)
		f.start(t)
		require.Zero(t, p.executes.Load())
		require.Equal(t, "cancelled", *f.receipt(t).HubDisposition)
	})
	t.Run("later-active", func(t *testing.T) {
		p := &remoteTransportProbe{}
		f := remoteFixture(t, p, RuntimeReconcileSchedule{})
		p.execute = func(_ context.Context, e activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error) {
			return remoteAnswer(e.Identity, f.store.now, activitytypes.StatusRunning), nil
		}
		p.query = func(_ context.Context, id activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
			return remoteAnswer(id, f.store.now, activitytypes.StatusRunning), nil
		}
		p.cancel = func(_ context.Context, c activitytypes.RuntimeCancellation) (activitytypes.RuntimeExecutionOutcome, error) {
			return remoteAnswer(c.Identity, f.store.now, activitytypes.StatusCancelled), nil
		}
		f.start(t)
		snapshot, err := f.control.Cancel(t.Context(), f.ref, f.commandValue.Actor)
		require.NoError(t, err)
		require.Equal(t, activitytypes.StatusCancelled, snapshot.Status)
		require.EqualValues(t, 1, p.cancels.Load())
		require.True(t, f.receipt(t).CancelRequested)
	})
}
func TestRuntimeRemoteWindowReplacementRelayAndTerminalWins(t *testing.T) {
	p := &remoteTransportProbe{}
	f := remoteFixture(t, p, RuntimeReconcileSchedule{})
	p.execute = func(_ context.Context, e activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error) {
		return remoteAnswer(e.Identity, f.store.now, activitytypes.StatusRunning, 1), nil
	}
	f.start(t)
	id := runtimeWireIdentityInternal(new(f.receipt(t)))
	first := remoteAnswer(id, f.store.now, activitytypes.StatusRunning, 5, 6)
	first.Detail.StepTrace = []activitytypes.StepTraceEntry{{ID: uuid.NewString(), Name: "prepare", Status: activitytypes.StatusSuccess, StartedAt: f.store.now, EndedAt: new(f.store.now)}, {ID: uuid.NewString(), Name: "execute", Status: activitytypes.StatusRunning, StartedAt: f.store.now}}
	first.Detail.Activity.Metadata = map[string]any{"phase": "execute"}
	_, err := f.service.applyRemoteInternal(t.Context(), f.ref, first, true)
	require.NoError(t, err)
	detail, err := f.service.GetRuntimeDetail(t.Context(), f.ref)
	require.NoError(t, err)
	require.Len(t, detail.Output, 2)
	require.Equal(t, uint64(5), detail.RetainedStartSeq)
	expectedTrace, err := json.Marshal(first.Detail.StepTrace)
	require.NoError(t, err)
	actualTrace, err := json.Marshal(detail.StepTrace)
	require.NoError(t, err)
	require.Equal(t, string(expectedTrace), string(actualTrace))
	require.Equal(t, first.Detail.Activity.Metadata, detail.Activity.Metadata)
	relay := &RuntimeOutputRelay{}
	var lines []activitytypes.OutputLine
	emit := func(l activitytypes.OutputLine) error { lines = append(lines, l); return nil }
	require.NoError(t, relay.Relay(detail, emit))
	require.Len(t, lines, 3)
	require.Contains(t, lines[0].Text, "truncated")
	require.NoError(t, relay.Relay(detail, emit))
	require.Len(t, lines, 3)
	second := remoteAnswer(id, f.store.now, activitytypes.StatusRunning, 6, 7)
	_, err = f.service.applyRemoteInternal(t.Context(), f.ref, second, true)
	require.NoError(t, err)
	detail, err = f.service.GetRuntimeDetail(t.Context(), f.ref)
	require.NoError(t, err)
	require.Len(t, detail.Output, 2)
	require.NoError(t, relay.Relay(detail, emit))
	require.Len(t, lines, 4)
	gap := remoteAnswer(id, f.store.now, activitytypes.StatusRunning, 9)
	require.NoError(t, relay.Relay(gap.Detail, emit))
	require.Len(t, lines, 6)
	require.True(t, relay.LossReported)
	internalGap := remoteAnswer(id, f.store.now, activitytypes.StatusRunning, 9, 10, 13)
	_, err = f.service.applyRemoteInternal(t.Context(), f.ref, internalGap, true)
	require.NoError(t, err)
	detail, err = f.service.GetRuntimeDetail(t.Context(), f.ref)
	require.NoError(t, err)
	require.Equal(t, uint64(9), detail.RetainedStartSeq)
	require.NoError(t, relay.Relay(detail, emit))
	var added []string
	for _, line := range lines[6:] {
		added = append(added, line.Text)
	}
	require.Equal(t, []string{"line", "[runtime output truncated]", "line"}, added)
	require.Equal(t, uint64(13), relay.Seen)
	require.NoError(t, relay.Relay(detail, emit))
	require.Len(t, lines, 9)
	empty := remoteAnswer(id, f.store.now, activitytypes.StatusRunning)
	empty.Detail.RetainedStartSeq = 16
	empty.Detail.OutputTruncated = true
	require.NoError(t, relay.Relay(empty.Detail, emit))
	require.Len(t, lines, 10)
	require.Equal(t, uint64(15), relay.Seen)
	terminal := remoteAnswer(id, f.store.now, activitytypes.StatusSuccess, 16)
	_, err = f.service.applyRemoteInternal(t.Context(), f.ref, terminal, true)
	require.NoError(t, err)
	before := b3Activity(t, f.b1FixtureInternal, f.ref.RunID)
	_, err = f.service.applyRemoteInternal(t.Context(), f.ref, first, false)
	require.NoError(t, err)
	require.Equal(t, before, b3Activity(t, f.b1FixtureInternal, f.ref.RunID))
	_, err = f.service.applyRemoteInternal(t.Context(), f.ref, terminal, true)
	require.NoError(t, err)
	var finals int64
	require.NoError(t, f.store.svc.db.Model(&ActivityMessage{}).Where("activity_id = ? AND code='activity.success'", f.ref.RunID).Count(&finals).Error)
	require.EqualValues(t, 1, finals)
}
func remoteAdminContext(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, usertypes.CurrentUserContextKey{}, &user.User{ID: "example-admin", Username: "Admin"})
	return context.WithValue(ctx, middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
}
func TestRuntimeRemoteAdministratorRemovalAndLateEvidence(t *testing.T) {
	p := &remoteTransportProbe{}
	f := remoteFixture(t, p, RuntimeReconcileSchedule{})
	f.start(t)
	var blocked *RuntimeRemovalBlocked
	require.ErrorAs(t, f.service.RemovalGuard(t.Context(), f.ref.Store), &blocked)
	require.Len(t, blocked.Runs, 1)
	_, err := f.control.Release(t.Context(), f.ref, "Executor stopped and actual state inspected", true)
	require.ErrorIs(t, err, ErrRuntimeRecoveryDenied)
	ordinary := context.WithValue(t.Context(), usertypes.CurrentUserContextKey{}, &user.User{ID: "example-user", Username: "Operator"})
	ordinary = context.WithValue(ordinary, middleware.ContextKeyUserPermissions, authz.EnvironmentPermissionSet(f.ref.Store.EnvironmentID))
	_, err = f.control.Release(ordinary, f.ref, "Executor stopped", true)
	require.ErrorIs(t, err, ErrRuntimeRecoveryDenied)
	ctx := remoteAdminContext(t.Context())
	_, err = f.control.Release(ctx, f.ref, "", true)
	require.Error(t, err)
	_, err = f.control.Release(ctx, f.ref, "Executor stopped", false)
	require.Error(t, err)
	snapshot, err := f.control.Release(ctx, f.ref, "Executor stopped and actual state inspected", true)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusFailed, snapshot.Status)
	before := b3Activity(t, f.b1FixtureInternal, f.ref.RunID)
	require.NoError(t, f.service.MarkRemoved(ctx, f.ref.Store))
	_, err = f.service.Admit(t.Context(), f.commandValue)
	require.ErrorIs(t, err, ErrRuntimeTargetRemoved)
	require.EqualValues(t, 1, p.executes.Load())
	row := f.receipt(t)
	id := runtimeWireIdentityInternal(&row)
	active := remoteAnswer(id, f.store.now, activitytypes.StatusRunning)
	_, err = f.service.applyRemoteInternal(t.Context(), f.ref, active, true)
	require.NoError(t, err)
	p.query = func(context.Context, activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
		return remoteAnswer(id, f.store.now, activitytypes.StatusSuccess), nil
	}
	_, err = f.control.Refresh(t.Context(), f.ref)
	require.NoError(t, err)
	require.Equal(t, before, b3Activity(t, f.b1FixtureInternal, f.ref.RunID))
	recovery, err := f.service.RecoveryDetail(t.Context(), f.ref)
	require.NoError(t, err)
	require.Equal(t, "success", recovery.ExecutorState)
	require.Equal(t, "released", *recovery.Disposition)
	replacement := f.commandValue
	replacement.Key = uuid.NewString()
	replacement.Store.RegistrationID = "example-new-registration"
	replacement.Store.EnvironmentID = "example-replacement"
	admitted, err := f.service.Admit(t.Context(), replacement)
	require.NoError(t, err)
	require.NotEqual(t, f.ref.RunID, admitted.ActivityID)
}
func TestRuntimeRemoteQueriesCoalesceAndHealthyHostIndependent(t *testing.T) {
	p := &remoteTransportProbe{}
	base := time.Now()
	var elapsed atomic.Int64
	f := remoteFixture(t, p, RuntimeReconcileSchedule{Now: func() time.Time { return base.Add(time.Duration(elapsed.Load())) }})
	p.execute = func(_ context.Context, e activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error) {
		return remoteAnswer(e.Identity, f.store.now, activitytypes.StatusRunning), nil
	}
	f.start(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	p.query = func(ctx context.Context, id activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
		once.Do(func() { close(entered) })
		select {
		case <-release:
			return remoteAnswer(id, f.store.now, activitytypes.StatusRunning), nil
		case <-ctx.Done():
			return activitytypes.RuntimeExecutionOutcome{}, ctx.Err()
		}
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	done := make(chan struct{})
	for range 8 {
		wg.Go(func() { <-start; _, err := f.control.Refresh(t.Context(), f.ref); require.NoError(t, err) })
	}
	close(start)
	b1Await(t, entered)
	c2 := f.commandValue
	c2.Store.EnvironmentID = "example-healthy"
	c2.Store.RegistrationID = "example-healthy-registration"
	c2.Key = uuid.NewString()
	admitted, err := f.service.Admit(t.Context(), c2)
	require.NoError(t, err)
	other := RuntimeRunReference{Store: c2.Store, RunID: admitted.ActivityID}
	_, err = f.control.Start(t.Context(), other, c2.Options)
	require.NoError(t, err)
	require.NoError(t, f.control.WaitDispatch(t.Context(), other.RunID))
	require.EqualValues(t, 1, p.queries.Load())
	close(release)
	go func() { wg.Wait(); close(done) }()
	b1Await(t, done)
	require.EqualValues(t, 1, p.queries.Load())
	_, err = f.control.Refresh(t.Context(), f.ref)
	require.NoError(t, err)
	require.EqualValues(t, 1, p.queries.Load())
	elapsed.Store(int64(2 * time.Second))
	_, err = f.control.Refresh(t.Context(), f.ref)
	require.NoError(t, err)
	require.EqualValues(t, 2, p.queries.Load())
}
func TestRuntimeRemoteDispatchIgnoredWriteStartsNothing(t *testing.T) {
	p := &remoteTransportProbe{}
	f := remoteFixture(t, p, RuntimeReconcileSchedule{})
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER remote_ignore_dispatch BEFORE UPDATE OF dispatch_state ON runtime_receipts BEGIN SELECT RAISE(IGNORE); END").Error)
	_, err := f.control.Start(t.Context(), f.ref, f.commandValue.Options)
	require.Error(t, err)
	require.Zero(t, p.executes.Load())
	require.Equal(t, "pending", *f.receipt(t).DispatchState)
}
func TestRuntimeRemoteTerminalAtomicFaults(t *testing.T) {
	for _, mode := range []string{"receipt-ignored", "activity-ignored", "audit-ignored", "audit-failed"} {
		t.Run(mode, func(t *testing.T) {
			p := &remoteTransportProbe{}
			f := remoteFixture(t, p, RuntimeReconcileSchedule{})
			p.execute = func(_ context.Context, e activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error) {
				return remoteAnswer(e.Identity, f.store.now, activitytypes.StatusRunning), nil
			}
			f.start(t)
			statement := "BEFORE UPDATE OF executor_state ON runtime_receipts WHEN NEW.executor_state='success'"
			action := "SELECT RAISE(IGNORE);"
			if mode == "activity-ignored" {
				statement = "BEFORE UPDATE OF status ON activities WHEN NEW.status='success'"
			}
			if mode == "audit-ignored" || mode == "audit-failed" {
				statement = "BEFORE INSERT ON activity_messages WHEN NEW.code='activity.success'"
			}
			if mode == "audit-failed" {
				action = "SELECT RAISE(ABORT,'fixture');"
			}
			require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER remote_fault "+statement+" BEGIN "+action+" END").Error)
			before := f.receipt(t)
			answer := remoteAnswer(runtimeWireIdentityInternal(&before), f.store.now, activitytypes.StatusSuccess)
			_, err := f.service.applyRemoteInternal(t.Context(), f.ref, answer, true)
			require.Error(t, err)
			require.Equal(t, before, f.receipt(t))
			require.Nil(t, f.receipt(t).SettledAt)
			require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER remote_fault").Error)
			_, err = f.service.applyRemoteInternal(t.Context(), f.ref, answer, true)
			require.NoError(t, err)
			_, err = f.service.applyRemoteInternal(t.Context(), f.ref, answer, true)
			require.NoError(t, err)
			require.EqualValues(t, 1, p.executes.Load())
		})
	}
}
