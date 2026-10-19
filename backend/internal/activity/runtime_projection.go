// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Receipt-owned internal projection and compact fallback; no wire registration.

package activity

import (
	"errors"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

func runtimeKnowledgeInternal(row *RuntimeReceipt) activitytypes.RuntimeKnowledge {
	outcome := activitytypes.RuntimeOutcomeUnknown
	switch *row.ExecutorState {
	case "accepted", "executing", "success", "failed", "cancelled":
		outcome = activitytypes.RuntimeOutcomeKnown
	case "pending":
		if row.HubDisposition != nil && *row.HubDisposition != "released" {
			outcome = activitytypes.RuntimeOutcomeKnown
		}
	}
	return activitytypes.RuntimeKnowledge{OutcomeState: outcome, CancelRequested: row.CancelRequested}
}

func runtimeStatusInternal(row *RuntimeReceipt) activitytypes.Status {
	if row.HubDisposition != nil {
		if *row.HubDisposition == "cancelled" {
			return activitytypes.StatusCancelled
		}
		return activitytypes.StatusFailed
	}
	switch *row.ExecutorState {
	case "success":
		return activitytypes.StatusSuccess
	case "failed":
		return activitytypes.StatusFailed
	case "cancelled":
		return activitytypes.StatusCancelled
	default:
		return activitytypes.StatusRunning
	}
}

// The caller has checked binding identity and current read permission before
// entering this projection. Missing output history does not erase a receipt.
func runtimeSnapshotTxInternal(tx *gorm.DB, row *RuntimeReceipt) (activitytypes.RuntimeSnapshot, error) {
	var model Activity
	err := tx.First(&model, "id = ? AND environment_id = ?", row.RunID, row.EnvironmentID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return activitytypes.RuntimeSnapshot{}, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		kind, _ := runtimeActivityTypeInternal(*row.OperationID, *row.TargetKind)
		model = Activity{BaseModel: database.BaseModel{ID: row.RunID, CreatedAt: *row.AdmittedAt, UpdatedAt: new(row.FinalWriteAt)},
			EnvironmentID: row.EnvironmentID, Type: kind, ResourceType: row.TargetKind, ResourceID: row.TargetID, ResourceName: row.TargetName,
			StartedByUserID: row.CallerID, StartedByUsername: row.CallerName, StartedByDisplayName: row.CallerName,
			StartedAt: *row.AdmittedAt, EndedAt: row.SettledAt, Error: row.ExecutorError}
	}
	// Knowledge and workflow disposition come from the receipt, independently of
	// legacy status/read health. A manual release retains unknown external outcome.
	status := runtimeStatusInternal(row)
	if status == activitytypes.StatusRunning && model.Status == activitytypes.StatusQueued &&
		row.EnvironmentID == "0" && row.ExecutorGeneration == row.StoreGeneration && *row.DispatchState == "pending" &&
		(*row.ExecutorState == "pending" || *row.ExecutorState == "accepted") && runtimeQueuesInternal(RuntimeOperation(*row.OperationID)) {
		status = activitytypes.StatusQueued
	}
	model.Status = status
	if row.HubDisposition != nil {
		model.MessageCode = string(activitytypes.CodeRuntimeInterrupted)
		model.Error = row.HubDispositionReason
	} else if row.ExecutorResultCode != nil {
		model.MessageCode = *row.ExecutorResultCode
	}
	return activitytypes.RuntimeSnapshot{Activity: activityToDTOInternal(&model), RuntimeKnowledge: runtimeKnowledgeInternal(row)}, nil
}
