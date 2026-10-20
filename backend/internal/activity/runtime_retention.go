// Source: docklet-next/docs/design/runtime-control.md
// Revision: c641dc24b3f364ac5e4ec03bc9c5dfe2a10d5f61a6bcba825785f9dc351a9e50
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded compact-evidence cleanup, independent of full history.

package activity

import (
	"context"
	"time"

	"gorm.io/gorm"
)

const runtimeRetentionInternal = 24 * time.Hour
const runtimePruneCapInternal = 100

// The caller owns the transaction and its one store-local clock sample. No
// executor timestamp, admission age or key first-seen time controls retention.
func runtimePruneTxInternal(tx *gorm.DB, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	if limit > runtimePruneCapInternal {
		limit = runtimePruneCapInternal
	}
	var ids []string
	if err := tx.Model(&RuntimeReceipt{}).
		Where("settled_at IS NOT NULL AND final_write_at <= ?", runtimeTimeInternal(now).Add(-runtimeRetentionInternal)).
		Order("final_write_at ASC, run_id ASC").Limit(limit).Pluck("run_id", &ids).Error; err != nil {
		return 0, err
	}
	var removedIDs []string
	if err := tx.Model(&RuntimeRemovedTarget{}).Where("removed_at <= ? AND NOT EXISTS (SELECT 1 FROM runtime_receipts WHERE runtime_receipts.registration_id = runtime_removed_targets.registration_id)", runtimeTimeInternal(now).Add(-runtimeRetentionInternal)).Order("removed_at ASC, registration_id ASC").Limit(limit).Pluck("registration_id", &removedIDs).Error; err != nil {
		return 0, err
	}
	if len(removedIDs) > 0 {
		if err := tx.Where("registration_id IN ?", removedIDs).Delete(&RuntimeRemovedTarget{}).Error; err != nil {
			return 0, err
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if err := tx.Where("run_id IN ?", ids).Delete(&RuntimeKeyBinding{}).Error; err != nil {
		return 0, err
	}
	result := tx.Where("run_id IN ?", ids).Delete(&RuntimeReceipt{})
	return result.RowsAffected, result.Error
}

// Startup/hourly scheduling remains F-d. Keep the mutex-before-transaction order
// used by admission; zero is returned on any rollback, including commit failure.
func (s *ActivityService) pruneRuntimeInternal(ctx context.Context, limit int) (int64, error) {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	var count int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		count, err = runtimePruneTxInternal(tx, s.nowInternal(), limit)
		return err
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}
