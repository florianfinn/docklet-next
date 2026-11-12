// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation; M21 container image update failures; M24 recovery notices by trusted admission mode.

package activity

import (
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
	"time"
)

// Stack deployment and container image update notices share the terminal receipt
// transaction and contain only coded facts.
// Scheduled interruptions notify at their authoritative end; manual startup stays silent.
func runtimeNotifyFailureTx(tx *gorm.DB, now time.Time, row *RuntimeReceipt, code string, silent, disposition bool, checked *dto.LifecycleResult) error {
	cfg := config.LoadRole()
	if cfg.AgentMode || cfg.EdgeAgent || row.OperationID == nil || row.TargetKind == nil {
		return nil
	}
	observed, err := runtimeObservedRunTx(tx, row.RunID)
	if err != nil {
		return err
	}
	scheduled := observed != nil && observed.Mode == "scheduled"
	if silent && !scheduled {
		return nil
	}
	reason := code
	if scheduled {
		if checked != nil && checked.Code == "update.candidate-failed" {
			return nil
		}
		switch recoveryOutcome(checked) {
		case "restored":
			reason = "auto-restored"
		case "restore-failed":
			reason = "auto-restore-failed"
		case "unavailable":
			reason = "auto-restore-unavailable"
		case "interrupted":
			reason = "update-interrupted"
		}
		if code == "runtime.interrupted" {
			reason = "update-interrupted"
		}
	}
	switch *row.TargetKind + ":" + *row.OperationID {
	case "stack:deploy-project", "stack:redeploy-project", "stack:update-project", "stack:restore-project",
		"container:update-container", "container:restore-container-image":
	default:
		return nil
	}
	if code != "runtime.failed" && (code != "runtime.interrupted" || !disposition && !scheduled) && !(scheduled && reason == "update-interrupted") {
		return nil
	}
	host := "Local Docker"
	if row.EnvironmentID != "0" {
		var env struct{ Name string }
		if err := tx.Table("environments").Select("name").Where("id=?", row.EnvironmentID).Take(&env).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		host = "Remote host"
		if env.Name != "" {
			host = env.Name
		}
	}
	name := ""
	if row.TargetName != nil {
		name = *row.TargetName
	}
	payload := database.JSON{"host": host, "stack": name, "operation": *row.OperationID, "reason": reason}
	if *row.TargetKind == "container" {
		payload = database.JSON{"host": host, "container": name, "operation": *row.OperationID, "reason": reason}
	}
	if scheduled {
		payload["scheduled"] = true
		if outcome := recoveryOutcome(checked); outcome != "" {
			payload["recovery"] = outcome
		}
		if checked != nil && checked.Recovery != nil && checked.Recovery.Code != "" {
			payload["recoveryReason"] = checked.Recovery.Code
		}
	}
	return notificationoutbox.Insert(tx, notificationoutbox.Row{Event: "deploy_failed", EnvironmentID: row.EnvironmentID, DedupKey: "deploy:" + row.EnvironmentID + ":" + row.RunID, Payload: payload, CreatedAt: now})
}
