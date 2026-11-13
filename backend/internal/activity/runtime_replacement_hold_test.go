// Source: docklet-next/issues/366
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original replacement receipt boundary proofs.

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

func TestReplacementReceiptPredicate(t *testing.T) {
	for _, kind := range []string{"active", "settled", "pruned", "unknown", "absent", "self", "other-environment", "not-received"} {
		t.Run(kind, func(t *testing.T) {
			f := runtimeFixture(t)
			request := f.request()
			if kind == "other-environment" {
				request.Identity.EnvironmentID = "example-remote"
			}
			var row *RuntimeReceipt
			var err error
			if kind == "not-received" {
				row = runtimeNegative(t, f, request.Identity)
			} else {
				row, err = f.admit(request)
				require.NoError(t, err)
			}
			label, own := row.RunID, uuid.NewString()
			switch kind {
			case "settled":
				runtimeSettle(t, f, request)
			case "pruned":
				runtimeSettle(t, f, request)
				require.NoError(t, f.svc.db.Delete(&RuntimeKeyBinding{}, "run_id=?", row.RunID).Error)
				require.NoError(t, f.svc.db.Delete(row, "run_id=?", row.RunID).Error)
			case "unknown":
				label = uuid.NewString()
			case "absent":
				label = ""
			case "self":
				own = row.RunID
			}
			err = f.tx(func(tx *gorm.DB, _ time.Time) error { return replacementHoldTxInternal(tx, "0", label, own) })
			if kind == "active" {
				var gate *GateConflict
				require.ErrorAs(t, err, &gate, "active replacement must be busy")
				require.Equal(t, row.RunID, gate.RunID)
				require.Equal(t, "run.busy", gate.Code)
			} else {
				require.NoError(t, err, "nonmatching receipt must not hold replacement: %s", kind)
			}
		})
	}
}

func TestReplacementLookupFailureRefusesAdmission(t *testing.T) {
	f := b1Fixture(t)
	label := uuid.NewString()
	f.resolver.resolve = func(_ context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
		return RuntimeResolvedTarget{Target: target, ReplacementRunID: label}, nil
	}
	sentinel := errors.New("synthetic replacement lookup failure")
	require.NoError(t, f.store.svc.db.Callback().Query().Before("gorm:query").Register("replacement-read-error", func(tx *gorm.DB) {
		if tx.Statement.Table == "runtime_receipts" && len(tx.Statement.Selects) == 1 && tx.Statement.Selects[0] == "run_id" {
			tx.AddError(sentinel)
		}
	}))
	defer f.store.svc.db.Callback().Query().Remove("replacement-read-error")
	c := f.command()
	c.Operation = RuntimeStopContainer
	c.Target = RuntimeTarget{Kind: "container", ID: "replacement"}
	result, err := f.service.Admit(t.Context(), c)
	require.ErrorIs(t, err, sentinel, "lookup error must fail admission closed")
	b1Zero(t, result)
	var n int64
	require.NoError(t, f.store.svc.db.Model(&RuntimeReceipt{}).Count(&n).Error)
	require.Zero(t, n, "failed lookup must admit no competing effect")
}
