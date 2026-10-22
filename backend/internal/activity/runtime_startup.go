// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Process-owned local interruption before runtime readiness.

package activity

import (
	"context"
	"errors"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

// ErrRuntimeStartupOwnersLive refuses recovery while this service still owns
// workers or retained outcomes. The caller must end that process lifetime first.
var ErrRuntimeStartupOwnersLive = errors.New("runtime startup has live local owners")

// SettleLocalStartup requires the caller to own the hub database lifetime lock
// after the old process has ended. Call it before intake, workers or readiness.
// The binding is trusted process identity, never a request or environment flag.
// Remote and uncertain receipts remain held for their owning reconciler.
func (s *RuntimeService) SettleLocalStartup(ctx context.Context, current RuntimeStoreBinding) (int64, error) {
	if err := s.checkInternal(); err != nil {
		return 0, err
	}
	if current.EnvironmentID != "0" || current.StoreGeneration == "" ||
		current.ExecutorGeneration != current.StoreGeneration || current.ManagerID == "" || current.RegistrationID == "" {
		return 0, errRuntimeInvalidInternal
	}
	if s.localExecution != nil && s.localExecution.Store != current {
		return 0, &runtimeConflictInternal{}
	}
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	if len(s.owners) != 0 {
		return 0, ErrRuntimeStartupOwnersLive
	}
	var candidates []string
	err := s.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		if err := runtimeCheckStoreTxInternal(tx, runtimeIdentityInternal{
			StoreGeneration: current.StoreGeneration, ExecutorGeneration: current.ExecutorGeneration,
			ManagerID: current.ManagerID, RegistrationID: current.RegistrationID,
			EnvironmentID: current.EnvironmentID, RequestDigest: "startup-local",
		}); err != nil {
			return err
		}
		return tx.Model(&RuntimeReceipt{}).Where("kind = 'receipt' AND settled_at IS NULL AND environment_id = ?", current.EnvironmentID).
			Where("store_generation = ? AND executor_generation = ? AND manager_id = ? AND registration_id = ?", current.StoreGeneration,
				current.ExecutorGeneration, current.ManagerID, current.RegistrationID).
			Where("dispatch_state = 'pending' AND executor_state IN ?", []string{"pending", "accepted", "executing"}).
			Order("run_id").Pluck("run_id", &candidates).Error
	})
	if err != nil {
		return 0, err
	}
	var repaired int64
	for _, runID := range candidates {
		// A run token grants only its own Activity writer. The receipt identity is
		// reread inside the same transaction that writes receipt, Activity and final.
		writeCtx, cancel := context.WithTimeout(context.WithValue(ctx, gateRunContextKey{}, runID), ownerWriteTimeout)
		var snapshot activitytypes.RuntimeSnapshot
		var final *ActivityMessage
		err = func() error {
			defer cancel()
			lock := s.activity.publishLockInternal(runID)
			lock.Lock()
			defer lock.Unlock()
			err := s.transactionInternal(writeCtx, func(tx *gorm.DB, now time.Time) error {
				var row RuntimeReceipt
				if err := tx.First(&row, "run_id = ?", runID).Error; err != nil {
					return err
				}
				if row.Kind != "receipt" || row.SettledAt != nil || row.StoreGeneration != current.StoreGeneration ||
					row.ExecutorGeneration != current.ExecutorGeneration || row.ManagerID != current.ManagerID ||
					row.RegistrationID != current.RegistrationID || row.EnvironmentID != current.EnvironmentID ||
					row.DispatchState == nil || *row.DispatchState != "pending" || row.ExecutorState == nil {
					return &runtimeConflictInternal{}
				}
				switch *row.ExecutorState {
				case "pending", "accepted", "executing":
				default:
					return &runtimeConflictInternal{}
				}
				message := "Runtime workflow interrupted; effects unverified"
				snapshot, final, err = runtimeCompleteTxInternal(tx, now, runtimeRowIdentityInternal(&row), runtimeLocalResultInternal{
					state: "failed", code: activitytypes.CodeRuntimeInterrupted, message: message,
					errorText: &message, startedAt: copyPtrInternal(row.ExecutorStartedAt), settledAt: now,
				})
				return err
			})
			if err != nil {
				return err
			}
			if final != nil {
				s.activity.publishMessageInternal(snapshot.EnvironmentID, snapshot.Type, activityMessageToDTOInternal(final))
				s.activity.publishActivityInternal(snapshot.Activity)
				repaired++
			}
			return nil
		}()
		if err != nil {
			return repaired, err
		}
	}
	return repaired, nil
}
