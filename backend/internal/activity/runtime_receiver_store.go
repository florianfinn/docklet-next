// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Keyless executor acceptance.

package activity

import (
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The caller has already checked the run ID in this same immediate transaction.
// No key binding or different-ID join participates in executor acceptance.
func runtimeReceiveTxInternal(tx *gorm.DB, now time.Time, c runtimeCanonicalCommandInternal) (*RuntimeReceipt, error) {
	id := c.identity(c.command.ProposedRunID)
	if err := runtimeCheckStoreTxInternal(tx, id); err != nil {
		return nil, err
	}
	conflict, err := runtimeActiveConflictTxInternal(tx, c)
	if err != nil {
		return nil, err
	}
	if conflict != nil {
		return nil, &GateConflict{RunID: conflict.RunID, Code: string(activitytypes.CodeRunBusy)}
	}
	target, actor := c.command.Target, c.command.Actor
	row := &RuntimeReceipt{RunID: id.RunID, Kind: "receipt", StoreGeneration: id.StoreGeneration,
		ManagerID: id.ManagerID, RegistrationID: id.RegistrationID, EnvironmentID: id.EnvironmentID,
		ExecutorGeneration: id.ExecutorGeneration, RequestDigest: id.RequestDigest,
		CallerID: new(actor.ID), CallerName: new(actor.Username), OperationID: new(string(c.command.Operation)),
		TargetKind: new(target.Kind), TargetID: new(target.ID), TargetName: target.Name, ParentProjectID: target.ParentProjectID,
		GateScope: new(c.gateScope), GateKey: new(c.gateKey), AdmittedAt: new(now),
		DispatchState: new("pending"), ExecutorState: new("pending"), FinalWriteAt: now}
	if err := runtimeReceiverCreateTxInternal(tx, row); err != nil {
		return nil, runtimeWriteErrorInternal(err)
	}
	if err := runtimeSaveImageIntent(tx, row.RunID, c.command); err != nil {
		return nil, err
	}
	kind, _ := runtimeActivityTypeInternal(string(c.command.Operation), target.Kind)
	display := actor.Username
	if actor.DisplayName != nil {
		display = *actor.DisplayName
	}
	model := &Activity{BaseModel: database.BaseModel{ID: id.RunID, CreatedAt: now, UpdatedAt: new(now)},
		EnvironmentID: "0", Type: kind, Status: activitytypes.StatusRunning,
		ResourceType: row.TargetKind, ResourceID: row.TargetID, ResourceName: row.TargetName,
		StartedByUserID: row.CallerID, StartedByUsername: row.CallerName, StartedByDisplayName: new(display),
		StartedAt: now, GateScope: c.gateScope, GateKey: c.gateKey, MessageCode: string(activitytypes.CodeRunStarted)}
	if err := runtimeReceiverCreateTxInternal(tx, model); err != nil {
		return nil, runtimeWriteErrorInternal(err)
	}
	if runtimeQueuesInternal(c.command.Operation) {
		changed := tx.Model(&Activity{}).Where("id = ? AND environment_id = ?", id.RunID, "0").
			UpdateColumns(map[string]any{"status": activitytypes.StatusQueued, "updated_at": now})
		if changed.Error != nil {
			return nil, changed.Error
		}
		if changed.RowsAffected != 1 {
			return nil, &runtimeConflictInternal{}
		}
	}
	// A keyless start audit must not present an empty key as client evidence.
	if err := runtimeReceiverCreateTxInternal(tx, &ActivityMessage{BaseModel: database.BaseModel{ID: uuid.NewString(), CreatedAt: now, UpdatedAt: new(now)},
		ActivityID: id.RunID, Level: activitytypes.MessageLevelInfo, Code: string(activitytypes.CodeRunStarted),
		Message: string(activitytypes.CodeRunStarted), Payload: database.JSON{"actorId": actor.ID, "operation": string(c.command.Operation)}}); err != nil {
		return nil, err
	}
	// Reuse the accepted writer, including its idempotent worker phase contract.
	accepted, err := runtimeLocalAcceptTxInternal(tx, now, id)
	if err != nil {
		return nil, err
	}
	if accepted.ExecutorState == nil || *accepted.ExecutorState != "accepted" {
		return nil, &runtimeConflictInternal{}
	}
	return accepted, nil
}

func runtimeReceiverCreateTxInternal(tx *gorm.DB, model any) error {
	changed := tx.Create(model)
	if changed.Error != nil {
		return runtimeWriteErrorInternal(changed.Error)
	}
	if changed.RowsAffected != 1 {
		return &runtimeConflictInternal{}
	}
	return nil
}
