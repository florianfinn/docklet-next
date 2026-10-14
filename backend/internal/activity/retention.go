// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; extracted retention logic.

package activity

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"gorm.io/gorm"
)

func (s *ActivityService) PruneHistory(ctx context.Context, retentionDays, maxEntries int) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if retentionDays < 0 {
		retentionDays = defaultActivityRetentionDays
	}
	if maxEntries < 0 {
		maxEntries = defaultActivityHistoryLimit
	}

	var deleted int64
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if retentionDays > 0 {
			cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
			ids, err := findTerminalActivityIDsInternal(tx.Where("COALESCE(ended_at, updated_at, created_at) < ?", cutoff))
			if err != nil {
				return fmt.Errorf("failed to find activities older than retention window: %w", err)
			}
			count, err := deleteActivitiesByIDInternal(tx, ids)
			if err != nil {
				return err
			}
			deleted += count
		}

		if maxEntries > 0 {
			ids, err := findActivityIDsBeyondHistoryLimitInternal(tx, maxEntries)
			if err != nil {
				return err
			}
			count, err := deleteActivitiesByIDInternal(tx, ids)
			if err != nil {
				return err
			}
			deleted += count
		}

		return nil
	}); err != nil {
		return 0, err
	}

	return deleted, nil
}

func (s *ActivityService) DeleteHistory(ctx context.Context, environmentID string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}

	environmentID = cmp.Or(strings.TrimSpace(environmentID), "0")

	var deleted int64
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids, err := findTerminalActivityIDsInternal(scopeJobActivityVisibilityInternal(ctx, tx.Where("environment_id = ?", environmentID), environmentID, authz.PermActivitiesDelete))
		if err != nil {
			return fmt.Errorf("failed to find activity history: %w", err)
		}
		count, err := deleteActivitiesByIDInternal(tx, ids)
		if err != nil {
			return err
		}
		deleted = count
		return nil
	}); err != nil {
		return 0, err
	}

	return deleted, nil
}

// PruneSyncReceipts removes only runs retired from the durable coordinator.
// The grace protects a receipt synchronized after the coordinator snapshot.
func (s *ActivityService) PruneSyncReceipts(ctx context.Context, retainedRunIDs []string) (int64, error) {
	cutoff := time.Now().UTC().Add(-abandonedActivityGrace)
	query := s.db.WithContext(ctx).Where("synced_at < ?", cutoff)
	if len(retainedRunIDs) > 0 {
		query = query.Where("run_id NOT IN ?", retainedRunIDs)
	}
	result := query.Delete(&ActivitySyncReceipt{})
	if result.Error != nil {
		return 0, fmt.Errorf("prune retired activity receipts: %w", result.Error)
	}
	return result.RowsAffected, nil
}
