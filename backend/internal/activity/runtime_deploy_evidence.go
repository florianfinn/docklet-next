// Source: docklet-next/issues/344
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original receipt settlement projection of durable deploy-point evidence.

package activity

import (
	"errors"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
)

func runtimeDeployOperation(operation string) bool {
	return operation == "deploy-project" || operation == "redeploy-project"
}

// All receipt transitions share this writer, including startup interruption.
// Remote hub receipts have no local lifecycle row and cannot invent one.
func runtimeDeployEvidenceTx(tx *gorm.DB, row *RuntimeReceipt) error {
	if row.OperationID == nil || !runtimeDeployOperation(*row.OperationID) {
		return nil
	}
	var run struct {
		PointID   string
		PointCode *string
	}
	err := tx.Table("image_lifecycle_runs").Where("id = ?", row.RunID).Take(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var model Activity
	if err = runtimeEnsureActivityTx(tx, row, &model); err != nil {
		return err
	}
	result := runtimeImageResult(&model)
	if result == nil {
		result = &dto.LifecycleResult{RunID: row.RunID}
	}
	result.RestorePointID, result.PointCode = run.PointID, ""
	if run.PointCode != nil {
		result.PointCode = *run.PointCode
	}
	checked := checkedLifecycleResult(*result)
	return runtimeImageResultTx(tx, row.RunID, &checked)
}
