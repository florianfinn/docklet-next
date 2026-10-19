// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Trusted authorized-lookup cancellation intent commits before owner signaling.

package activity

import (
	"context"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RuntimeRunReference is an already authorized run lookup with trusted host
// continuity. It is not a browser dispatch envelope or proof of permission.
type RuntimeRunReference struct {
	Store RuntimeStoreBinding
	RunID string
}

func runtimeReferenceRowTxInternal(tx *gorm.DB, ref RuntimeRunReference) (*RuntimeReceipt, error) {
	id := runtimeIdentityInternal{RunID: ref.RunID, StoreGeneration: ref.Store.StoreGeneration,
		ManagerID: ref.Store.ManagerID, RegistrationID: ref.Store.RegistrationID, EnvironmentID: ref.Store.EnvironmentID,
		ExecutorGeneration: ref.Store.ExecutorGeneration, RequestDigest: "authorized-lookup"}
	if canonical, err := runtimeUUIDInternal(ref.RunID); err != nil || canonical != ref.RunID {
		return nil, errRuntimeInvalidInternal
	}
	if err := runtimeCheckStoreTxInternal(tx, id); err != nil {
		return nil, err
	}
	var row RuntimeReceipt
	if err := tx.First(&row, "run_id = ?", ref.RunID).Error; err != nil {
		return nil, err
	}
	id.RequestDigest = row.RequestDigest
	if row.Kind != "receipt" || !runtimeMatchesInternal(&row, id) {
		return nil, &runtimeConflictInternal{}
	}
	return &row, nil
}

func runtimeRowIdentityInternal(row *RuntimeReceipt) runtimeIdentityInternal {
	return runtimeIdentityInternal{RunID: row.RunID, StoreGeneration: row.StoreGeneration, ManagerID: row.ManagerID,
		RegistrationID: row.RegistrationID, EnvironmentID: row.EnvironmentID, ExecutorGeneration: row.ExecutorGeneration, RequestDigest: row.RequestDigest}
}

// Cancel requires current activities:read/cancel grants for this run. The later
// HTTP owner must enforce them before this trusted lookup, including for joins.
// Untracked and remote receipts retain their exclusion for their owning recovery.
func (s *RuntimeService) Cancel(ctx context.Context, ref RuntimeRunReference, actor usertypes.Actor) (activitytypes.RuntimeSnapshot, error) {
	if err := s.checkInternal(); err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	if !runtimeTextInternal(actor.ID, 256) {
		return activitytypes.RuntimeSnapshot{}, errRuntimeInvalidInternal
	}
	runID := ref.RunID
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	lock := s.activity.publishLockInternal(runID)
	lock.Lock()
	defer lock.Unlock()
	var snapshot activitytypes.RuntimeSnapshot
	var message *ActivityMessage
	var row *RuntimeReceipt
	err := s.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		var err error
		row, err = runtimeReferenceRowTxInternal(tx, ref)
		if err != nil {
			return err
		}
		if row.SettledAt == nil && !row.CancelRequested {
			row, err = runtimeCancelIntentTxInternal(tx, now, runtimeRowIdentityInternal(row))
			if err != nil {
				return err
			}
			message = &ActivityMessage{BaseModel: database.BaseModel{ID: uuid.NewString(), CreatedAt: now, UpdatedAt: new(now)},
				ActivityID: runID, Level: activitytypes.MessageLevelWarning, Code: string(activitytypes.CodeRunCancelling), Message: "Runtime cancellation requested",
				Payload: database.JSON{"actorId": actor.ID, "operation": *row.OperationID}}
			if err := tx.Create(message).Error; err != nil {
				return err
			}
		}
		snapshot, err = runtimeSnapshotTxInternal(tx, row)
		return err
	})
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	// COMMIT is the cancellation linearization point, never the caller's timer.
	if row.SettledAt == nil && row.CancelRequested {
		if owner := s.owners[runID]; owner != nil {
			owner.cancelRequested = true
			owner.cancel(activitylib.ErrCanceled)
		}
	}
	if message != nil {
		s.activity.publishMessageInternal(snapshot.EnvironmentID, snapshot.Type, activityMessageToDTOInternal(message))
	}
	return snapshot, nil
}
