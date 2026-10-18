// Source: docklet-next/docs/design/runtime-control.md
// Revision: c641dc24b3f364ac5e4ec03bc9c5dfe2a10d5f61a6bcba825785f9dc351a9e50
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Actual SQLite atomicity, identity, races and immutable evidence proofs.

package activity

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type runtimeFixtureInternal struct {
	t          *testing.T
	svc        *ActivityService
	now        time.Time
	generation string
	path       string
}

func runtimeFixture(t *testing.T) *runtimeFixtureInternal {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runtime.db")
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	f := &runtimeFixtureInternal{t: t, svc: NewActivityService(db, nil), path: path,
		now: time.Date(2026, 10, 19, 12, 0, 0, 123456789, time.FixedZone("fixture", 3600))}
	f.svc.clock = func() time.Time { return f.now }
	require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
		f.generation, err = runtimeGenerationTxInternal(tx, now)
		return err
	}))
	return f
}

func (f *runtimeFixtureInternal) tx(fn func(*gorm.DB, time.Time) error) error {
	f.svc.runningMu.Lock()
	defer f.svc.runningMu.Unlock()
	return f.svc.db.WithContext(f.t.Context()).Transaction(func(tx *gorm.DB) error {
		return fn(tx, runtimeTimeInternal(f.svc.nowInternal()))
	})
}

func (f *runtimeFixtureInternal) request() runtimeAdmissionInternal {
	id := uuid.NewString()
	return runtimeAdmissionInternal{
		Identity: runtimeIdentityInternal{RunID: id, StoreGeneration: f.generation, ManagerID: "example-manager",
			RegistrationID: "example-registration", EnvironmentID: "0", ExecutorGeneration: f.generation,
			RequestDigest: "example-canonical-digest"},
		Binding:     runtimeBindingInternal{Key: uuid.NewString(), CallerID: "example-owner", CallerName: new("Original operator")},
		OperationID: "deploy-project", TargetKind: "stack", TargetID: id, TargetName: new("Example stack"),
		GateScope: "stack:" + id, GateKey: "deploy-project:example-canonical-digest",
	}
}

func (f *runtimeFixtureInternal) admit(request runtimeAdmissionInternal) (*RuntimeReceipt, error) {
	var row *RuntimeReceipt
	err := f.tx(func(tx *gorm.DB, now time.Time) error {
		var err error
		row, err = runtimeAdmitTxInternal(tx, now, request)
		return err
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (f *runtimeFixtureInternal) lookup(request runtimeAdmissionInternal) (*RuntimeReceipt, error) {
	id := request.Identity
	id.RunID = ""
	var row *RuntimeReceipt
	err := f.tx(func(tx *gorm.DB, _ time.Time) error {
		var err error
		row, err = runtimeLookupBindingTxInternal(tx, id, request.Binding)
		return err
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (f *runtimeFixtureInternal) join(id runtimeIdentityInternal, binding runtimeBindingInternal) (*RuntimeReceipt, error) {
	var row *RuntimeReceipt
	err := f.tx(func(tx *gorm.DB, now time.Time) error {
		var err error
		row, err = runtimeJoinTxInternal(tx, now, id, binding)
		return err
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

func runtimeCount(t *testing.T, tx *gorm.DB, model any, want int64) {
	t.Helper()
	var count int64
	require.NoError(t, tx.Model(model).Count(&count).Error)
	require.Equal(t, want, count)
}

func runtimeMismatch(t *testing.T, row *RuntimeReceipt, err error, secret string) {
	t.Helper()
	var conflict *runtimeConflictInternal
	require.ErrorAs(t, err, &conflict)
	require.Nil(t, row)
	require.Equal(t, "runtime request mismatch", err.Error())
	require.NotContains(t, fmt.Sprintf("%+v", conflict), secret)
}

func TestRuntimeStoreBindingsAndRetainedRetry(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	// A v1 UUID is opaque too; its embedded timestamp never controls admission.
	request.Binding.Key = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	original, err := f.admit(request)
	require.NoError(t, err)
	require.Equal(t, request.Identity.RunID, original.RunID)
	require.Equal(t, runtimeTimeInternal(f.now), *original.AdmittedAt)
	require.Equal(t, time.UTC, original.AdmittedAt.Location())
	storedActivity := matrixRow(t, f.svc.db.DB, original.RunID)
	require.Equal(t, original.RunID, storedActivity.ID)
	require.Equal(t, request.Binding.CallerID, *storedActivity.StartedByUserID)
	for _, age := range []time.Duration{9 * time.Minute, 10 * time.Minute, 11 * time.Minute, 25 * time.Hour} {
		f.now = original.AdmittedAt.Add(age)
		retry := request
		retry.Identity.RunID = uuid.NewString()
		row, err := f.admit(retry)
		require.NoError(t, err)
		require.Equal(t, original, row)
	}
	runtimeCount(t, f.svc.db.DB, &RuntimeReceipt{}, 1)
	runtimeCount(t, f.svc.db.DB, &RuntimeKeyBinding{}, 1)
	runtimeCount(t, f.svc.db.DB, &Activity{}, 1)
	joinedKey := runtimeBindingInternal{Key: uuid.NewString(), CallerID: "example-joiner", CallerName: new("Joining operator")}
	joined, err := f.join(request.Identity, joinedKey)
	require.NoError(t, err)
	require.Equal(t, original.CallerID, joined.CallerID)
	require.Equal(t, original.CallerName, joined.CallerName)
	require.Equal(t, runtimeTimeInternal(f.now), joined.FinalWriteAt)
	require.Equal(t, storedActivity, matrixRow(t, f.svc.db.DB, original.RunID))
	var binding RuntimeKeyBinding
	require.NoError(t, f.svc.db.First(&binding, "idempotency_key = ?", joinedKey.Key).Error)
	require.Equal(t, joinedKey.CallerID, binding.CallerID)
	require.Equal(t, joinedKey.CallerName, binding.CallerName)
	require.Equal(t, runtimeTimeInternal(f.now), binding.FirstSeenAt)
	f.now = f.now.Add(time.Hour)
	again, err := f.join(request.Identity, joinedKey)
	require.NoError(t, err)
	require.Equal(t, joined, again) // Existing-key reads do not extend retention.
	for _, mutate := range []func(*runtimeAdmissionInternal){
		func(r *runtimeAdmissionInternal) { r.Binding.CallerID = "example-other-caller" },
		func(r *runtimeAdmissionInternal) { r.Identity.RequestDigest = "different-digest" },
		func(r *runtimeAdmissionInternal) { r.Identity.StoreGeneration = uuid.NewString() },
		func(r *runtimeAdmissionInternal) { r.Identity.ExecutorGeneration = uuid.NewString() },
		func(r *runtimeAdmissionInternal) { r.Identity.ManagerID = "example-other-manager" },
		func(r *runtimeAdmissionInternal) { r.Identity.RegistrationID = "example-other-registration" },
	} {
		changed := request
		mutate(&changed)
		row, err := f.lookup(changed)
		runtimeMismatch(t, row, err, original.RunID)
		row, err = f.admit(changed)
		runtimeMismatch(t, row, err, original.RunID)
	}
	wrongEnvironment := request
	wrongEnvironment.Identity.EnvironmentID = "example-other-environment"
	row, err := f.lookup(wrongEnvironment)
	require.Nil(t, row)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	// Environment-scoped keys may be reused by a genuinely distinct admission.
	wrongEnvironment.Identity.RunID = uuid.NewString()
	row, err = f.admit(wrongEnvironment)
	require.NoError(t, err)
	require.NotEqual(t, original.RunID, row.RunID)
	for _, key := range []string{"", "not-a-uuid", "urn:uuid:" + uuid.NewString(), "12345678-1234-1234-1234-12345678901z"} {
		bad := request
		bad.Binding.Key = key
		row, err := f.admit(bad)
		require.Nil(t, row)
		require.ErrorIs(t, err, errRuntimeInvalidInternal)
	}
	changedJoin := joinedKey
	changedJoin.CallerID = "example-other-caller"
	row, err = f.join(request.Identity, changedJoin)
	runtimeMismatch(t, row, err, original.RunID)
}

func TestRuntimeStoreRollbackAtEveryAdmissionAndJoinBoundary(t *testing.T) {
	for _, table := range []string{"runtime_receipts", "activities", "runtime_key_bindings"} {
		t.Run(table, func(t *testing.T) {
			f := runtimeFixture(t)
			require.NoError(t, f.svc.db.Exec("CREATE TRIGGER reject_write BEFORE INSERT ON "+table+" BEGIN SELECT RAISE(ABORT, 'fixture write rejected'); END").Error)
			row, err := f.admit(f.request())
			require.Nil(t, row)
			require.ErrorContains(t, err, "fixture write rejected")
			runtimeCount(t, f.svc.db.DB, &RuntimeReceipt{}, 0)
			runtimeCount(t, f.svc.db.DB, &RuntimeKeyBinding{}, 0)
			runtimeCount(t, f.svc.db.DB, &Activity{}, 0)
		})
	}
	for _, statement := range []string{
		"BEFORE INSERT ON runtime_key_bindings", "BEFORE UPDATE OF final_write_at ON runtime_receipts",
	} {
		t.Run(statement, func(t *testing.T) {
			f := runtimeFixture(t)
			request := f.request()
			original, err := f.admit(request)
			require.NoError(t, err)
			f.now = f.now.Add(time.Hour)
			require.NoError(t, f.svc.db.Exec("CREATE TRIGGER reject_join "+statement+" BEGIN SELECT RAISE(ABORT, 'fixture join rejected'); END").Error)
			row, err := f.join(request.Identity, runtimeBindingInternal{Key: uuid.NewString(), CallerID: "example-joiner"})
			require.Nil(t, row)
			require.ErrorContains(t, err, "fixture join rejected")
			row, err = f.lookup(request)
			require.NoError(t, err)
			require.Equal(t, original, row)
			runtimeCount(t, f.svc.db.DB, &RuntimeKeyBinding{}, 1)
			runtimeCount(t, f.svc.db.DB, &Activity{}, 1)
		})
	}
}

func TestRuntimeStoreScopeUniquenessAndAdmissionIdentity(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	original, err := f.admit(request)
	require.NoError(t, err)
	contender := request
	contender.Identity.RunID = uuid.NewString()
	contender.Binding.Key = uuid.NewString()
	row, err := f.admit(contender)
	runtimeMismatch(t, row, err, original.RunID)
	contender.Identity.RunID = original.RunID
	contender.TargetID = "other-target"
	contender.GateScope = "stack:other-target"
	row, err = f.admit(contender)
	runtimeMismatch(t, row, err, original.RunID)
	for _, mutate := range []func(*runtimeAdmissionInternal){
		func(r *runtimeAdmissionInternal) { r.Identity.RunID = "malformed" },
		func(r *runtimeAdmissionInternal) { r.OperationID = "remove-project" },
		func(r *runtimeAdmissionInternal) { r.GateScope = "stack:wrong-target" },
		func(r *runtimeAdmissionInternal) { r.ParentProjectID = new("example-parent") },
	} {
		bad := f.request()
		mutate(&bad)
		row, err := f.admit(bad)
		require.Nil(t, row)
		require.ErrorIs(t, err, errRuntimeInvalidInternal)
	}
	for operation, target := range map[string]string{"down-project": "stack", "restart-project": "stack", "redeploy-project": "stack",
		"start-container": "container", "stop-container": "container", "restart-container": "container"} {
		r := f.request()
		r.OperationID, r.TargetKind = operation, target
		r.GateScope = target + ":" + r.TargetID
		if target == "container" {
			r.ParentProjectID = new("example-parent")
		}
		row, err := f.admit(r)
		require.NoError(t, err)
		require.Equal(t, operation, *row.OperationID)
	}
}

func TestRuntimeStoreExecutorEvidenceAndIndependentDisposition(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	original, err := f.admit(request)
	require.NoError(t, err)
	apply := func(e runtimeEvidenceInternal) (*RuntimeReceipt, error) {
		var row *RuntimeReceipt
		err := f.tx(func(tx *gorm.DB, now time.Time) error {
			var err error
			row, err = runtimeEvidenceTxInternal(tx, now, request.Identity, e)
			return err
		})
		if err != nil {
			return nil, err
		}
		return row, nil
	}
	f.now = f.now.Add(time.Hour)
	unknown := runtimeEvidenceInternal{State: "unknown", UnknownReason: new("executor-unavailable")}
	row, err := apply(unknown)
	require.NoError(t, err)
	require.Nil(t, row.SettledAt)
	require.Equal(t, original.AdmittedAt, row.AdmittedAt)
	require.Equal(t, original.CallerID, row.CallerID)
	dispose := runtimeDispositionInternal{Disposition: "released", By: "example-admin", Reason: "Executor stopped and inspected", ReleaseAcknowledged: true}
	require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
		_, err := runtimeDispositionTxInternal(tx, now, request.Identity, dispose)
		return err
	}))
	before, err := f.lookup(request)
	require.NoError(t, err)
	require.Equal(t, "unknown", *before.ExecutorState)
	require.NotNil(t, before.SettledAt)
	f.now = f.now.Add(time.Hour)
	// Cross-clock executor times deliberately do not control local final-write time.
	evidence := runtimeEvidenceInternal{State: "success", ResultCode: new("example-success"),
		StartedAt: new(f.now.Add(24 * time.Hour)), SettledAt: new(f.now.Add(25 * time.Hour))}
	after, err := apply(evidence)
	require.NoError(t, err)
	require.Equal(t, before.HubDisposition, after.HubDisposition)
	require.Equal(t, before.HubDispositionAt, after.HubDispositionAt)
	require.Equal(t, before.HubDispositionBy, after.HubDispositionBy)
	require.Equal(t, before.HubDispositionReason, after.HubDispositionReason)
	require.True(t, after.ReleaseAcknowledged)
	require.Equal(t, before.SettledAt, after.SettledAt)
	require.Equal(t, runtimeTimeInternal(f.now), after.FinalWriteAt)
	f.now = f.now.Add(time.Hour)
	again, err := apply(evidence)
	require.NoError(t, err)
	require.Equal(t, after, again)
	require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
		_, err := runtimeDispositionTxInternal(tx, now, request.Identity, dispose)
		return err
	}))
	again, err = f.lookup(request)
	require.NoError(t, err)
	require.Equal(t, after, again)
	for _, change := range []runtimeEvidenceInternal{
		unknown, {State: "failed", SettledAt: evidence.SettledAt},
		{State: "success", SettledAt: evidence.SettledAt, ResultCode: new("changed")},
	} {
		row, err := apply(change)
		runtimeMismatch(t, row, err, original.RunID)
	}
	dispose.Reason = "Changed reason"
	err = f.tx(func(tx *gorm.DB, now time.Time) error {
		_, err := runtimeDispositionTxInternal(tx, now, request.Identity, dispose)
		return err
	})
	var conflict *runtimeConflictInternal
	require.ErrorAs(t, err, &conflict)
	for _, bad := range []runtimeEvidenceInternal{
		{State: "pending"}, {State: "unknown"}, {State: "success"},
		{State: "accepted", Error: new("not terminal")}, {State: "unknown", UnknownReason: new("")},
	} {
		row, err := apply(bad)
		require.Nil(t, row)
		require.ErrorIs(t, err, errRuntimeInvalidInternal)
	}
}

func TestRuntimeStoreEvidenceComposesWithActivityAndRollsBack(t *testing.T) {
	for _, failing := range []string{"runtime_receipts", "activities", "activity_messages"} {
		t.Run(failing, func(t *testing.T) {
			f := runtimeFixture(t)
			request := f.request()
			before, err := f.admit(request)
			require.NoError(t, err)
			verb := "UPDATE ON "
			if failing == "activity_messages" {
				verb = "INSERT ON "
			}
			require.NoError(t, f.svc.db.Exec("CREATE TRIGGER reject_terminal BEFORE "+verb+failing+" BEGIN SELECT RAISE(ABORT, 'fixture terminal rejected'); END").Error)
			err = f.tx(func(tx *gorm.DB, now time.Time) error {
				_, err := runtimeEvidenceTxInternal(tx, now, request.Identity, runtimeEvidenceInternal{State: "failed", SettledAt: new(now)})
				if err != nil {
					return err
				}
				if err := tx.Model(&Activity{}).Where("id = ? AND status = ?", request.Identity.RunID, activitytypes.StatusRunning).
					Updates(map[string]any{"status": activitytypes.StatusFailed, "ended_at": now}).Error; err != nil {
					return err
				}
				return tx.Create(&ActivityMessage{ActivityID: request.Identity.RunID, Level: activitytypes.MessageLevelError, Message: "Interrupted"}).Error
			})
			require.ErrorContains(t, err, "fixture terminal rejected")
			row, err := f.lookup(request)
			require.NoError(t, err)
			require.Equal(t, before, row)
			require.Equal(t, activitytypes.StatusRunning, matrixRow(t, f.svc.db.DB, before.RunID).Status)
			runtimeCount(t, f.svc.db.DB, &ActivityMessage{}, 0)
		})
	}
}

// A second test-only pool shares the locked owner's file and production SQLite
// policy. It never acquires or bypasses a second production lifetime ownership.
func runtimeContender(t *testing.T, f *runtimeFixtureInternal) *gorm.DB {
	t.Helper()
	pool, err := sqlitestore.Open(t.Context(), f.path, true, false)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	db, err := gorm.Open(sqlitestore.Dialector(pool), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	return db
}

func TestRuntimeStoreConcurrentGenerationAndKey(t *testing.T) {
	f := runtimeFixture(t)
	require.NoError(t, f.svc.db.Where("id = 1").Delete(&RuntimeStoreGeneration{}).Error)
	other := runtimeContender(t, f)
	type result struct {
		generation string
		row        *RuntimeReceipt
		err        error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for _, db := range []*gorm.DB{f.svc.db.DB, other} {
		go func(db *gorm.DB) {
			<-start
			var generation string
			err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var err error
				generation, err = runtimeGenerationTxInternal(tx, f.now)
				return err
			})
			results <- result{generation: generation, err: err}
		}(db)
	}
	close(start)
	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.Equal(t, first.generation, second.generation)
	f.generation = first.generation
	runtimeCount(t, other, &RuntimeStoreGeneration{}, 1)
	request := f.request()
	start = make(chan struct{})
	for _, db := range []*gorm.DB{f.svc.db.DB, other} {
		go func(db *gorm.DB) {
			<-start
			candidate := request
			candidate.Identity.RunID = uuid.NewString()
			var row *RuntimeReceipt
			err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var err error
				row, err = runtimeAdmitTxInternal(tx, f.now, candidate)
				return err
			})
			results <- result{row: row, err: err}
		}(db)
	}
	close(start)
	first, second = <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.Equal(t, first.row, second.row)
	runtimeCount(t, other, &RuntimeReceipt{}, 1)
	runtimeCount(t, other, &Activity{}, 1)
	runtimeCount(t, other, &RuntimeKeyBinding{}, 1)
}

func TestRuntimeStoreQueryAdmissionRace(t *testing.T) {
	for _, winner := range []string{"query", "admit"} {
		t.Run(winner, func(t *testing.T) {
			f := runtimeFixture(t)
			other := runtimeContender(t, f)
			request := f.request()
			reached, release, attempting := make(chan struct{}), make(chan struct{}), make(chan struct{})
			type result struct {
				row *RuntimeReceipt
				err error
			}
			results := make(chan result, 2)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			write := func(tx *gorm.DB, mode string) (*RuntimeReceipt, error) {
				if mode == "query" {
					return runtimeQueryTxInternal(tx, f.now, request.Identity)
				}
				return runtimeAdmitTxInternal(tx, f.now, request)
			}
			go func() {
				var row *RuntimeReceipt
				err := f.svc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
					var err error
					row, err = write(tx, winner)
					close(reached) // Winner has written, with its immediate transaction still open.
					select {
					case <-release:
						return err
					case <-ctx.Done():
						return ctx.Err()
					}
				})
				results <- result{row, err}
			}()
			select {
			case <-reached:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			go func() {
				var row *RuntimeReceipt
				close(attempting)
				err := other.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
					var err error
					mode := "query"
					if winner == "query" {
						mode = "admit"
					}
					row, err = write(tx, mode)
					return err
				})
				results <- result{row, err}
			}()
			<-attempting
			close(release)
			first, second := <-results, <-results
			if winner == "query" {
				var negative *RuntimeReceipt
				for _, got := range []result{first, second} {
					if got.err == nil {
						negative = got.row
					} else {
						runtimeMismatch(t, got.row, got.err, request.Identity.RunID)
					}
				}
				require.NotNil(t, negative)
				require.Equal(t, "not_received", negative.Kind)
				require.Nil(t, negative.CallerID)
				require.Nil(t, negative.AdmittedAt)
				require.Nil(t, negative.TargetID)
				runtimeCount(t, other, &Activity{}, 0)
				runtimeCount(t, other, &RuntimeKeyBinding{}, 0)
				row, err := f.join(request.Identity, request.Binding)
				runtimeMismatch(t, row, err, request.Identity.RunID)
			} else {
				require.NoError(t, first.err)
				require.NoError(t, second.err)
				require.Equal(t, "receipt", first.row.Kind)
				require.Equal(t, first.row, second.row)
				runtimeCount(t, other, &Activity{}, 1)
				runtimeCount(t, other, &RuntimeKeyBinding{}, 1)
			}
			runtimeCount(t, other, &RuntimeReceipt{}, 1)
			for _, mutate := range []func(*runtimeIdentityInternal){
				func(id *runtimeIdentityInternal) { id.StoreGeneration = uuid.NewString() },
				func(id *runtimeIdentityInternal) { id.ExecutorGeneration = uuid.NewString() },
				func(id *runtimeIdentityInternal) { id.ManagerID = "different-manager" },
				func(id *runtimeIdentityInternal) { id.RegistrationID = "different-registration" },
				func(id *runtimeIdentityInternal) { id.EnvironmentID = "different-environment" },
				func(id *runtimeIdentityInternal) { id.RequestDigest = "different-digest" },
			} {
				id := request.Identity
				mutate(&id)
				var row *RuntimeReceipt
				err := f.tx(func(tx *gorm.DB, now time.Time) error {
					var err error
					row, err = runtimeQueryTxInternal(tx, now, id)
					return err
				})
				runtimeMismatch(t, row, err, request.Identity.RunID)
			}
		})
	}
}

func TestRuntimeStoreMissingGenerationCannotAdmitOrProveAbsence(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	require.NoError(t, f.svc.db.Where("id = 1").Delete(&RuntimeStoreGeneration{}).Error)
	row, err := f.admit(request)
	require.Nil(t, row)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	err = f.tx(func(tx *gorm.DB, now time.Time) error {
		var err error
		row, err = runtimeQueryTxInternal(tx, now, request.Identity)
		return err
	})
	require.Nil(t, row)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	runtimeCount(t, f.svc.db.DB, &RuntimeReceipt{}, 0)
}

func TestRuntimeStoreCanonicalKeySpelling(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	original, err := f.admit(request)
	require.NoError(t, err)
	request.Binding.Key = strings.ToUpper(request.Binding.Key)
	row, err := f.admit(request)
	require.NoError(t, err)
	require.Equal(t, original, row)
	var conflict *runtimeConflictInternal
	require.False(t, errors.As(err, &conflict))
}

func TestRuntimeStoreCommitFailureReturnsNoIdentity(t *testing.T) {
	f := runtimeFixture(t)
	// All three writes succeed, then SQLite rejects the deferred FK at COMMIT.
	require.NoError(t, f.svc.db.Exec("CREATE TABLE fixture_parent(id INTEGER PRIMARY KEY)").Error)
	require.NoError(t, f.svc.db.Exec("CREATE TABLE fixture_deferred(id INTEGER REFERENCES fixture_parent(id) DEFERRABLE INITIALLY DEFERRED)").Error)
	require.NoError(t, f.svc.db.Exec("CREATE TRIGGER reject_commit AFTER INSERT ON runtime_key_bindings BEGIN INSERT INTO fixture_deferred VALUES(1); END").Error)
	row, err := f.admit(f.request())
	require.Nil(t, row)
	require.ErrorContains(t, err, "FOREIGN KEY constraint failed")
	runtimeCount(t, f.svc.db.DB, &RuntimeReceipt{}, 0)
	runtimeCount(t, f.svc.db.DB, &Activity{}, 0)
	runtimeCount(t, f.svc.db.DB, &RuntimeKeyBinding{}, 0)
	var count int
	require.NoError(t, f.svc.db.Raw("SELECT count(*) FROM fixture_deferred").Scan(&count).Error)
	require.Zero(t, count)
}

func TestRuntimeStoreNegativeQueryRequiresContinuousExecutor(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	request.Identity.ExecutorGeneration = uuid.NewString()
	var row *RuntimeReceipt
	err := f.tx(func(tx *gorm.DB, now time.Time) error {
		var err error
		row, err = runtimeQueryTxInternal(tx, now, request.Identity)
		return err
	})
	runtimeMismatch(t, row, err, request.Identity.RunID)
	runtimeCount(t, f.svc.db.DB, &RuntimeReceipt{}, 0)
	// A hub may bind a distinct remote generation on an admitted receipt; that
	// does not turn it into the executor's negative-evidence authority.
	row, err = f.admit(request)
	require.NoError(t, err)
	require.Equal(t, request.Identity.ExecutorGeneration, row.ExecutorGeneration)
}

func TestRuntimeStoreCompetingCallersCannotShareKey(t *testing.T) {
	f := runtimeFixture(t)
	other := runtimeContender(t, f)
	request := f.request()
	start := make(chan struct{})
	type result struct {
		row *RuntimeReceipt
		err error
	}
	results := make(chan result, 2)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for i, db := range []*gorm.DB{f.svc.db.DB, other} {
		go func(i int, db *gorm.DB) {
			<-start
			candidate := request
			candidate.Binding.CallerID = fmt.Sprintf("example-caller-%d", i)
			candidate.Identity.RunID = uuid.NewString()
			var row *RuntimeReceipt
			err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var err error
				row, err = runtimeAdmitTxInternal(tx, f.now, candidate)
				return err
			})
			results <- result{row, err}
		}(i, db)
	}
	close(start)
	first, second := <-results, <-results
	if first.err != nil {
		first, second = second, first
	}
	require.NoError(t, first.err)
	require.NotNil(t, first.row)
	runtimeMismatch(t, second.row, second.err, first.row.RunID)
	runtimeCount(t, other, &RuntimeReceipt{}, 1)
	runtimeCount(t, other, &Activity{}, 1)
	runtimeCount(t, other, &RuntimeKeyBinding{}, 1)
}

func TestRuntimeStoreExecutingAndTerminalWithoutDisposition(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	_, err := f.admit(request)
	require.NoError(t, err)
	started := f.now.Add(-time.Hour)
	for _, state := range []string{"accepted", "executing", "unknown", "executing", "failed"} {
		f.now = f.now.Add(time.Minute)
		evidence := runtimeEvidenceInternal{State: state, StartedAt: new(started)}
		if state == "unknown" {
			evidence.UnknownReason = new("executor-unavailable")
		}
		if state == "failed" {
			evidence.SettledAt = new(started.Add(time.Minute))
			evidence.Error = new("Example interruption")
		}
		require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
			row, err := runtimeEvidenceTxInternal(tx, now, request.Identity, evidence)
			if err != nil {
				return err
			}
			require.Equal(t, state, *row.ExecutorState)
			require.Nil(t, row.HubDisposition)
			if state == "failed" {
				require.Equal(t, new(now), row.SettledAt)
			} else {
				require.Nil(t, row.SettledAt)
			}
			return nil
		}))
	}
}
