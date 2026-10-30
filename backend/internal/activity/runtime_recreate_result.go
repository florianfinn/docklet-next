// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original compact replacement identity persistence and conditional hub projection.

package activity

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/container"
	"gorm.io/gorm"
)

func runtimeRecreateResultTx(tx *gorm.DB, row *RuntimeReceipt, result *dto.RecreateResult) error {
	if result == nil {
		return nil
	}
	if row.OperationID == nil || !runtimeRecreateOperation(RuntimeOperation(*row.OperationID)) || row.TargetID == nil || result.OldID != *row.TargetID || result.NewID == "" || result.OldName == "" || result.NewName == "" || result.NewID == result.OldID || (row.TargetName != nil && result.OldName != *row.TargetName) {
		return activityDTO.ErrRuntimeWire
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if err := tx.Model(&RuntimeReceipt{}).Where("run_id=?", row.RunID).UpdateColumn("result", string(body)).Error; err != nil {
		return err
	}
	row.Result = new(string(body))
	return origin.Move(tx, row.RegistrationID, result)
}
func runtimeRecreateResult(row *RuntimeReceipt) (*dto.RecreateResult, error) {
	if row.Result == nil {
		return nil, nil
	}
	var result dto.RecreateResult
	if err := json.Unmarshal([]byte(*row.Result), &result); err != nil {
		return nil, activityDTO.ErrRuntimeWire
	}
	return &result, nil
}

// Recreate recovery commits before generic interruption can release an exclusion.
func (s *RuntimeService) recoverRecreatesStartup(ctx context.Context) error {
	adapter, ok := s.localExecution.Executor.(*RuntimeServiceAdapter)
	if !ok {
		return nil
	}
	port, ok := adapter.containers.(runtimeContainerRecreatePort)
	if !ok {
		return nil
	}
	results, err := port.RecoverRecreates(ctx)
	if err != nil {
		return err
	}
	for _, recovered := range results {
		ctx := context.WithValue(ctx, gateRunContextKey{}, recovered.RunID)
		err := s.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
			var row RuntimeReceipt
			if err := tx.First(&row, "run_id=?", recovered.RunID).Error; err != nil {
				return err
			}
			if row.SettledAt != nil {
				return nil
			}
			state, code := "failed", activityDTO.Code(recovered.Code)
			if recovered.Result != nil {
				state, code = "success", activityDTO.CodeActivitySuccess
			}
			// The journal's committed observations are the effect evidence. Keep the
			// ordinary terminal writer and its original receipt identity/actor.
			result := runtimeLocalResultInternal{recovered: true, state: state, code: code, message: string(code), settledAt: now, startedAt: row.ExecutorStartedAt, recreate: recovered.Result}
			if state == "failed" {
				result.errorText = new(string(code))
			}
			_, _, err := runtimeCompleteTxInternal(tx, now, runtimeRowIdentityInternal(&row), result)
			return err
		})
		if err != nil {
			return err
		}
	}
	return port.PruneRecreateJournals(ctx)
}

// Clearing history cannot remove the compact authority required to project a
// late terminal result. Rebuild only its minimal receipt-owned activity row.
func runtimeEnsureActivityTx(tx *gorm.DB, row *RuntimeReceipt, model *Activity) error {
	err := tx.First(model, "id=? AND environment_id=?", row.RunID, row.EnvironmentID).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	kind, _ := runtimeActivityTypeInternal(*row.OperationID, *row.TargetKind)
	*model = Activity{BaseModel: database.BaseModel{ID: row.RunID, CreatedAt: *row.AdmittedAt, UpdatedAt: new(row.FinalWriteAt)}, EnvironmentID: row.EnvironmentID, Type: kind, Status: activityDTO.StatusRunning, ResourceType: row.TargetKind, ResourceID: row.TargetID, ResourceName: row.TargetName, StartedByUserID: row.CallerID, StartedByUsername: row.CallerName, StartedByDisplayName: row.CallerName, StartedAt: *row.AdmittedAt, GateScope: *row.GateScope, GateKey: *row.GateKey, MessageCode: string(activityDTO.CodeRunStarted)}
	return tx.Create(model).Error
}
