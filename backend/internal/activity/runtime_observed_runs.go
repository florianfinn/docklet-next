// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original durable observed-run and failed-candidate recovery storage.

package activity

import (
	"encoding/json"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"reflect"
	"slices"
	"time"
)

func runtimeObservedExecution(command RuntimeCommand) (dto.ObservedExecution, bool) {
	var binding *dto.PolicyBinding
	var manual bool
	switch command.Operation {
	case RuntimeUpdateContainer:
		if command.Options.ContainerImage != nil {
			binding = command.Options.ContainerImage.Policy
			manual = command.Options.ContainerImage.AutoRestore
		}
	case RuntimeUpdateProject:
		if command.Options.Lifecycle != nil {
			binding = command.Options.Lifecycle.Policy
			manual = command.Options.Lifecycle.Request.AutoRestore
		}
	}
	if binding != nil {
		return dto.ObservedExecution{Mode: "scheduled", PolicyID: binding.PolicyID, ChildID: binding.ChildID, Suppressed: slices.Clone(binding.Suppressed)}, true
	}
	if manual {
		return dto.ObservedExecution{Mode: "manual"}, true
	}
	return dto.ObservedExecution{}, false
}
func runtimeObserveAdmissionTx(tx *gorm.DB, now time.Time, run string, command RuntimeCommand) error {
	observed, ok := runtimeObservedExecution(command)
	if !ok {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&RuntimeObservedRun{RunID: run, Kind: command.Target.Kind, Mode: observed.Mode, CreatedAt: now}).Error
}
func runtimeObservedRunTx(tx *gorm.DB, run string) (*RuntimeObservedRun, error) {
	var row RuntimeObservedRun
	found := tx.Where("run_id = ?", run).Limit(1).Find(&row)
	if found.Error != nil {
		return nil, found.Error
	}
	if found.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

// Receipt and marker are loaded in the terminal transaction, never from payload.
func runtimeCheckedTerminalTx(tx *gorm.DB, row *RuntimeReceipt, status, code string, result *dto.LifecycleResult) (*dto.LifecycleResult, error) {
	observed, err := runtimeObservedRunTx(tx, row.RunID)
	if err != nil {
		return nil, err
	}
	return checkedTerminalLifecycle(row.RunID, status, code, observed != nil, result), nil
}
func runtimeWriteObservedTx(tx *gorm.DB, now time.Time, row *RuntimeReceipt, result *dto.LifecycleResult) error {
	if result == nil {
		return tx.Model(&RuntimeObservedRun{}).Where("run_id = ?", row.RunID).UpdateColumn("settled_at", now).Error
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if len(body) > dto.MaxRecoveryEvidenceBytes {
		return errRuntimeInvalidInternal
	}
	outcome := ""
	if result.Recovery != nil {
		outcome = result.Recovery.Outcome
	}
	return tx.Model(&RuntimeObservedRun{}).Where("run_id = ?", row.RunID).UpdateColumns(map[string]any{"evidence": string(body), "outcome": outcome, "settled_at": now}).Error
}

func runtimeProjectObservedTx(tx *gorm.DB, row *RuntimeReceipt, item *activityDTO.Activity) error {
	observed, err := runtimeObservedRunTx(tx, row.RunID)
	if err != nil {
		return err
	}
	checkedActivityLifecycle(item)
	if observed == nil {
		return nil
	}
	if observed.Evidence != "" {
		var result dto.LifecycleResult
		if json.Unmarshal([]byte(observed.Evidence), &result) != nil || result.RunID != row.RunID || len(observed.Evidence) > dto.MaxRecoveryEvidenceBytes || recoveryOutcome(&result) != observed.Outcome {
			result = unknownLifecycle(row.RunID)
		}
		checked := checkedTerminalLifecycle(row.RunID, *row.ExecutorState, runtimeReceiptCode(row), true, &result)
		if item.ImageLifecycle == nil {
			item.ImageLifecycle = checked
		} else if !reflect.DeepEqual(item.ImageLifecycle, checked) {
			value := unknownLifecycle(row.RunID)
			item.ImageLifecycle = &value
		}
	}
	if row.ExecutorSettledAt != nil {
		item.ImageLifecycle = checkedTerminalLifecycle(row.RunID, *row.ExecutorState, runtimeReceiptCode(row), true, item.ImageLifecycle)
	}
	if item.ImageLifecycle != nil {
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		item.Metadata["imageLifecycle"] = *item.ImageLifecycle
	}
	return nil
}
func runtimeReceiptCode(row *RuntimeReceipt) string {
	if row.ExecutorResultCode != nil {
		return *row.ExecutorResultCode
	}
	return ""
}
func recoveryOutcome(result *dto.LifecycleResult) string {
	if result != nil && result.Recovery != nil {
		return result.Recovery.Outcome
	}
	return ""
}
