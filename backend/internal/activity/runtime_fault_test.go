// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Actual admission/audit/COMMIT/store/context faults, no execution claims.

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
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"modernc.org/sqlite"
	sqlitecode "modernc.org/sqlite/lib"
)

func TestRuntimeAdmissionFaultAtomicWrites(t *testing.T) {
	for _, table := range []string{"runtime_receipts", "activities", "runtime_key_bindings", "activity_messages"} {
		t.Run(table, func(t *testing.T) {
			f := b1Fixture(t)
			before := runtimeBoundarySnapshot(t, f.store)
			require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER reject_admission BEFORE INSERT ON "+table+" BEGIN SELECT RAISE(ABORT, 'fixture admission rejected'); END").Error)
			result, err := f.service.Admit(t.Context(), f.command())
			b1Zero(t, result)
			var storeError *RuntimeStoreError
			require.ErrorAs(t, err, &storeError)
			require.ErrorContains(t, err, "fixture admission rejected")
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			require.Empty(t, f.store.svc.running)
		})
	}
}

func TestRuntimeAdmissionFaultJoinWrites(t *testing.T) {
	for _, statement := range []string{"BEFORE INSERT ON runtime_key_bindings", "BEFORE UPDATE OF final_write_at ON runtime_receipts", "BEFORE INSERT ON activity_messages"} {
		t.Run(statement, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			_, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			f.store.now = f.store.now.Add(time.Hour)
			before := runtimeBoundarySnapshot(t, f.store)
			require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER reject_join "+statement+" BEGIN SELECT RAISE(ABORT, 'fixture join rejected'); END").Error)
			c.Key = uuid.NewString()
			c.Actor.ID = "example-joiner"
			result, err := f.service.Admit(t.Context(), c)
			b1Zero(t, result)
			require.ErrorContains(t, err, "fixture join rejected")
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
		})
	}
}

func TestRuntimeAdmissionFaultDeferredCommit(t *testing.T) {
	for _, join := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "join"}[join], func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			if join {
				_, err := f.service.Admit(t.Context(), c)
				require.NoError(t, err)
				c.Key = uuid.NewString()
				c.Actor.ID = "example-joiner"
			}
			before := runtimeBoundarySnapshot(t, f.store)
			require.NoError(t, f.store.svc.db.Exec("CREATE TABLE fixture_parent(id INTEGER PRIMARY KEY)").Error)
			require.NoError(t, f.store.svc.db.Exec("CREATE TABLE fixture_deferred(id INTEGER REFERENCES fixture_parent(id) DEFERRABLE INITIALLY DEFERRED)").Error)
			// The last required admission write succeeds; the actual SQLite COMMIT fails.
			require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER reject_commit AFTER INSERT ON activity_messages BEGIN INSERT INTO fixture_deferred VALUES(1); END").Error)
			result, err := f.service.Admit(t.Context(), c)
			b1Zero(t, result)
			require.ErrorContains(t, err, "FOREIGN KEY constraint failed")
			var storeError *RuntimeStoreError
			require.ErrorAs(t, err, &storeError)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			var count int
			require.NoError(t, f.store.svc.db.Raw("SELECT count(*) FROM fixture_deferred").Scan(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestRuntimeAdmissionFaultGenerationAndDriver(t *testing.T) {
	for _, mode := range []string{"missing", "mismatch", "driver"} {
		t.Run(mode, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			before := runtimeBoundarySnapshot(t, f.store)
			switch mode {
			case "missing":
				require.NoError(t, f.store.svc.db.Where("id = 1").Delete(&RuntimeStoreGeneration{}).Error)
				before = runtimeBoundarySnapshot(t, f.store)
			case "mismatch":
				c.Store.StoreGeneration = uuid.NewString()
				c.Store.ExecutorGeneration = c.Store.StoreGeneration
			case "driver":
				require.NoError(t, f.store.svc.db.Exec("ALTER TABLE runtime_store_generation RENAME TO fixture_generation").Error)
			}
			result, err := f.service.Admit(t.Context(), c)
			b1Zero(t, result)
			require.False(t, errors.Is(err, gorm.ErrRecordNotFound))
			if mode == "mismatch" {
				b1Mismatch(t, result, err, "unassigned")
			} else {
				var storeError *RuntimeStoreError
				require.ErrorAs(t, err, &storeError)
				if mode == "missing" {
					require.ErrorIs(t, err, errRuntimeGenerationUnavailableInternal)
				} else {
					require.ErrorContains(t, err, "no such table: runtime_store_generation")
					require.False(t, errors.Is(err, errRuntimeGenerationUnavailableInternal))
				}
			}
			if mode == "driver" {
				require.NoError(t, f.store.svc.db.Exec("ALTER TABLE fixture_generation RENAME TO runtime_store_generation").Error)
			}
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			require.Zero(t, f.resolver.calls.Load())
			if mode == "missing" {
				generation, err := f.service.InitializeGeneration(t.Context())
				require.NoError(t, err)
				require.NotEqual(t, f.store.generation, generation)
				var own RuntimeStoreGeneration
				require.NoError(t, f.store.svc.db.First(&own).Error)
				require.Equal(t, generation, own.Generation)
			}
		})
	}
}

func TestRuntimeAdmissionFaultResolver(t *testing.T) {
	for _, mode := range []string{"offline", "unresolved-parent", "changed-parent", "changed-id", "cancelled-after-io"} {
		t.Run(mode, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			c.Operation = RuntimeStartContainer
			c.Target = RuntimeTarget{Kind: "container", ID: "example-child"}
			before := runtimeBoundarySnapshot(t, f.store)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			cause := errors.New("fixture source unavailable")
			f.resolver.resolve = func(_ context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
				switch mode {
				case "offline":
					return RuntimeResolvedTarget{}, cause
				case "unresolved-parent":
					return RuntimeResolvedTarget{Target: target, ComposeManaged: true}, nil
				case "changed-parent":
					target.ParentProjectID = new("example-parent")
				case "changed-id":
					target.ID = "example-other-container"
				case "cancelled-after-io":
					cancel()
				}
				return RuntimeResolvedTarget{Target: target}, nil
			}
			result, err := f.service.Admit(ctx, c)
			b1Zero(t, result)
			switch mode {
			case "offline":
				var targetError *RuntimeTargetError
				require.ErrorAs(t, err, &targetError)
				require.ErrorIs(t, err, cause)
			case "unresolved-parent":
				var targetError *RuntimeTargetError
				require.ErrorAs(t, err, &targetError)
				require.Equal(t, string(activitytypes.CodeRuntimeParentUnresolved), targetError.Code)
			case "cancelled-after-io":
				require.ErrorIs(t, err, context.Canceled)
			default:
				b1Mismatch(t, result, err, "unassigned")
			}
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
		})
	}
}

func TestRuntimeAdmissionFaultPreCancelledAndMutexWait(t *testing.T) {
	f := b1Fixture(t)
	c := f.command()
	before := runtimeBoundarySnapshot(t, f.store)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := f.service.Admit(ctx, c)
	b1Zero(t, result)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	require.Zero(t, f.resolver.calls.Load())
	// Cancellation during the mutex wait must not turn into detached intake.
	f.store.svc.runningMu.Lock()
	started := make(chan struct{})
	done := make(chan struct{})
	ctx, cancel = context.WithCancel(t.Context())
	go func() { close(started); result, err = f.service.Admit(ctx, c); close(done) }()
	b1Await(t, started)
	cancel()
	f.store.svc.runningMu.Unlock()
	b1Await(t, done)
	b1Zero(t, result)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	require.Zero(t, f.resolver.calls.Load())
}

// The wrapper observes an actual pool BeginTx attempt without replacing its
// SQLite driver or result. The other pool retains an immediate writer lock.
type b1BeginObserverInternal struct {
	*sql.DB
	entered chan struct{}
	once    sync.Once
}

func (p *b1BeginObserverInternal) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	p.once.Do(func() { close(p.entered) })
	return p.DB.BeginTx(ctx, opts)
}

func TestRuntimeAdmissionFaultCancelledSQLiteLockWait(t *testing.T) {
	f := b1Fixture(t)
	other := f.contender(t, &b1ResolverInternal{})
	c := f.command()
	before := runtimeBoundarySnapshot(t, f.store)
	lock := f.store.svc.db.Begin()
	require.NoError(t, lock.Error)
	released := false
	defer func() {
		if !released {
			require.NoError(t, lock.Rollback().Error)
		}
	}()
	pool, err := other.activity.db.DB.DB()
	require.NoError(t, err)
	// Verify this is a real SQLite write-lock collision, not a mutex/pool wait.
	_, busyErr := pool.ExecContext(t.Context(), "BEGIN IMMEDIATE")
	require.Error(t, busyErr)
	require.True(t, dbutil.IsRetryableWriteError(busyErr), "%v", busyErr)
	observer := &b1BeginObserverInternal{DB: pool, entered: make(chan struct{})}
	session := other.activity.db.Session(&gorm.Session{NewDB: true})
	session.Config.ConnPool = observer
	session.Statement.ConnPool = observer
	other.activity = NewActivityService(&database.DB{DB: session}, nil)
	other.activity.clock = f.store.svc.clock
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	type answer struct {
		result RuntimeAdmissionResult
		err    error
	}
	done := make(chan answer, 1)
	go func() { r, e := other.Admit(ctx, c); done <- answer{r, e} }()
	b1Await(t, observer.entered)
	select {
	case answer := <-done:
		t.Fatalf("locked admission completed before cancellation: %v", answer.err)
	default:
	}
	cancel()
	got := <-done
	b1Zero(t, got.result)
	require.ErrorIs(t, got.err, context.Canceled)
	require.False(t, errors.Is(got.err, gorm.ErrRecordNotFound))
	require.NoError(t, lock.Rollback().Error)
	released = true
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
}

func TestRuntimeAdmissionCompactRetryAfterHistoryClear(t *testing.T) {
	for _, mode := range []string{"success", "unknown-release"} {
		t.Run(mode, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			id := b1Identity(t, c, admitted.ActivityID)
			// Explicit storage fixture, not an admission-to-finished runtime helper or a
			// claimed workflow. B2 owns the real conditional settlement transaction.
			b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
				if mode == "success" {
					_, err := runtimeEvidenceTxInternal(tx, now, id, runtimeEvidenceInternal{State: "success", SettledAt: new(now), ResultCode: new("activity.success")})
					if err != nil {
						return err
					}
				} else {
					if _, err := runtimeEvidenceTxInternal(tx, now, id, runtimeEvidenceInternal{State: "unknown", UnknownReason: new("executor-unavailable")}); err != nil {
						return err
					}
					if _, err := runtimeDispositionTxInternal(tx, now, id, runtimeDispositionInternal{Disposition: "released", By: "example-admin", Reason: "Executor stopped and inspected", ReleaseAcknowledged: true}); err != nil {
						return err
					}
				}
				status := activitytypes.StatusSuccess
				if mode != "success" {
					status = activitytypes.StatusFailed
				}
				return tx.Model(&Activity{}).Where("id = ?", id.RunID).Updates(map[string]any{"status": status, "ended_at": now, "gate_scope": "", "gate_key": ""}).Error
			})
			deleted, err := f.store.svc.DeleteHistory(t.Context(), c.Store.EnvironmentID)
			require.NoError(t, err)
			require.EqualValues(t, 1, deleted)
			before := runtimeBoundarySnapshot(t, f.store)
			require.Empty(t, before.Activities)
			require.Empty(t, before.Messages)
			f.store.now = f.store.now.Add(11 * time.Minute)
			f.resolver.resolve = func(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error) {
				return RuntimeResolvedTarget{}, errors.New("fixture offline")
			}
			retry, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			require.Equal(t, admitted.ActivityID, retry.ActivityID)
			require.Equal(t, activitytypes.CodeRunJoined, retry.Code)
			require.Equal(t, c.Actor.ID, retry.Activity.StartedBy.UserID)
			require.Nil(t, retry.Activity.Progress)
			require.Empty(t, retry.Activity.Metadata)
			require.Empty(t, retry.Activity.AffectedServices)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			if mode == "success" {
				require.Equal(t, activitytypes.StatusSuccess, retry.Activity.Status)
				require.Equal(t, activitytypes.RuntimeOutcomeKnown, retry.Activity.OutcomeState)
			} else {
				require.Equal(t, activitytypes.StatusFailed, retry.Activity.Status)
				require.Equal(t, activitytypes.RuntimeOutcomeUnknown, retry.Activity.OutcomeState)
				// Late executor knowledge does not overwrite the hub disposition/history.
				b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
					_, err := runtimeEvidenceTxInternal(tx, now, id, runtimeEvidenceInternal{State: "success", SettledAt: new(now)})
					return err
				})
				late, err := f.service.Admit(t.Context(), c)
				require.NoError(t, err)
				require.Equal(t, activitytypes.StatusFailed, late.Activity.Status)
			}
			foreign := c
			foreign.Actor.ID = "example-other-caller"
			leaked, err := f.service.Admit(t.Context(), foreign)
			b1Mismatch(t, leaked, err, id.RunID)
			f.resolver.resolve = nil
			c.Key = uuid.NewString()
			deliberate, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			require.Equal(t, activitytypes.CodeRunStarted, deliberate.Code)
			require.NotEqual(t, id.RunID, deliberate.ActivityID)
		})
	}
}

func TestRuntimeAdmissionFaultFinalGenerationRecheck(t *testing.T) {
	for _, missing := range []bool{true, false} {
		t.Run(map[bool]string{true: "missing", false: "replacement"}[missing], func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			f.resolver.resolve = func(_ context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
				if missing {
					require.NoError(t, f.store.svc.db.Where("id = 1").Delete(&RuntimeStoreGeneration{}).Error)
				} else {
					require.NoError(t, f.store.svc.db.Model(&RuntimeStoreGeneration{}).Where("id = 1").Update("generation", uuid.NewString()).Error)
				}
				return RuntimeResolvedTarget{Target: target}, nil
			}
			result, err := f.service.Admit(t.Context(), c)
			b1Zero(t, result)
			if missing {
				var unreadable *RuntimeStoreError
				require.ErrorAs(t, err, &unreadable)
				require.ErrorIs(t, err, errRuntimeGenerationUnavailableInternal)
			} else {
				b1Mismatch(t, result, err, "unassigned")
			}
			runtimeCount(t, f.store.svc.db.DB, &RuntimeReceipt{}, 0)
			runtimeCount(t, f.store.svc.db.DB, &Activity{}, 0)
			runtimeCount(t, f.store.svc.db.DB, &ActivityMessage{}, 0)
			runtimeCount(t, f.store.svc.db.DB, &RuntimeKeyBinding{}, 0)
		})
	}
}

func TestRuntimeAdmissionFaultCancelledBeforeCommit(t *testing.T) {
	f := b1Fixture(t)
	c := f.command()
	before := runtimeBoundarySnapshot(t, f.store)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reached := false
	name := "fixture_cancel_after_start"
	require.NoError(t, f.store.svc.db.Callback().Create().After("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "activity_messages" && tx.Error == nil {
			reached = true
			cancel()
		}
	}))
	defer func() { require.NoError(t, f.store.svc.db.Callback().Create().Remove(name)) }()
	result, err := f.service.Admit(ctx, c)
	require.True(t, reached, "cancellation must follow all required writes")
	b1Zero(t, result)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
}

// SQLite's real page quota forces file growth to fail inside the composed
// terminal transaction. This is SQLITE_FULL, not a synthetic RAISE message or
// proof that the host filesystem is full.
func TestRuntimeFaultRealSQLiteFullRetainsOwnerAndRetriesStorageOnly(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
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
	before := runtimeBoundarySnapshot(t, f.store)
	require.NoError(t, f.store.svc.db.Exec("CREATE TABLE fixture_b3_growth(payload BLOB NOT NULL)").Error)
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_b3_full AFTER INSERT ON activity_messages WHEN NEW.code = 'activity.success' BEGIN INSERT INTO fixture_b3_growth VALUES(zeroblob(1048576)); END").Error)
	var pages int64
	require.NoError(t, f.store.svc.db.Raw("PRAGMA page_count").Scan(&pages).Error)
	require.Greater(t, pages, int64(0))
	require.NoError(t, f.store.svc.db.Exec(fmt.Sprintf("PRAGMA max_page_count = %d", pages+2)).Error)
	close(release)
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
	b2Held(t, f, c, admitted.ActivityID)
	b2FinalCount(t, f, admitted.ActivityID, 0)
	require.EqualValues(t, 1, calls.Load())
	require.NoError(t, f.store.svc.db.Exec("PRAGMA max_page_count = 1000000").Error)
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER fixture_b3_full").Error)
	settled, err := f.service.RetrySettlement(t.Context(), b2Ref(c, admitted.ActivityID))
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, settled.Status)
	require.Equal(t, &frozen.settledAt, b3Receipt(t, f, admitted.ActivityID).ExecutorSettledAt)
	b2FinalCount(t, f, admitted.ActivityID, 1)
	require.EqualValues(t, 1, calls.Load())
}
