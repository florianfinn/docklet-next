// Source: docklet-next/issues/366
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original executing-environment replacement exclusion.

package activity

import (
	"errors"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

// A recreate label survives settlement. Only an unsettled receipt on this
// executing environment holds its replacement; internal steps keep their run ID.
func replacementHoldTxInternal(tx *gorm.DB, environment, replacementRunID, admittedRunID string) error {
	if replacementRunID == "" || replacementRunID == admittedRunID {
		return nil
	}
	var row RuntimeReceipt
	err := tx.Select("run_id").Where("kind='receipt' AND run_id=? AND environment_id=? AND settled_at IS NULL", replacementRunID, environment).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return &GateConflict{RunID: row.RunID, Code: string(activitytypes.CodeRunBusy)}
}
