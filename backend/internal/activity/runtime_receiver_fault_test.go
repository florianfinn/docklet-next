// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Real SQLite refusal/COMMIT/context faults and storage-only settlement of receiver-origin runs.

package activity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"modernc.org/sqlite"
	sqlitecode "modernc.org/sqlite/lib"
)

func receiverDeferred(t *testing.T, f *receiverFixture, statement string) {
	t.Helper()
	require.NoError(t, f.store.svc.db.Exec("CREATE TABLE receiver_parent(id INTEGER PRIMARY KEY)").Error)
	require.NoError(t, f.store.svc.db.Exec("CREATE TABLE receiver_deferred(id INTEGER REFERENCES receiver_parent(id) DEFERRABLE INITIALLY DEFERRED)").Error)
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER receiver_fault "+statement+" BEGIN INSERT INTO receiver_deferred VALUES(1); END").Error)
}
func TestRuntimeReceiverAdmissionAtomicFaults(t *testing.T) {
	for _, mode := range []string{"receipt", "activity", "audit", "activity-ignored", "audit-ignored", "accepted", "accepted-ignored", "queued", "queued-ignored", "commit"} {
		t.Run(mode, func(t *testing.T) {
			f, p := receiverProbeFixture(t)
			before := runtimeBoundarySnapshot(t, f.store)
			statement := "BEFORE INSERT ON runtime_receipts"
			action := "SELECT RAISE(ABORT, 'fixture receiver admission');"
			switch mode {
			case "activity", "activity-ignored":
				statement = "BEFORE INSERT ON activities"
			case "audit", "audit-ignored":
				statement = "BEFORE INSERT ON activity_messages"
			case "accepted", "accepted-ignored":
				statement = "BEFORE UPDATE OF executor_state ON runtime_receipts WHEN NEW.executor_state = 'accepted'"
			case "queued", "queued-ignored":
				statement = "BEFORE UPDATE OF status ON activities WHEN NEW.status = 'queued'"
			}
			if mode == "activity-ignored" || mode == "audit-ignored" || mode == "accepted-ignored" || mode == "queued-ignored" {
				action = "SELECT RAISE(IGNORE);"
			}
			if mode == "commit" {
				receiverDeferred(t, f, "AFTER UPDATE OF executor_state ON runtime_receipts WHEN NEW.executor_state = 'accepted'")
			} else {
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER receiver_fault "+statement+" BEGIN "+action+" END").Error)
			}
			result, err := f.receiver.Execute(t.Context(), receiverEnvelope(t, f.command()))
			receiverZero(t, result)
			require.Error(t, err)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			f.store.svc.runningMu.Lock()
			require.Empty(t, f.service.owners)
			f.store.svc.runningMu.Unlock()
			require.Zero(t, p.calls.Load())
			slotsHeld(t, f.b1FixtureInternal, 0)
			require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER receiver_fault").Error)
		})
	}
}

func TestRuntimeReceiverGenerationNeverMeansAbsence(t *testing.T) {
	for _, mode := range []string{"missing", "replaced", "read-error"} {
		t.Run(mode, func(t *testing.T) {
			f, p := receiverProbeFixture(t)
			e := receiverEnvelope(t, f.command())
			switch mode {
			case "missing":
				require.NoError(t, f.store.svc.db.Where("id = 1").Delete(&RuntimeStoreGeneration{}).Error)
			case "replaced":
				require.NoError(t, f.store.svc.db.Model(&RuntimeStoreGeneration{}).Where("id = 1").Update("generation", uuid.NewString()).Error)
			case "read-error":
				require.NoError(t, f.store.svc.db.Exec("ALTER TABLE runtime_store_generation RENAME TO receiver_generation").Error)
			}
			for _, operation := range []string{"execute", "query", "cancel"} {
				var result RuntimeReceiverResult
				var err error
				switch operation {
				case "execute":
					result, err = f.receiver.Execute(t.Context(), e)
				case "query":
					result, err = f.receiver.Query(t.Context(), e.Identity)
				case "cancel":
					result, err = f.receiver.Cancel(t.Context(), e.Identity, e.Attribution)
				}
				receiverZero(t, result)
				require.False(t, errors.Is(err, gorm.ErrRecordNotFound))
				if mode == "replaced" {
					receiverMismatch(t, result, err, e.Identity.RunID)
				} else {
					var store *RuntimeStoreError
					require.ErrorAs(t, err, &store)
					if mode == "missing" {
						require.ErrorIs(t, err, errRuntimeGenerationUnavailableInternal)
					}
				}
			}
			if mode == "read-error" {
				require.NoError(t, f.store.svc.db.Exec("ALTER TABLE receiver_generation RENAME TO runtime_store_generation").Error)
			}
			receiverCounts(t, f, 0, 0, 0)
			require.Zero(t, p.calls.Load())
			require.Zero(t, f.resolver.calls.Load())
		})
	}
}

func TestRuntimeReceiverQueryCommitFailureNoNegativeEvidence(t *testing.T) {
	f, p := receiverProbeFixture(t)
	e := receiverEnvelope(t, f.command())
	before := runtimeBoundarySnapshot(t, f.store)
	receiverDeferred(t, f, "AFTER INSERT ON runtime_receipts WHEN NEW.kind = 'not_received'")
	result, err := f.receiver.Query(t.Context(), e.Identity)
	receiverZero(t, result)
	var store *RuntimeStoreError
	require.ErrorAs(t, err, &store)
	require.ErrorContains(t, err, "FOREIGN KEY constraint failed")
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER receiver_fault").Error)
	receiverExecute(t, f, e)
	b1Await(t, p.entered)
	p.unblock()
	receiverWait(t, f, e)
}

func TestRuntimeReceiverPreCancelledMutexAndPreCommit(t *testing.T) {
	for _, mode := range []string{"before", "mutex", "last-write"} {
		t.Run(mode, func(t *testing.T) {
			f, p := receiverProbeFixture(t)
			e := receiverEnvelope(t, f.command())
			before := runtimeBoundarySnapshot(t, f.store)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var reached atomic.Bool
			if mode == "last-write" {
				name := "receiver_precommit_cancel"
				require.NoError(t, f.store.svc.db.Callback().Update().After("gorm:update").Register(name, func(tx *gorm.DB) {
					if tx.Statement.Table == "runtime_receipts" {
						reached.Store(true)
						cancel()
					}
				}))
				defer f.store.svc.db.Callback().Update().Remove(name)
			}
			var result RuntimeReceiverResult
			var err error
			if mode == "mutex" {
				f.store.svc.runningMu.Lock()
				started := make(chan struct{})
				done := make(chan receiverAnswer, 1)
				go func() { close(started); r, err := f.receiver.Execute(ctx, e); done <- receiverAnswer{r, err} }()
				b1Await(t, started)
				cancel()
				f.store.svc.runningMu.Unlock()
				got := receiverAnswerWait(t, done)
				result, err = got.result, got.err
			} else {
				if mode == "before" {
					cancel()
				}
				result, err = f.receiver.Execute(ctx, e)
			}
			receiverZero(t, result)
			require.ErrorIs(t, err, context.Canceled)
			if mode == "last-write" {
				require.True(t, reached.Load(), "cancellation follows all required writes")
			}
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			require.Zero(t, p.calls.Load())
			require.Empty(t, f.service.owners)
		})
	}
}

func TestRuntimeReceiverSQLiteWaitKeepsRequestContext(t *testing.T) {
	f, p := receiverProbeFixture(t)
	other := runtimeContender(t, f.store)
	before := runtimeBoundarySnapshot(t, f.store)
	lock := other.Begin()
	require.NoError(t, lock.Error)
	defer lock.Rollback()
	pool, err := f.store.svc.db.DB.DB()
	require.NoError(t, err)
	_, busy := pool.ExecContext(t.Context(), "BEGIN IMMEDIATE")
	require.Error(t, busy)
	require.True(t, dbutil.IsRetryableWriteError(busy))
	observer := &receiverBeginObserver{DB: pool, entered: make(chan struct{})}
	session := f.store.svc.db.Session(&gorm.Session{NewDB: true})
	session.Config.ConnPool = observer
	session.Statement.ConnPool = observer
	f.store.svc.db = &database.DB{DB: session}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan receiverAnswer, 1)
	e := receiverEnvelope(t, f.command())
	go func() { r, err := f.receiver.Execute(ctx, e); done <- receiverAnswer{r, err} }()
	b1Await(t, observer.entered)
	cancel()
	got := receiverAnswerWait(t, done)
	receiverZero(t, got.result)
	require.ErrorIs(t, got.err, context.Canceled)
	require.NoError(t, lock.Rollback().Error)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	require.Zero(t, p.calls.Load())
}

// Real COMMIT succeeds first; cancellation at this boundary must not prevent
// application-lifetime registration. No production-only test seam is introduced.
type receiverCommitPool struct {
	*sql.DB
	after func(*sql.Tx)
}
type receiverCommitTx struct {
	*sql.Tx
	after func(*sql.Tx)
}

func (p *receiverCommitPool) GetDBConn() (*sql.DB, error) { return p.DB, nil }
func (p *receiverCommitPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &receiverCommitTx{tx, p.after}, nil
}
func (tx *receiverCommitTx) Commit() error {
	var accepted int
	_ = tx.QueryRow("SELECT count(*) FROM runtime_receipts WHERE executor_state = 'accepted'").Scan(&accepted)
	err := tx.Tx.Commit()
	if err == nil && accepted > 0 {
		tx.after(tx.Tx)
	}
	return err
}
func TestRuntimeReceiverCommittedDisconnectRegistersOnce(t *testing.T) {
	p := slotsNewProbe(t)
	var detached atomic.Bool
	f := receiverNew(t, func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error {
		detached.Store(ctx.Err() == nil && context.Cause(ctx) == nil)
		return p.run(ctx)
	})
	t.Cleanup(p.unblock)
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	var once sync.Once
	pool, err := f.store.svc.db.DB.DB()
	require.NoError(t, err)
	wrapper := &receiverCommitPool{DB: pool, after: func(*sql.Tx) { once.Do(func() { cancel(errors.New("fixture disconnected after commit")) }) }}
	session := f.store.svc.db.Session(&gorm.Session{NewDB: true})
	session.Config.ConnPool = wrapper
	session.Statement.ConnPool = wrapper
	f.store.svc.db = &database.DB{DB: session}
	e := receiverEnvelope(t, f.command())
	result, err := f.receiver.Execute(ctx, e)
	require.NoError(t, err)
	require.Error(t, ctx.Err())
	require.Equal(t, e.Identity, result.Identity)
	b1Await(t, p.entered)
	require.True(t, detached.Load())
	require.EqualValues(t, 1, p.calls.Load())
	// Duplicate under a fresh context reads the same committed receipt.
	receiverExecute(t, f, e)
	receiverCounts(t, f, 1, 1, 1)
	p.unblock()
	receiverWait(t, f, e)
}

func TestRuntimeReceiverTerminalFaultRetainsSlotAndStorageOnlyRetry(t *testing.T) {
	for _, mode := range []string{"phase", "final-write", "commit"} {
		t.Run(mode, func(t *testing.T) {
			f, p := receiverProbeFixture(t)
			e := receiverEnvelope(t, f.command())
			if mode == "phase" {
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER receiver_phase BEFORE UPDATE OF executor_state ON runtime_receipts WHEN NEW.executor_state = 'executing' BEGIN SELECT RAISE(ABORT, 'fixture phase'); END").Error)
			}
			if mode == "commit" {
				receiverDeferred(t, f, "AFTER INSERT ON activity_messages WHEN NEW.code = 'activity.success'")
			} else {
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER receiver_fault BEFORE INSERT ON activity_messages WHEN NEW.code IN ('activity.success','runtime.failed') BEGIN SELECT RAISE(ABORT, 'fixture final'); END").Error)
			}
			receiverExecute(t, f, e)
			owner := b2Owner(t, f.b1FixtureInternal, e.Identity.RunID)
			if mode != "phase" {
				b1Await(t, p.entered)
				p.unblock()
			}
			b1Await(t, owner.attemptDone)
			slotsHeld(t, f.b1FixtureInternal, 1)
			row := b3Receipt(t, f.b1FixtureInternal, e.Identity.RunID)
			require.Nil(t, row.SettledAt)
			f.store.svc.runningMu.Lock()
			require.Error(t, owner.err)
			require.NotNil(t, owner.result)
			frozen := *owner.result
			f.store.svc.runningMu.Unlock()
			if mode == "phase" {
				require.Nil(t, row.ExecutorStartedAt)
				require.Zero(t, p.calls.Load())
				require.Equal(t, "accepted", *row.ExecutorState)
			} else {
				require.EqualValues(t, 1, p.calls.Load())
			}
			other := e
			other.Identity.RunID = uuid.NewString()
			result, err := f.receiver.Execute(t.Context(), other)
			receiverZero(t, result)
			var busy *GateConflict
			require.ErrorAs(t, err, &busy)
			require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER receiver_fault").Error)
			if mode == "phase" {
				require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER receiver_phase").Error)
			}
			done := make(chan error, 8)
			for range 8 {
				go func() {
					_, err := f.service.RetrySettlement(t.Context(), RuntimeRunReference{Store: f.receiver.store, RunID: e.Identity.RunID})
					done <- err
				}()
			}
			for range 8 {
				select {
				case err := <-done:
					require.NoError(t, err)
				case <-time.After(8 * time.Second):
					t.Fatal("retry did not join")
				}
			}
			slotsHeld(t, f.b1FixtureInternal, 0)
			b2FinalCount(t, f.b1FixtureInternal, e.Identity.RunID, 1)
			row = b3Receipt(t, f.b1FixtureInternal, e.Identity.RunID)
			require.Equal(t, frozen.settledAt, *row.ExecutorSettledAt)
			require.Equal(t, frozen.state, *row.ExecutorState)
			require.EqualValues(t, map[bool]int64{true: 0, false: 1}[mode == "phase"], p.calls.Load())
		})
	}
}

func TestRuntimeReceiverCancelFailureDoesNotSignal(t *testing.T) {
	f, p := receiverProbeFixture(t)
	e := receiverEnvelope(t, f.command())
	receiverExecute(t, f, e)
	b1Await(t, p.entered)
	owner := b2Owner(t, f.b1FixtureInternal, e.Identity.RunID)
	before := runtimeBoundarySnapshot(t, f.store)
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER receiver_fault BEFORE INSERT ON activity_messages WHEN NEW.code = 'run.cancelling' BEGIN SELECT RAISE(ABORT, 'fixture cancel audit'); END").Error)
	result, err := f.receiver.Cancel(t.Context(), e.Identity, e.Attribution)
	receiverZero(t, result)
	require.Error(t, err)
	require.NoError(t, owner.ctx.Err())
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER receiver_fault").Error)
	p.unblock()
	receiverWait(t, f, e)
}

type receiverBeginObserver struct {
	*sql.DB
	entered chan struct{}
	once    sync.Once
}

func (p *receiverBeginObserver) GetDBConn() (*sql.DB, error) { return p.DB, nil }
func (p *receiverBeginObserver) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	p.once.Do(func() { close(p.entered) })
	return p.DB.BeginTx(ctx, opts)
}

func TestRuntimeReceiverRealSQLiteFullRetainsSlot(t *testing.T) {
	f, p := receiverProbeFixture(t)
	e := receiverEnvelope(t, f.command())
	receiverExecute(t, f, e)
	b1Await(t, p.entered)
	owner := b2Owner(t, f.b1FixtureInternal, e.Identity.RunID)
	before := runtimeBoundarySnapshot(t, f.store)
	require.NoError(t, f.store.svc.db.Exec("CREATE TABLE receiver_growth(payload BLOB NOT NULL)").Error)
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER receiver_full AFTER INSERT ON activity_messages WHEN NEW.code = 'activity.success' BEGIN INSERT INTO receiver_growth VALUES(zeroblob(1048576)); END").Error)
	var pages int64
	require.NoError(t, f.store.svc.db.Raw("PRAGMA page_count").Scan(&pages).Error)
	require.NoError(t, f.store.svc.db.Exec(fmt.Sprintf("PRAGMA max_page_count = %d", pages+2)).Error)
	// Restore storage before cleanup joins or retries, even on assertion failure.
	defer func() {
		_ = f.store.svc.db.Exec("PRAGMA max_page_count = 1000000").Error
		_ = f.store.svc.db.Exec("DROP TRIGGER IF EXISTS receiver_full").Error
	}()
	p.unblock()
	b1Await(t, owner.attemptDone)
	f.store.svc.runningMu.Lock()
	writeErr := owner.err
	frozen := *owner.result
	f.store.svc.runningMu.Unlock()
	var sqliteErr *sqlite.Error
	require.ErrorAs(t, writeErr, &sqliteErr)
	require.Equal(t, sqlitecode.SQLITE_FULL, sqliteErr.Code()&0xff)
	require.False(t, dbutil.IsRetryableWriteError(writeErr))
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	slotsHeld(t, f.b1FixtureInternal, 1)
	require.NoError(t, f.store.svc.db.Exec("PRAGMA max_page_count = 1000000").Error)
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER receiver_full").Error)
	snapshot, err := f.service.RetrySettlement(t.Context(), RuntimeRunReference{Store: f.receiver.store, RunID: e.Identity.RunID})
	require.NoError(t, err)
	require.Equal(t, "success", string(snapshot.Status))
	row := b3Receipt(t, f.b1FixtureInternal, e.Identity.RunID)
	require.Equal(t, frozen.settledAt, *row.ExecutorSettledAt)
	require.EqualValues(t, 1, p.calls.Load())
	slotsHeld(t, f.b1FixtureInternal, 0)
	b2FinalCount(t, f.b1FixtureInternal, e.Identity.RunID, 1)
}

func TestRuntimeReceiverQueryIgnoredFenceNeverAnswersAbsence(t *testing.T) {
	f, p := receiverProbeFixture(t)
	e := receiverEnvelope(t, f.command())
	before := runtimeBoundarySnapshot(t, f.store)
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER receiver_fault BEFORE INSERT ON runtime_receipts WHEN NEW.kind = 'not_received' BEGIN SELECT RAISE(IGNORE); END").Error)
	result, err := f.receiver.Query(t.Context(), e.Identity)
	receiverZero(t, result)
	require.Error(t, err)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	require.Zero(t, p.calls.Load())
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER receiver_fault").Error)
}
