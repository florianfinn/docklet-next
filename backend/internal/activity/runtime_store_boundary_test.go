// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Isolated storage predicates and generation-error contracts on real SQLite.

package activity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Preserve the raw helper result on failure: the assertion must detect a leaked
// identity even if the enclosing transaction rolls back.
func runtimeBoundaryCall(f *runtimeFixtureInternal, request runtimeAdmissionInternal, operation string) (*RuntimeReceipt, error) {
	var row *RuntimeReceipt
	err := f.tx(func(tx *gorm.DB, now time.Time) error {
		var err error
		switch operation {
		case "read":
			row, err = runtimeReadTxInternal(tx, request.Identity)
		case "lookup":
			row, err = runtimeLookupBindingTxInternal(tx, request.Identity, request.Binding)
		case "admit":
			row, err = runtimeAdmitTxInternal(tx, now, request)
		case "join":
			row, err = runtimeJoinTxInternal(tx, now, request.Identity, request.Binding)
		case "query":
			row, err = runtimeQueryTxInternal(tx, now, request.Identity)
		case "evidence":
			row, err = runtimeEvidenceTxInternal(tx, now, request.Identity, runtimeEvidenceInternal{State: "accepted"})
		case "disposition":
			row, err = runtimeDispositionTxInternal(tx, now, request.Identity, runtimeDispositionInternal{
				Disposition: "interrupted", By: "example-admin", Reason: "Executor stopped"})
		default:
			f.t.Fatalf("unexpected operation %q", operation)
		}
		return err
	})
	return row, err
}

type runtimeBoundarySnapshotInternal struct {
	Receipts    []RuntimeReceipt
	Bindings    []RuntimeKeyBinding
	Activities  []Activity
	Messages    []ActivityMessage
	Generations []RuntimeStoreGeneration
}

// Read durable rows directly, independently of the helper whose guard is tested.
func runtimeBoundarySnapshot(t *testing.T, f *runtimeFixtureInternal) runtimeBoundarySnapshotInternal {
	t.Helper()
	var snapshot runtimeBoundarySnapshotInternal
	require.NoError(t, f.svc.db.Order("run_id").Find(&snapshot.Receipts).Error)
	require.NoError(t, f.svc.db.Order("environment_id, idempotency_key").Find(&snapshot.Bindings).Error)
	require.NoError(t, f.svc.db.Order("id").Find(&snapshot.Activities).Error)
	require.NoError(t, f.svc.db.Order("id").Find(&snapshot.Messages).Error)
	require.NoError(t, f.svc.db.Order("id").Find(&snapshot.Generations).Error)
	return snapshot
}

func runtimeBoundaryEvidence(f *runtimeFixtureInternal, id runtimeIdentityInternal, evidence runtimeEvidenceInternal) (*RuntimeReceipt, error) {
	var row *RuntimeReceipt
	err := f.tx(func(tx *gorm.DB, now time.Time) error {
		var err error
		row, err = runtimeEvidenceTxInternal(tx, now, id, evidence)
		return err
	})
	return row, err
}

func TestRuntimeStoreBoundaryTerminalEvidenceImmutable(t *testing.T) {
	for _, knownStart := range []bool{true, false} {
		name := "nil-start"
		if knownStart {
			name = "known-start"
		}
		t.Run(name, func(t *testing.T) {
			f := runtimeFixture(t)
			request := f.request()
			_, err := f.admit(request)
			require.NoError(t, err)
			// A separate hub disposition makes every follow-up SQL-valid, including
			// nonterminal evidence, while preserving its independent settlement.
			require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
				_, err := runtimeDispositionTxInternal(tx, now, request.Identity, runtimeDispositionInternal{
					Disposition: "released", By: "example-admin", Reason: "Executor stopped and inspected", ReleaseAcknowledged: true})
				return err
			}))
			var started *time.Time
			if knownStart {
				started = new(runtimeTimeInternal(f.now.Add(-time.Hour)))
			}
			settled := runtimeTimeInternal(f.now.Add(-time.Minute))
			terminal := runtimeEvidenceInternal{State: "success", StartedAt: started, SettledAt: new(settled), ResultCode: new("example-success")}
			_, err = runtimeBoundaryEvidence(f, request.Identity, terminal)
			require.NoError(t, err)
			before := runtimeBoundarySnapshot(t, f)
			f.now = f.now.Add(time.Hour)
			changes := []struct {
				name     string
				evidence runtimeEvidenceInternal
			}{
				{"failed", runtimeEvidenceInternal{State: "failed", StartedAt: started, SettledAt: new(settled), ResultCode: terminal.ResultCode}},
				{"cancelled", runtimeEvidenceInternal{State: "cancelled", StartedAt: started, SettledAt: new(settled), ResultCode: terminal.ResultCode}},
				{"result", runtimeEvidenceInternal{State: "success", StartedAt: started, SettledAt: new(settled), ResultCode: new("changed")}},
				{"error", runtimeEvidenceInternal{State: "success", StartedAt: started, SettledAt: new(settled), ResultCode: terminal.ResultCode, Error: new("changed")}},
				{"settled-time", runtimeEvidenceInternal{State: "success", StartedAt: started, SettledAt: new(settled.Add(time.Second)), ResultCode: terminal.ResultCode}},
				{"accepted", runtimeEvidenceInternal{State: "accepted", StartedAt: started}},
				{"executing", runtimeEvidenceInternal{State: "executing", StartedAt: started}},
				{"unknown", runtimeEvidenceInternal{State: "unknown", StartedAt: started, UnknownReason: new("executor-unavailable")}},
			}
			for _, change := range changes {
				t.Run(change.name, func(t *testing.T) {
					row, err := runtimeBoundaryEvidence(f, request.Identity, change.evidence)
					runtimeMismatch(t, row, err, request.Identity.RunID)
					require.Equal(t, before, runtimeBoundarySnapshot(t, f))
					row, err = runtimeBoundaryEvidence(f, request.Identity, terminal)
					require.NoError(t, err)
					require.Equal(t, before.Receipts[0], *row)
					require.Equal(t, before, runtimeBoundarySnapshot(t, f))
				})
			}
		})
	}
}

func TestRuntimeStoreBoundaryActiveEvidencePredicates(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	_, err := f.admit(request)
	require.NoError(t, err)
	started := runtimeTimeInternal(f.now.Add(-time.Minute))
	executing := runtimeEvidenceInternal{State: "executing", StartedAt: new(started)}
	_, err = runtimeBoundaryEvidence(f, request.Identity, executing)
	require.NoError(t, err)
	before := runtimeBoundarySnapshot(t, f)
	f.now = f.now.Add(time.Hour)
	for _, change := range []struct {
		name     string
		evidence runtimeEvidenceInternal
	}{
		{"regress-to-accepted", runtimeEvidenceInternal{State: "accepted", StartedAt: new(started)}},
		{"changed-start", runtimeEvidenceInternal{State: "executing", StartedAt: new(started.Add(time.Second))}},
		{"lost-start", runtimeEvidenceInternal{State: "executing"}},
	} {
		t.Run(change.name, func(t *testing.T) {
			row, err := runtimeBoundaryEvidence(f, request.Identity, change.evidence)
			runtimeMismatch(t, row, err, request.Identity.RunID)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f))
		})
	}
	row, err := runtimeBoundaryEvidence(f, request.Identity, executing)
	require.NoError(t, err)
	require.Equal(t, before.Receipts[0], *row)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f))
}

func TestRuntimeStoreBoundaryReplacementGeneration(t *testing.T) {
	for _, operation := range []string{"query", "admit", "lookup", "join"} {
		t.Run(operation, func(t *testing.T) {
			// Each entry point gets its own empty replacement store, so even a
			// regression in another subtest cannot create a masking receipt.
			f := runtimeFixture(t)
			request := f.request()
			old := uuid.NewString()
			require.NotEqual(t, f.generation, old)
			request.Identity.StoreGeneration, request.Identity.ExecutorGeneration = old, old
			before := runtimeBoundarySnapshot(t, f)
			require.Empty(t, before.Receipts)
			require.Empty(t, before.Bindings)
			require.Empty(t, before.Activities)
			require.Empty(t, before.Messages)
			require.Len(t, before.Generations, 1)
			require.Equal(t, f.generation, before.Generations[0].Generation)
			row, err := runtimeBoundaryCall(f, request, operation)
			runtimeMismatch(t, row, err, request.Identity.RunID)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f))
			request.Identity.StoreGeneration, request.Identity.ExecutorGeneration = f.generation, f.generation
			for _, absent := range []string{"read", "lookup", "join"} {
				row, err := runtimeBoundaryCall(f, request, absent)
				require.Nil(t, row)
				require.ErrorIs(t, err, gorm.ErrRecordNotFound)
				require.Equal(t, before, runtimeBoundarySnapshot(t, f))
			}
		})
	}
}

func TestRuntimeStoreBoundaryReadIdentityPredicates(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	_, err := f.admit(request)
	require.NoError(t, err)
	before := runtimeBoundarySnapshot(t, f)
	for _, change := range []struct {
		name   string
		mutate func(*runtimeIdentityInternal)
	}{
		{"manager", func(id *runtimeIdentityInternal) { id.ManagerID = "example-other-manager" }},
		{"registration", func(id *runtimeIdentityInternal) { id.RegistrationID = "example-other-registration" }},
		{"environment", func(id *runtimeIdentityInternal) { id.EnvironmentID = "example-other-environment" }},
		{"executor", func(id *runtimeIdentityInternal) { id.ExecutorGeneration = uuid.NewString() }},
		{"digest", func(id *runtimeIdentityInternal) { id.RequestDigest = "different-digest" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			changed := request
			change.mutate(&changed.Identity)
			row, err := runtimeBoundaryCall(f, changed, "read")
			runtimeMismatch(t, row, err, request.Identity.RunID)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f))
		})
	}
	// Own-store validation stays valid while only the receipt's stored generation
	// differs, so the row-match predicate must independently refuse stale evidence.
	require.NoError(t, f.svc.db.Model(&RuntimeReceipt{}).Where("run_id = ?", request.Identity.RunID).
		Update("store_generation", uuid.NewString()).Error)
	stale := runtimeBoundarySnapshot(t, f)
	row, err := runtimeBoundaryCall(f, request, "read")
	runtimeMismatch(t, row, err, request.Identity.RunID)
	require.Equal(t, stale, runtimeBoundarySnapshot(t, f))
}

func TestRuntimeStoreBoundaryBindingPredicates(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	_, err := f.admit(request)
	require.NoError(t, err)
	before := runtimeBoundarySnapshot(t, f)
	for _, change := range []string{"caller", "digest", "run"} {
		t.Run(change, func(t *testing.T) {
			changed := request
			switch change {
			case "caller":
				changed.Binding.CallerID = "example-other-caller"
			case "digest":
				changed.Identity.RequestDigest = "different-digest"
			case "run":
				changed.Identity.RunID = uuid.NewString()
			}
			operations := []string{"lookup", "join"}
			if change != "run" {
				operations = append(operations, "admit")
			}
			for _, operation := range operations {
				row, err := runtimeBoundaryCall(f, changed, operation)
				runtimeMismatch(t, row, err, request.Identity.RunID)
				require.Equal(t, before, runtimeBoundarySnapshot(t, f))
			}
		})
	}
	// Keep the receipt and request digest equal while varying only the binding:
	// otherwise runtimeRead's receipt digest check could mask this predicate.
	require.NoError(t, f.svc.db.Model(&RuntimeKeyBinding{}).Where("idempotency_key = ?", request.Binding.Key).
		Update("request_digest", "different-bound-digest").Error)
	changedBinding := runtimeBoundarySnapshot(t, f)
	for _, operation := range []string{"lookup", "join", "admit"} {
		row, err := runtimeBoundaryCall(f, request, operation)
		runtimeMismatch(t, row, err, request.Identity.RunID)
		require.Equal(t, changedBinding, runtimeBoundarySnapshot(t, f))
	}
	require.NoError(t, f.svc.db.Model(&RuntimeKeyBinding{}).Where("idempotency_key = ?", request.Binding.Key).
		Update("request_digest", request.Identity.RequestDigest).Error)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f))
	// Admission deliberately ignores a retry's newly proposed UUID.
	request.Identity.RunID = uuid.NewString()
	row, err := runtimeBoundaryCall(f, request, "admit")
	require.NoError(t, err)
	require.Equal(t, before.Receipts[0], *row)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f))
}

func TestRuntimeStoreBoundaryDispositionPredicates(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	_, err := f.admit(request)
	require.NoError(t, err)
	disposition := runtimeDispositionInternal{Disposition: "interrupted", By: "example-admin", Reason: "Executor stopped"}
	apply := func(value runtimeDispositionInternal) (*RuntimeReceipt, error) {
		var row *RuntimeReceipt
		err := f.tx(func(tx *gorm.DB, now time.Time) error {
			var err error
			row, err = runtimeDispositionTxInternal(tx, now, request.Identity, value)
			return err
		})
		return row, err
	}
	_, err = apply(disposition)
	require.NoError(t, err)
	before := runtimeBoundarySnapshot(t, f)
	f.now = f.now.Add(time.Hour)
	for _, field := range []string{"disposition", "by", "reason"} {
		t.Run(field, func(t *testing.T) {
			changed := disposition
			switch field {
			case "disposition":
				changed.Disposition = "refused"
			case "by":
				changed.By = "example-other-admin"
			case "reason":
				changed.Reason = "Different reason"
			}
			row, err := apply(changed)
			runtimeMismatch(t, row, err, request.Identity.RunID)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f))
			row, err = apply(disposition)
			require.NoError(t, err)
			require.Equal(t, before.Receipts[0], *row)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f))
		})
	}
}

func TestRuntimeStoreBoundaryMissingGeneration(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	require.NoError(t, f.svc.db.Where("id = 1").Delete(&RuntimeStoreGeneration{}).Error)
	before := runtimeBoundarySnapshot(t, f)
	require.Empty(t, before.Generations)
	for _, operation := range []string{"admit", "query", "lookup", "join", "read", "evidence", "disposition"} {
		t.Run(operation, func(t *testing.T) {
			row, err := runtimeBoundaryCall(f, request, operation)
			require.Nil(t, row)
			require.ErrorIs(t, err, errRuntimeGenerationUnavailableInternal)
			require.False(t, errors.Is(err, gorm.ErrRecordNotFound))
			var conflict *runtimeConflictInternal
			require.False(t, errors.As(err, &conflict))
			require.Equal(t, "runtime store generation unavailable", err.Error())
			require.Equal(t, before, runtimeBoundarySnapshot(t, f))
		})
	}
	// Explicit initialization remains the sole lazy-creation path.
	require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
		var err error
		f.generation, err = runtimeGenerationTxInternal(tx, now)
		return err
	}))
	request.Identity.StoreGeneration, request.Identity.ExecutorGeneration = f.generation, f.generation
	row, err := runtimeBoundaryCall(f, request, "lookup")
	require.Nil(t, row)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.False(t, errors.Is(err, errRuntimeGenerationUnavailableInternal))
	runtimeCount(t, f.svc.db.DB, &RuntimeReceipt{}, 0)
	runtimeCount(t, f.svc.db.DB, &RuntimeKeyBinding{}, 0)
	runtimeCount(t, f.svc.db.DB, &Activity{}, 0)
	runtimeCount(t, f.svc.db.DB, &RuntimeStoreGeneration{}, 1)
}

func TestRuntimeStoreBoundaryGenerationReadErrors(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	before := runtimeBoundarySnapshot(t, f)
	// Renaming the real table forces the driver's SELECT failure without a hook
	// or a synthetic gorm error; preserve all rows for restoration and comparison.
	require.NoError(t, f.svc.db.Exec("ALTER TABLE runtime_store_generation RENAME TO fixture_generation").Error)
	var driverError error
	for _, operation := range []string{"admit", "query", "lookup", "join"} {
		row, err := runtimeBoundaryCall(f, request, operation)
		require.Nil(t, row)
		require.ErrorContains(t, err, "no such table: runtime_store_generation")
		require.False(t, errors.Is(err, errRuntimeGenerationUnavailableInternal))
		require.False(t, errors.Is(err, gorm.ErrRecordNotFound))
		if driverError == nil {
			driverError = err
		} else {
			require.IsType(t, driverError, err)
			require.Equal(t, driverError.Error(), err.Error())
		}
	}
	require.NoError(t, f.svc.db.Exec("ALTER TABLE fixture_generation RENAME TO runtime_store_generation").Error)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := runtimeCheckStoreTxInternal(f.svc.db.WithContext(ctx), request.Identity)
	require.Equal(t, context.Canceled, err)
	require.False(t, errors.Is(err, errRuntimeGenerationUnavailableInternal))
	require.False(t, errors.Is(err, gorm.ErrRecordNotFound))
	require.Equal(t, before, runtimeBoundarySnapshot(t, f))
}

// Ordinary terminal evidence has no administrator disposition. Keep every
// follower terminal and the original start unchanged, so SQL/start predicates
// cannot mask the unconditional terminal immutability guard.
func TestRuntimeStoreBoundaryOrdinaryTerminalEvidenceImmutable(t *testing.T) {
	for _, knownStart := range []bool{true, false} {
		name := "nil-start"
		if knownStart {
			name = "known-start"
		}
		t.Run(name, func(t *testing.T) {
			f := runtimeFixture(t)
			request := f.request()
			_, err := f.admit(request)
			require.NoError(t, err)
			var started *time.Time
			if knownStart {
				started = new(runtimeTimeInternal(f.now.Add(-time.Hour)))
			}
			settled := runtimeTimeInternal(f.now.Add(-time.Minute))
			terminal := runtimeEvidenceInternal{State: "success", StartedAt: started, SettledAt: new(settled), ResultCode: new("example-success")}
			_, err = runtimeBoundaryEvidence(f, request.Identity, terminal)
			require.NoError(t, err)
			before := runtimeBoundarySnapshot(t, f)
			require.Nil(t, before.Receipts[0].HubDisposition)
			f.now = f.now.Add(time.Hour)
			for _, change := range []struct {
				name     string
				evidence runtimeEvidenceInternal
			}{
				{"failed", runtimeEvidenceInternal{State: "failed", StartedAt: started, SettledAt: new(settled), ResultCode: terminal.ResultCode}},
				{"cancelled", runtimeEvidenceInternal{State: "cancelled", StartedAt: started, SettledAt: new(settled), ResultCode: terminal.ResultCode}},
				{"result", runtimeEvidenceInternal{State: "success", StartedAt: started, SettledAt: new(settled), ResultCode: new("changed")}},
				{"error", runtimeEvidenceInternal{State: "success", StartedAt: started, SettledAt: new(settled), ResultCode: terminal.ResultCode, Error: new("changed")}},
				{"settled-time", runtimeEvidenceInternal{State: "success", StartedAt: started, SettledAt: new(settled.Add(time.Second)), ResultCode: terminal.ResultCode}},
			} {
				t.Run(change.name, func(t *testing.T) {
					row, err := runtimeBoundaryEvidence(f, request.Identity, change.evidence)
					runtimeMismatch(t, row, err, request.Identity.RunID)
					require.Equal(t, before, runtimeBoundarySnapshot(t, f))
					row, err = runtimeBoundaryEvidence(f, request.Identity, terminal)
					require.NoError(t, err)
					require.Equal(t, before.Receipts[0], *row)
					require.Equal(t, before, runtimeBoundarySnapshot(t, f))
				})
			}
		})
	}
}
