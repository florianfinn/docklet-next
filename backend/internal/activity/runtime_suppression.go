// Source: docklet-next/issues/441
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original hub failed-candidate settlement integration.

package activity

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"gorm.io/gorm"
	"strings"
)

// A successful manual update clears every historical identity of this exact
// hub target. Agent stores and scheduled updates never clear suppression.
func runtimeClearFailedTargetTx(tx *gorm.DB, row *RuntimeReceipt) error {
	cfg := config.LoadRole()
	if cfg.AgentMode || cfg.EdgeAgent || row.OperationID == nil || row.TargetKind == nil || row.TargetID == nil {
		return nil
	}
	if *row.OperationID != "update-project" && *row.OperationID != "update-container" {
		return nil
	}
	observed, err := runtimeObservedRunTx(tx, row.RunID)
	if err != nil {
		return err
	}
	if observed != nil && observed.Mode == "scheduled" {
		return nil
	}
	target := *row.TargetID
	if *row.TargetKind == "container" {
		if row.TargetName == nil || *row.TargetName == "" {
			return errRuntimeInvalidInternal
		}
		target = strings.TrimPrefix(*row.TargetName, "/")
	}
	return policy.ClearFailedTargetTx(tx, row.EnvironmentID, row.RegistrationID, *row.TargetKind, target)
}
