// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Narrow transaction-owned intent and local/dispatch markers; no worker or receiver.

package activity

import (
	"time"

	"gorm.io/gorm"
)

func runtimeMutableReceiptTxInternal(tx *gorm.DB, id runtimeIdentityInternal) (*RuntimeReceipt, error) {
	row, err := runtimeReadTxInternal(tx, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != "receipt" {
		return nil, &runtimeConflictInternal{}
	}
	return row, nil
}

// Persist before signaling a worker. Repeats and terminal requests are reads;
// this helper neither cancels a context nor settles/releases a live exclusion.
func runtimeCancelIntentTxInternal(tx *gorm.DB, now time.Time, id runtimeIdentityInternal) (*RuntimeReceipt, error) {
	row, err := runtimeMutableReceiptTxInternal(tx, id)
	if err != nil {
		return nil, err
	}
	if row.SettledAt != nil || row.CancelRequested {
		return row, nil
	}
	if err := runtimeUpdateTxInternal(tx, now, row, map[string]any{"cancel_requested": true, "cancel_requested_at": runtimeTimeInternal(now)}); err != nil {
		return nil, err
	}
	return row, nil
}

// Local acceptance has no remote send and leaves dispatch pending with null T.
// A separate caller-owned transaction must settle a pre-acceptance cancellation.
func runtimeLocalAcceptTxInternal(tx *gorm.DB, now time.Time, id runtimeIdentityInternal) (*RuntimeReceipt, error) {
	row, err := runtimeMutableReceiptTxInternal(tx, id)
	if err != nil {
		return nil, err
	}
	if row.ExecutorGeneration != row.StoreGeneration || row.SettledAt != nil || row.CancelRequested || *row.DispatchState != "pending" {
		return nil, &runtimeConflictInternal{}
	}
	if *row.ExecutorState == "accepted" {
		return row, nil
	}
	if *row.ExecutorState != "pending" {
		return nil, &runtimeConflictInternal{}
	}
	return runtimeEvidenceTxInternal(tx, now, id, runtimeEvidenceInternal{State: "accepted"})
}

func runtimeLocalExecutingTxInternal(tx *gorm.DB, now time.Time, id runtimeIdentityInternal) (*RuntimeReceipt, error) {
	row, err := runtimeMutableReceiptTxInternal(tx, id)
	if err != nil {
		return nil, err
	}
	if row.ExecutorGeneration != row.StoreGeneration || row.SettledAt != nil || row.CancelRequested || *row.DispatchState != "pending" {
		return nil, &runtimeConflictInternal{}
	}
	if *row.ExecutorState == "executing" {
		return row, nil
	}
	if *row.ExecutorState != "accepted" {
		return nil, &runtimeConflictInternal{}
	}
	return runtimeEvidenceTxInternal(tx, now, id, runtimeEvidenceInternal{State: "executing", StartedAt: new(runtimeTimeInternal(now))})
}

// A dispatch timestamp is written once before a possible send. Acknowledgment
// cannot manufacture T or replace it. These markers do not perform transport I/O.
func runtimeDispatchTxInternal(tx *gorm.DB, now time.Time, id runtimeIdentityInternal, accepted bool) (*RuntimeReceipt, error) {
	row, err := runtimeMutableReceiptTxInternal(tx, id)
	if err != nil {
		return nil, err
	}
	if row.SettledAt != nil {
		return nil, &runtimeConflictInternal{}
	}
	state := *row.DispatchState
	if accepted {
		if state == "accepted" {
			return row, nil
		}
		if state != "dispatching" || row.DispatchedAt == nil {
			return nil, &runtimeConflictInternal{}
		}
		if err := runtimeUpdateTxInternal(tx, now, row, map[string]any{"dispatch_state": "accepted"}); err != nil {
			return nil, err
		}
	} else {
		if state == "dispatching" {
			return row, nil
		}
		if state != "pending" || row.CancelRequested || *row.ExecutorState != "pending" {
			return nil, &runtimeConflictInternal{}
		}
		if err := runtimeUpdateTxInternal(tx, now, row, map[string]any{"dispatch_state": "dispatching", "dispatched_at": runtimeTimeInternal(now)}); err != nil {
			return nil, err
		}
	}
	return row, nil
}
