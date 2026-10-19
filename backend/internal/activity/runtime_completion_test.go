// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Real migrated SQLite settlement faults, retained outcomes and conditional proofs.

package activity

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// All effect counters in this file count a CONTROLLED EXECUTOR port, not SDK calls.
type b2ExecutorFunc func(context.Context, RuntimeCommand, io.Writer) error

func (fn b2ExecutorFunc) ExecuteRuntime(ctx context.Context, c RuntimeCommand, output io.Writer) error {
	return fn(ctx, c, output)
}
func b2Fixture(t *testing.T, app context.Context, executor b2ExecutorFunc) *b1FixtureInternal {
	t.Helper()
	f := b1Fixture(t)
	f.service = NewRuntimeService(f.store.svc, f.resolver, RuntimeLocalConfig{ApplicationContext: app, Store: f.command().Store, Executor: executor})
	return f
}
func b2Ref(c RuntimeCommand, id string) RuntimeRunReference {
	return RuntimeRunReference{Store: c.Store, RunID: id}
}
func b2Wait(t *testing.T, f *b1FixtureInternal, c RuntimeCommand, id string) (activitytypes.RuntimeSnapshot, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	return f.service.WaitLocal(ctx, b2Ref(c, id))
}
func b2Owner(t *testing.T, f *b1FixtureInternal, id string) *runtimeOwnerInternal {
	t.Helper()
	f.store.svc.runningMu.Lock()
	defer f.store.svc.runningMu.Unlock()
	owner := f.service.owners[id]
	require.NotNil(t, owner)
	return owner
}
func b2Held(t *testing.T, f *b1FixtureInternal, c RuntimeCommand, id string) {
	t.Helper()
	snap := runtimeBoundarySnapshot(t, f.store)
	require.Len(t, snap.Receipts, 1)
	require.Nil(t, snap.Receipts[0].SettledAt)
	require.Len(t, snap.Activities, 1)
	require.Equal(t, activitytypes.StatusRunning, snap.Activities[0].Status)
	b2Owner(t, f, id)
	for _, parent := range []bool{false, true} {
		competing := c
		competing.Key = uuid.NewString()
		competing.Operation = RuntimeDownProject
		competing.Options = RuntimeOptions{}
		if parent {
			competing.Operation = RuntimeStartContainer
			competing.Target = RuntimeTarget{Kind: "container", ID: "example-child", ParentProjectID: new(c.Target.ID)}
		}
		result, err := f.service.Admit(t.Context(), competing)
		b1Zero(t, result)
		var conflict *GateConflict
		require.ErrorAs(t, err, &conflict)
	}
}
func b2FinalCount(t *testing.T, f *b1FixtureInternal, id string, want int64) {
	t.Helper()
	var count int64
	require.NoError(t, f.store.svc.db.Model(&ActivityMessage{}).Where("activity_id = ? AND code IN ?", id, []string{"activity.success", "runtime.failed", "runtime.cancelled", "runtime.interrupted"}).Count(&count).Error)
	require.Equal(t, want, count)
}
func b2Deferred(t *testing.T, f *b1FixtureInternal, predicate string) {
	t.Helper()
	require.NoError(t, f.store.svc.db.Exec("CREATE TABLE fixture_b2_parent(id INTEGER PRIMARY KEY)").Error)
	require.NoError(t, f.store.svc.db.Exec("CREATE TABLE fixture_b2_deferred(id INTEGER REFERENCES fixture_b2_parent(id) DEFERRABLE INITIALLY DEFERRED)").Error)
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_b2_fault AFTER INSERT ON activity_messages WHEN "+predicate+" BEGIN INSERT INTO fixture_b2_deferred VALUES(1); END").Error)
}

func TestRuntimeCompletionAtomicFaultAndRetainedRetry(t *testing.T) {
	for _, fault := range []string{"receipt", "activity", "final-message", "commit"} {
		t.Run(fault, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			var calls atomic.Int64
			f := b2Fixture(t, t.Context(), func(context.Context, RuntimeCommand, io.Writer) error {
				calls.Add(1)
				close(entered)
				<-release
				return nil
			})
			c := f.command()
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			b1Await(t, entered)
			owner := b2Owner(t, f, admitted.ActivityID)
			var slotReleased atomic.Int64
			f.store.svc.registerSlotReleaseInternal(admitted.ActivityID, func() { slotReleased.Add(1) })
			before := runtimeBoundarySnapshot(t, f.store)
			switch fault {
			case "receipt":
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_b2_fault BEFORE UPDATE OF executor_settled_at ON runtime_receipts BEGIN SELECT RAISE(ABORT, 'fixture terminal receipt'); END").Error)
			case "activity":
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_b2_fault BEFORE UPDATE OF status ON activities WHEN NEW.status = 'success' BEGIN SELECT RAISE(ABORT, 'fixture terminal activity'); END").Error)
			case "final-message":
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_b2_fault BEFORE INSERT ON activity_messages WHEN NEW.code = 'activity.success' BEGIN SELECT RAISE(ABORT, 'fixture terminal message'); END").Error)
			case "commit":
				b2Deferred(t, f, "NEW.code = 'activity.success'")
			}
			events, _, unsubscribe := f.store.svc.Subscribe("0")
			defer unsubscribe()
			unblock()
			b1Await(t, owner.attemptDone)
			f.store.svc.runningMu.Lock()
			persistErr, result := owner.err, *owner.result
			f.store.svc.runningMu.Unlock()
			require.Error(t, persistErr)
			require.Zero(t, slotReleased.Load())
			if fault == "commit" {
				require.ErrorContains(t, persistErr, "FOREIGN KEY constraint failed")
			}
			require.False(t, dbutil.IsRetryableWriteError(persistErr))
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store), "whole transaction must roll back")
			f.store.svc.subscribersMu.RLock()
			for _, sub := range f.store.svc.subscribers {
				sub.mu.Lock()
				require.Empty(t, sub.queue)
				sub.mu.Unlock()
			}
			f.store.svc.subscribersMu.RUnlock()
			select {
			case event := <-events:
				t.Fatalf("uncommitted publication: %v", event.Type)
			default:
			}
			b2Held(t, f, c, admitted.ActivityID)
			b2FinalCount(t, f, admitted.ActivityID, 0)
			require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER fixture_b2_fault").Error)
			// A late durable intent may not rewrite the already joined success.
			cancelled, err := f.service.Cancel(t.Context(), b2Ref(c, admitted.ActivityID), c.Actor)
			require.NoError(t, err)
			require.True(t, cancelled.CancelRequested)
			settled, err := f.service.RetrySettlement(t.Context(), b2Ref(c, admitted.ActivityID))
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusSuccess, settled.Status)
			after := runtimeBoundarySnapshot(t, f.store)
			require.Equal(t, new(result.settledAt), after.Receipts[0].ExecutorSettledAt)
			require.Equal(t, new(result.settledAt), after.Activities[0].EndedAt)
			require.Equal(t, "activity.success", after.Activities[0].MessageCode)
			require.EqualValues(t, 1, calls.Load())
			require.EqualValues(t, 1, slotReleased.Load())
			b2FinalCount(t, f, admitted.ActivityID, 1)
			again, err := f.service.RetrySettlement(t.Context(), b2Ref(c, admitted.ActivityID))
			require.NoError(t, err)
			require.Equal(t, settled, again)
			late, err := f.service.Cancel(t.Context(), b2Ref(c, admitted.ActivityID), c.Actor)
			require.NoError(t, err)
			require.Equal(t, settled, late)
			require.Equal(t, after, runtimeBoundarySnapshot(t, f.store))
			f.store.svc.runningMu.Lock()
			require.Empty(t, f.service.owners)
			f.store.svc.runningMu.Unlock()
		})
	}
}

func TestRuntimeCompletionConditionalDuplicateAndConflict(t *testing.T) {
	f := b2Fixture(t, t.Context(), func(context.Context, RuntimeCommand, io.Writer) error { return nil })
	c := f.command()
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	settled, err := b2Wait(t, f, c, admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, settled.Status)
	before := runtimeBoundarySnapshot(t, f.store)
	row := before.Receipts[0]
	result := runtimeLocalResultInternal{state: *row.ExecutorState, code: activitytypes.Code(*row.ExecutorResultCode), message: "Runtime workflow completed", settledAt: *row.ExecutorSettledAt, startedAt: row.ExecutorStartedAt}
	ctx := context.WithValue(t.Context(), gateRunContextKey{}, admitted.ActivityID)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			f.store.svc.runningMu.Lock()
			defer f.store.svc.runningMu.Unlock()
			e := f.service.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
				snapshot, final, e := runtimeCompleteTxInternal(tx, now, b1Identity(t, c, admitted.ActivityID), result)
				if e == nil {
					require.Equal(t, settled, snapshot)
					require.Nil(t, final)
				}
				return e
			})
			require.NoError(t, e)
		})
	}
	wg.Wait()
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	b2FinalCount(t, f, admitted.ActivityID, 1)
	for _, change := range []string{"state", "code", "error", "time", "identity"} {
		t.Run(change, func(t *testing.T) {
			follower := result
			id := b1Identity(t, c, admitted.ActivityID)
			switch change {
			case "state":
				follower.state = "failed"
				follower.code = activitytypes.CodeRuntimeFailed
			case "code":
				follower.state = "failed"
				follower.code = activitytypes.CodeRuntimeInterrupted
			case "error":
				follower.errorText = new("different")
			case "time":
				follower.settledAt = follower.settledAt.Add(time.Second)
			case "identity":
				id.RegistrationID = "example-other"
			}
			f.store.svc.runningMu.Lock()
			err := f.service.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
				snapshot, final, e := runtimeCompleteTxInternal(tx, now, id, follower)
				require.Equal(t, activitytypes.RuntimeSnapshot{}, snapshot)
				require.Nil(t, final)
				return e
			})
			f.store.svc.runningMu.Unlock()
			var conflict *runtimeConflictInternal
			require.ErrorAs(t, err, &conflict)
			require.NotContains(t, err.Error(), admitted.ActivityID)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
		})
	}
}

// This pool decorator counts actual BEGIN attempts without changing SQLite's
// driver results. BUSY is produced by an independent real immediate writer.
type b2BeginCounter struct {
	*sql.DB
	calls   atomic.Int64
	entered chan struct{}
}

func (p *b2BeginCounter) GetDBConn() (*sql.DB, error) { return p.DB, nil }
func (p *b2BeginCounter) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	p.calls.Add(1)
	if p.entered != nil {
		p.entered <- struct{}{}
	}
	return p.DB.BeginTx(ctx, opts)
}
func TestRuntimeCompletionRealBusyIsBoundedAndNoReplay(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var calls atomic.Int64
	f := b2Fixture(t, t.Context(), func(context.Context, RuntimeCommand, io.Writer) error {
		calls.Add(1)
		close(entered)
		<-release
		return nil
	})
	c := f.command()
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	b1Await(t, entered)
	owner := b2Owner(t, f, admitted.ActivityID)
	other := runtimeContender(t, f.store)
	lock := other.Begin()
	require.NoError(t, lock.Error)
	var rollback sync.Once
	unlock := func() { rollback.Do(func() { require.NoError(t, lock.Rollback().Error) }) }
	defer unlock()
	pool, err := f.store.svc.db.DB.DB()
	require.NoError(t, err)
	_, err = pool.ExecContext(t.Context(), "PRAGMA busy_timeout=1")
	require.NoError(t, err)
	counter := &b2BeginCounter{DB: pool}
	original := f.store.svc.db
	session := original.Session(&gorm.Session{NewDB: true})
	session.Config.ConnPool = counter
	session.Statement.ConnPool = counter
	f.store.svc.db = &database.DB{DB: session}
	unblock()
	b1Await(t, owner.attemptDone)
	require.EqualValues(t, 3, counter.calls.Load())
	f.store.svc.runningMu.Lock()
	persistErr := owner.err
	f.store.svc.runningMu.Unlock()
	require.Error(t, persistErr)
	require.True(t, dbutil.IsRetryableWriteError(persistErr), "%v", persistErr)
	unlock()
	b2Held(t, f, c, admitted.ActivityID)
	_, err = f.service.RetrySettlement(t.Context(), b2Ref(c, admitted.ActivityID))
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
	b2FinalCount(t, f, admitted.ActivityID, 1)
	f.store.svc.db = original
}

func TestRuntimeCompletionConditionalActivityAndNonretryable(t *testing.T) {
	for _, fault := range []string{"already-terminal", "io"} {
		t.Run(fault, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			id := b1Identity(t, c, admitted.ActivityID)
			b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
				if _, err := runtimeLocalAcceptTxInternal(tx, now, id); err != nil {
					return err
				}
				_, err := runtimeLocalExecutingTxInternal(tx, now, id)
				return err
			})
			if fault == "already-terminal" {
				require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", admitted.ActivityID).UpdateColumn("status", "failed").Error)
			}
			before := runtimeBoundarySnapshot(t, f.store)
			ctx := context.WithValue(t.Context(), gateRunContextKey{}, admitted.ActivityID)
			result := runtimeLocalResultInternal{state: "success", code: activitytypes.CodeActivitySuccess, message: "Runtime workflow completed", settledAt: runtimeTimeInternal(f.store.now), startedAt: before.Receipts[0].ExecutorStartedAt}
			var attempts atomic.Int64
			injected := errors.New("fixture I/O failure")
			f.store.svc.runningMu.Lock()
			err = f.service.ownerTransactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
				attempts.Add(1)
				if fault == "io" {
					return injected
				}
				_, _, err := runtimeCompleteTxInternal(tx, now, id, result)
				return err
			})
			f.store.svc.runningMu.Unlock()
			require.Error(t, err)
			require.EqualValues(t, 1, attempts.Load())
			if fault == "io" {
				require.ErrorIs(t, err, injected)
			} else {
				var conflict *runtimeConflictInternal
				require.ErrorAs(t, err, &conflict)
			}
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
		})
	}
}

func TestRuntimeCompletionConcurrentRetryAndLateCancel(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var calls atomic.Int64
	f := b2Fixture(t, t.Context(), func(context.Context, RuntimeCommand, io.Writer) error {
		calls.Add(1)
		close(entered)
		<-release
		return nil
	})
	c := f.command()
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	b1Await(t, entered)
	owner := b2Owner(t, f, admitted.ActivityID)
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_retry BEFORE INSERT ON activity_messages WHEN NEW.code = 'activity.success' BEGIN SELECT RAISE(ABORT, 'fixture retained result'); END").Error)
	unblock()
	b1Await(t, owner.attemptDone)
	f.store.svc.runningMu.Lock()
	require.Error(t, owner.err)
	frozen := *owner.result
	f.store.svc.runningMu.Unlock()
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER fixture_retry").Error)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			_, err := f.service.Cancel(t.Context(), b2Ref(c, admitted.ActivityID), c.Actor)
			require.NoError(t, err)
			got, err := f.service.RetrySettlement(t.Context(), b2Ref(c, admitted.ActivityID))
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusSuccess, got.Status)
		})
	}
	wg.Wait()
	after := runtimeBoundarySnapshot(t, f.store)
	require.Equal(t, new(frozen.settledAt), after.Receipts[0].ExecutorSettledAt)
	require.EqualValues(t, 1, calls.Load())
	b2FinalCount(t, f, admitted.ActivityID, 1)
	var count int64
	require.NoError(t, f.store.svc.db.Model(&ActivityMessage{}).Where("code = 'run.cancelling'").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestRuntimeCompletionRefusesForeignUnknownDisposedAndUnowned(t *testing.T) {
	for _, mode := range []string{"foreign", "unknown", "dispatched", "disposed", "unowned", "generation"} {
		t.Run(mode, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			if mode == "foreign" {
				c.Store.ExecutorGeneration = uuid.NewString()
			}
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			id := b1Identity(t, c, admitted.ActivityID)
			b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
				switch mode {
				case "unknown":
					_, err := runtimeEvidenceTxInternal(tx, now, id, runtimeEvidenceInternal{State: "unknown", UnknownReason: new("executor-unavailable")})
					return err
				case "dispatched":
					_, err := runtimeDispatchTxInternal(tx, now, id, false)
					return err
				case "disposed":
					_, err := runtimeDispositionTxInternal(tx, now, id, runtimeDispositionInternal{Disposition: "released", By: "example-admin", Reason: "Executor stopped and inspected", ReleaseAcknowledged: true})
					return err
				}
				return nil
			})
			before := runtimeBoundarySnapshot(t, f.store)
			ctx := context.WithValue(t.Context(), gateRunContextKey{}, admitted.ActivityID)
			if mode == "unowned" {
				ctx = context.WithValue(t.Context(), gateRunContextKey{}, "example-other")
			}
			if mode == "generation" {
				id.StoreGeneration = uuid.NewString()
			}
			result := runtimeLocalResultInternal{state: "failed", code: activitytypes.CodeRuntimeInterrupted, message: "Runtime workflow interrupted; effects unverified", settledAt: runtimeTimeInternal(f.store.now)}
			f.store.svc.runningMu.Lock()
			err = f.service.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
				snapshot, final, err := runtimeCompleteTxInternal(tx, now, id, result)
				require.Equal(t, activitytypes.RuntimeSnapshot{}, snapshot)
				require.Nil(t, final)
				return err
			})
			f.store.svc.runningMu.Unlock()
			if mode == "unowned" {
				require.ErrorIs(t, err, ErrRunNotOwned)
			} else {
				var conflict *runtimeConflictInternal
				require.ErrorAs(t, err, &conflict)
			}
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
		})
	}
}
