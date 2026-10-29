// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Atomic local evidence, conditional Activity/final audit and retained retry.

package activity

import (
	"context"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Frozen only after the executor and capture have actually joined. Settlement
// retries reuse these executor times and codes; they never rerun work or drain.
type runtimeLocalResultInternal struct {
	silentNotification bool
	imageLifecycle     *updaterDTO.LifecycleResult
	outputLost         bool
	state              string
	code               activitytypes.Code
	message            string
	errorText          *string
	startedAt          *time.Time
	settledAt          time.Time
}

// Caller holds runningMu. Only accepted owners use this detached write budget.
// Context, constraint, FULL, I/O and authorization failures are not retried.
func (s *RuntimeService) ownerTransactionInternal(values context.Context, fn func(*gorm.DB, time.Time) error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(values), ownerWriteTimeout)
	defer cancel()
	for attempt := 1; ; attempt++ {
		err := s.transactionInternal(ctx, fn)
		if err == nil || attempt == 3 || !dbutil.IsRetryableWriteError(err) || ctx.Err() != nil {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt) * 250 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
}

func runtimeCompleteTxInternal(tx *gorm.DB, now time.Time, id runtimeIdentityInternal, result runtimeLocalResultInternal) (activitytypes.RuntimeSnapshot, *ActivityMessage, error) {
	empty := activitytypes.RuntimeSnapshot{}
	row, err := runtimeMutableReceiptTxInternal(tx, id)
	if err != nil {
		return empty, nil, err
	}
	if row.ExecutorGeneration != row.StoreGeneration || *row.DispatchState != "pending" || row.HubDisposition != nil {
		return empty, nil, &runtimeConflictInternal{}
	}
	switch *row.ExecutorState {
	case "pending", "accepted", "executing", "success", "failed", "cancelled":
	default:
		return empty, nil, &runtimeConflictInternal{}
	}
	valid := (result.state == "success" && result.code == activitytypes.CodeActivitySuccess) ||
		(result.state == "cancelled" && result.code == activitytypes.CodeRuntimeCancelled) ||
		(result.state == "failed" && (result.code == activitytypes.CodeRuntimeFailed || result.code == activitytypes.CodeRuntimeInterrupted))
	if !valid || result.settledAt.IsZero() || result.message == "" {
		return empty, nil, errRuntimeInvalidInternal
	}
	wasTerminal := row.ExecutorSettledAt != nil
	if result.state == "success" && !wasTerminal && *row.ExecutorState != "executing" {
		return empty, nil, &runtimeConflictInternal{}
	}
	row, err = runtimeEvidenceTxInternal(tx, now, id, runtimeEvidenceInternal{State: result.state, ResultCode: new(string(result.code)),
		Error: result.errorText, StartedAt: result.startedAt, SettledAt: new(result.settledAt)})
	if err != nil {
		return empty, nil, err
	}
	if result.imageLifecycle != nil {
		if err := runtimeImageResultTx(tx, row.RunID, result.imageLifecycle); err != nil {
			return empty, nil, err
		}
	}
	if wasTerminal {
		snapshot, err := runtimeSnapshotTxInternal(tx, row)
		return snapshot, nil, err
	}
	var model Activity
	if err := tx.First(&model, "id = ? AND environment_id = ?", id.RunID, id.EnvironmentID).Error; err != nil {
		return empty, nil, err
	}
	if !gatedWriterAllowedInternal(tx.Statement.Context, &model) {
		return empty, nil, ErrRunNotOwned
	}
	status := runtimeStatusInternal(row)
	updates := completeActivityUpdatesInternal(model.StartedAt, status, result.message, result.errorText, nil, result.settledAt)
	updates["message_code"] = string(result.code)
	updates["updated_at"] = now
	updates["error"] = result.errorText
	updates["output_truncated"] = model.OutputTruncated || result.outputLost
	changed := tx.Model(&Activity{}).Where("id = ? AND environment_id = ? AND status IN ?", id.RunID, id.EnvironmentID,
		[]activitytypes.Status{activitytypes.StatusQueued, activitytypes.StatusRunning}).UpdateColumns(updates)
	if changed.Error != nil {
		return empty, nil, changed.Error
	}
	if changed.RowsAffected != 1 {
		return empty, nil, &runtimeConflictInternal{}
	}
	level := activitytypes.MessageLevelSuccess
	if status == activitytypes.StatusFailed {
		level = activitytypes.MessageLevelError
	}
	if status == activitytypes.StatusCancelled {
		level = activitytypes.MessageLevelWarning
	}
	final := &ActivityMessage{BaseModel: database.BaseModel{ID: uuid.NewString(), CreatedAt: now, UpdatedAt: new(now)},
		ActivityID: id.RunID, Level: level, Code: string(result.code), Message: result.message,
		Payload: database.JSON{"actorId": *row.CallerID, "operation": *row.OperationID, "effectVerified": false}}
	if err := tx.Create(final).Error; err != nil {
		return empty, nil, err
	}
	if err := runtimeNotifyFailureTx(tx, now, row, string(result.code), result.silentNotification); err != nil {
		return empty, nil, err
	}
	snapshot, err := runtimeSnapshotTxInternal(tx, row)
	if err != nil {
		return empty, nil, err
	}
	return snapshot, final, nil
}

// Caller holds runningMu. Keep the existing per-activity commit/publish order.
// Only a successful terminal COMMIT permits ownership and slot release.
func (s *RuntimeService) settleOwnerInternal(owner *runtimeOwnerInternal) (activitytypes.RuntimeSnapshot, error) {
	if owner.result == nil {
		return activitytypes.RuntimeSnapshot{}, ErrRuntimeNotSettled
	}
	lock := s.activity.publishLockInternal(owner.id.RunID)
	lock.Lock()
	defer lock.Unlock()
	var snapshot activitytypes.RuntimeSnapshot
	var final *ActivityMessage
	err := s.ownerTransactionInternal(owner.ctx, func(tx *gorm.DB, now time.Time) error {
		var err error
		snapshot, final, err = runtimeCompleteTxInternal(tx, now, owner.id, *owner.result)
		return err
	})
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	if final != nil {
		s.activity.publishMessageInternal(snapshot.EnvironmentID, snapshot.Type, activityMessageToDTOInternal(final))
		s.activity.publishActivityInternal(snapshot.Activity)
	}
	delete(s.owners, owner.id.RunID)
	owner.cancel(nil)
	owner.deadlineCancel()
	s.activity.releaseSlotInternal(owner.id.RunID)
	return snapshot, nil
}

// RetrySettlement requires current read/operation grants for this run. It retries
// only a joined local result; startup recovery owns lost in-memory outcomes.
func (s *RuntimeService) RetrySettlement(ctx context.Context, ref RuntimeRunReference) (activitytypes.RuntimeSnapshot, error) {
	if err := s.checkInternal(); err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	snapshot, err := s.boundSnapshotInternal(ctx, ref)
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	if isTerminalActivityStatusInternal(snapshot.Status) {
		return snapshot, nil
	}
	owner := s.owners[ref.RunID]
	if owner == nil || owner.result == nil {
		return activitytypes.RuntimeSnapshot{}, ErrRuntimeNotSettled
	}
	snapshot, owner.err = s.settleOwnerInternal(owner)
	return snapshot, owner.err
}
