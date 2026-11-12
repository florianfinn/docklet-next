// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Receipt-owned runtime projection and compact fallback shared by current exports.

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
		if (row.HubDisposition != nil && *row.HubDisposition != "released") ||
			(row.EnvironmentID == "0" && row.ExecutorGeneration == row.StoreGeneration &&
				row.UnknownReason == nil) {
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
	runtimeProjectModelInternal(&model, row)
	knowledge := runtimeKnowledgeInternal(row)
	dto := activityToDTOInternal(&model)
	dto.RuntimeKnowledge = &knowledge
	result, err := runtimeRecreateResult(row)
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	return activitytypes.RuntimeSnapshot{Activity: dto, RuntimeKnowledge: knowledge, Result: result}, nil
}
