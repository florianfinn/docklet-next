// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-14
// License: BSD-3-Clause
// Deviations: Immediate single-hub startup repair and ungated periodic sweep.

package activity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/florianfinn/docklet-next/types/activity"
)

func (s *ActivityService) isTrackedInternal(activityID string) bool {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	_, ok := s.running[activityID]
	return ok
}

// RepairStartup closes abandoned work after the hub session lock is acquired.
// Coordinator-owned activities are excluded until their own reconciliation ends.
func (s *ActivityService) RepairStartup(ctx context.Context, protectedIDs ...string) (int64, error) {
	return s.repairMatchingInternal(ctx, "", "", false, protectedIDs...)
}

// FailAbandonedActivities sweeps only old, untracked ungated work.
func (s *ActivityService) FailAbandonedActivities(ctx context.Context) (int64, error) {
	return s.repairMatchingInternal(ctx, "", "", true)
}

func (s *ActivityService) FailStaleImageUpdateChecks(ctx context.Context) (int64, error) {
	return s.repairMatchingInternal(ctx, activity.TypeImageUpdateCheck, activity.StatusRunning, false)
}

func (s *ActivityService) ResolveOrphanedQueuedActivities(ctx context.Context, protectedIDs ...string) (int64, error) {
	return s.repairMatchingInternal(ctx, "", activity.StatusQueued, false, protectedIDs...)
}

func (s *ActivityService) ResolveStaleAutoUpdateActivities(ctx context.Context, protectedIDs ...string) (int64, error) {
	return s.repairMatchingInternal(ctx, activity.TypeAutoUpdate, activity.StatusRunning, false, protectedIDs...)
}

func (s *ActivityService) repairMatchingInternal(ctx context.Context, kind activity.Type, status activity.Status, sweep bool, protectedIDs ...string) (int64, error) {
	if err := s.checkInitInternal(); err != nil {
		return 0, err
	}
	query := s.db.WithContext(ctx).Where("status IN ? AND type <> ?", []activity.Status{activity.StatusQueued, activity.StatusRunning}, activity.TypeJobRun)
	if kind != "" {
		query = query.Where("type = ?", kind)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if sweep {
		query = query.Where("COALESCE(gate_scope, '') = '' AND started_at < NOW() - INTERVAL '2 minutes'")
	}
	if kind == activity.TypeImageUpdateCheck && status == activity.StatusRunning && !sweep {
		query = query.Where("started_at < ?", time.Now().Add(-staleImageUpdateCheckAge))
	}
	if len(protectedIDs) != 0 {
		query = query.Where("id NOT IN ?", protectedIDs)
	}
	var rows []Activity
	if err := query.Find(&rows).Error; err != nil {
		return 0, fmt.Errorf("find abandoned activities: %w", err)
	}
	var repaired int64
	var repairErrors []error
	for _, row := range rows {
		if sweep && s.isTrackedInternal(row.ID) {
			continue
		}
		result := activity.StatusFailed
		message := "Activity interrupted by hub restart"
		var failure *string = &message
		if sweep {
			message = "Activity was marked failed because its worker is no longer running"
			failure = &message
		}
		if kind == activity.TypeImageUpdateCheck && status == activity.StatusRunning && !sweep {
			message = "Image update check failed because it was stale after hub restart"
			failure = &message
		}
		if row.Type == activity.TypeAutoUpdate {
			if triggered, _ := row.Metadata["selfUpdateTriggered"].(bool); triggered {
				result = activity.StatusSuccess
				message = "Auto-update completed: hub restarted with the updated image"
				failure = nil
			}
		}
		repairCtx := context.WithValue(ctx, startupRepairContextKey{}, true)
		finished, err := s.CompleteActivity(repairCtx, row.ID, result, message, failure)
		if err != nil {
			repairErrors = append(repairErrors, fmt.Errorf("repair activity %s: %w", row.ID, err))
			continue
		}
		if finished.Status == result && finished.EndedAt != nil {
			repaired++
		}
	}
	return repaired, errors.Join(repairErrors...)
}

const staleImageUpdateCheckAge = 6 * time.Hour
