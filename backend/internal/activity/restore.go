// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original activity continuity after database replacement for issue #32.

package activity

import (
	"context"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

// RestoreGatedActivity repairs interrupted archive-era gate holders and
// recreates the operation ID returned to the restore caller.
func (s *ActivityService) RestoreGatedActivity(ctx context.Context, run activitytypes.Activity, backupID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := failInterruptedForRestoreInternal(tx); err != nil {
			return err
		}
		model := Activity{
			BaseModel:     database.BaseModel{ID: run.ID, CreatedAt: run.CreatedAt},
			EnvironmentID: "0", Type: activitytypes.TypeResourceAction,
			Status: activitytypes.StatusRunning, ResourceType: new("system_backup"),
			ResourceID: new(backupID), StartedAt: run.StartedAt,
			MessageCode: "run.started", GateScope: "system-backup", GateKey: "restore:" + backupID,
			Metadata: database.JSON{}, AffectedServices: database.JSON{},
		}
		if model.CreatedAt.IsZero() {
			model.CreatedAt = time.Now().UTC()
		}
		if run.StartedBy != nil {
			if run.StartedBy.UserID != "" {
				model.StartedByUserID = new(run.StartedBy.UserID)
			}
			model.StartedByUsername = new(run.StartedBy.Username)
		}
		return tx.Create(&model).Error
	})
}

func (s *ActivityService) FailInterruptedAfterRestore(ctx context.Context) error {
	return failInterruptedForRestoreInternal(s.db.WithContext(ctx))
}

func failInterruptedForRestoreInternal(tx *gorm.DB) error {
	now := time.Now().UTC()
	// Receipt identity owns even settled rows with stale Activity state or copied scopes.
	return tx.Model(&Activity{}).
		Where("status IN ?", []activitytypes.Status{activitytypes.StatusRunning, activitytypes.StatusQueued}).
		Where("NOT EXISTS (SELECT 1 FROM runtime_receipts AS owned WHERE owned.kind = 'receipt' AND owned.environment_id = activities.environment_id AND owned.run_id = activities.id)").
		Updates(map[string]any{
			"status": activitytypes.StatusFailed, "message_code": "activity.failed",
			"error": "backup.interrupted", "ended_at": now,
			"duration_ms": gorm.Expr("max(0,(? - started_at)/1000)", now.UnixMicro()),
			"updated_at":  now,
		}).Error
}
