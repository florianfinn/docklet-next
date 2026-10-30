// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package activity

import (
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"gorm.io/gorm"
	"time"
)

// Deployment notices share the terminal receipt transaction and contain only coded facts.
// A shutdown or restart interruption stays silent (OQ-1 b); an administrator disposition notifies.
func runtimeNotifyFailureTx(tx *gorm.DB, now time.Time, row *RuntimeReceipt, code string, silent, disposition bool) error {
	cfg := config.Load()
	if silent || cfg.AgentMode || cfg.EdgeAgent || row.OperationID == nil || row.TargetKind == nil || *row.TargetKind != "stack" {
		return nil
	}
	switch *row.OperationID {
	case "deploy-project", "redeploy-project", "update-project", "restore-project":
	default:
		return nil
	}
	if code != "runtime.failed" && (code != "runtime.interrupted" || !disposition) {
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
	stack := ""
	if row.TargetName != nil {
		stack = *row.TargetName
	}
	return notificationoutbox.Insert(tx, notificationoutbox.Row{Event: "deploy_failed", EnvironmentID: row.EnvironmentID, DedupKey: "deploy:" + row.EnvironmentID + ":" + row.RunID, Payload: database.JSON{"host": host, "stack": stack, "operation": *row.OperationID, "reason": code}, CreatedAt: now})
}
