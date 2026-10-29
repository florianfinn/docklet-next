// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original database-only restored-history settlement and authority regeneration.

package backup

import (
	"context"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"gorm.io/gorm"
)

// ResetRestoredHistory runs for online and offline hub replacement before
// activation. Archived stopped/created-volume evidence never authorizes effects
// on today's Docker host; the restored set counter keeps its original ordering.
func ResetRestoredHistory(ctx context.Context, db *database.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("volume_backups").Where("status = ?", "running").Updates(map[string]any{"status": "failed", "error_code": "volume.backup.interrupted", "error": "volume.backup.interrupted"}).Error; err != nil {
			return err
		}
		if err := tx.Table("volume_restore_runs").Where("status = ?", "running").Updates(map[string]any{"status": "failed", "error_code": gorm.Expr("CASE WHEN phase = 'writing' THEN 'volume.restore.interrupted' ELSE 'volume.restore.interrupted-before-write' END"), "finished_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		if err := tx.Table("volume_restore_runs").Where("pending_restart = ? OR cleanup_pending = ? OR input_path <> ?", true, true, "").Updates(map[string]any{"pending_restart": false, "cleanup_pending": false, "input_path": ""}).Error; err != nil {
			return err
		}
		return (&backuptarget.Store{DB: &database.DB{DB: tx}}).ResetAuthority(ctx)
	})
}
