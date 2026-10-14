// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-14
// License: BSD-3-Clause
// Deviations: Extracted lease-aware sweep and startup repairs.

package activity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const staleImageUpdateCheckAge = 6 * time.Hour

// FailStaleImageUpdateChecks marks image update checks that were left running
// across a prior process lifetime as failed. It intentionally scopes cleanup to
// old image-update-check activities so startup repair cannot affect other work.
func (s *ActivityService) FailStaleImageUpdateChecks(ctx context.Context) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	failed, repairErr := s.failAbandonedActivitiesInternal(ctx, activity.TypeImageUpdateCheck, activity.StatusRunning)
	if repairErr != nil {
		return failed, repairErr
	}

	cutoff := time.Now().Add(-staleImageUpdateCheckAge)
	var staleChecks []Activity
	if err := s.db.WithContext(ctx).
		Where("type = ? AND status = ? AND started_at < ?", activity.TypeImageUpdateCheck, activity.StatusRunning, cutoff).
		Where("COALESCE(gate_scope, '') = ''").
		Find(&staleChecks).Error; err != nil {
		return 0, fmt.Errorf("find stale image update checks: %w", err)
	}

	const message = "Image update check failed because it was stale after Arcane restarted"
	errMessage := message
	var failErrs []error
	for i := range staleChecks {
		if _, err := s.CompleteActivity(ctx, staleChecks[i].ID, activity.StatusFailed, message, &errMessage, "Image update check failed"); err != nil {
			failErrs = append(failErrs, fmt.Errorf("fail stale image update check %s: %w", staleChecks[i].ID, err))
			continue
		}
		failed++
	}

	return failed, errors.Join(failErrs...)
}

// isTrackedInternal reports whether activityID has a live work registration in
// this process (i.e. a worker goroutine called Track and has not completed yet).
func (s *ActivityService) isTrackedInternal(activityID string) bool {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	_, ok := s.running[activityID]
	return ok
}

// FailAbandonedActivities fails untracked local work and gated work whose
// database heartbeat expired. The guarded terminal update fences a gated
// owner before another operation can enter the same scope.
func (s *ActivityService) FailAbandonedActivities(ctx context.Context) (int64, error) {
	return s.failAbandonedActivitiesInternal(ctx, "", "")
}

func (s *ActivityService) failAbandonedActivitiesInternal(ctx context.Context, gatedType activity.Type, gatedStatus activity.Status, protectedIDs ...string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}

	activeStatuses := []activity.Status{activity.StatusQueued, activity.StatusRunning}
	var candidates []Activity
	query := s.db.WithContext(ctx).
		Where("status IN ? AND started_at < NOW() - INTERVAL '2 minutes' AND (COALESCE(gate_scope, '') = '' OR gate_lease_at < NOW() - INTERVAL '2 minutes')", activeStatuses).
		Where("type <> ?", activity.TypeJobRun)
	if gatedStatus != "" {
		query = query.Where("status = ? AND COALESCE(gate_scope, '') <> ''", gatedStatus)
	}
	if gatedType != "" {
		query = query.Where("type = ?", gatedType)
	}
	if len(protectedIDs) > 0 {
		query = query.Where("id NOT IN ?", protectedIDs)
	}
	if err := query.Find(&candidates).Error; err != nil {
		return 0, fmt.Errorf("find abandoned activities: %w", err)
	}

	var swept int64
	var sweepErrs []error
	for i := range candidates {
		activityID := candidates[i].ID
		if candidates[i].GateScope == "" && s.isTrackedInternal(activityID) {
			continue
		}

		now := s.nowInternal()
		status := activity.StatusFailed
		message := "Activity was marked failed because its worker is no longer running"
		var errMessage *string = &message
		if gatedType == activity.TypeAutoUpdate {
			if triggered, _ := candidates[i].Metadata["selfUpdateTriggered"].(bool); triggered {
				status = activity.StatusSuccess
				message = "Auto-update completed — Arcane restarted with the updated image"
				errMessage = nil
			}
		}
		var finalized Activity
		lostRace := false
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			updates := completeActivityUpdatesInternal(candidates[i].StartedAt, status, message, errMessage, nil, now)
			if candidates[i].GateOwner != "" {
				updates["gate_owner"] = uuid.NewString()
				updates["ended_at"] = gorm.Expr("NOW()")
				updates["updated_at"] = gorm.Expr("NOW()")
				updates["duration_ms"] = gorm.Expr("GREATEST(0, EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000)::bigint")
			}
			result := tx.Model(&Activity{}).
				Where("id = ? AND status IN ? AND COALESCE(gate_owner, '') = ? AND (COALESCE(gate_scope, '') = '' OR gate_lease_at < NOW() - INTERVAL '2 minutes')", activityID, activeStatuses, candidates[i].GateOwner).
				Updates(updates)
			if result.Error != nil {
				return fmt.Errorf("fail abandoned activity: %w", result.Error)
			}
			if result.RowsAffected == 0 {
				lostRace = true
				return nil
			}
			if err := tx.First(&finalized, "id = ?", activityID).Error; err != nil {
				return fmt.Errorf("load failed abandoned activity: %w", err)
			}
			return nil
		}); err != nil {
			sweepErrs = append(sweepErrs, fmt.Errorf("sweep activity %s: %w", activityID, err))
			continue
		}
		if lostRace {
			continue
		}

		s.releaseSlotInternal(activityID)
		s.releaseCancelInternal(activityID)
		s.stopGateHeartbeatInternal(activityID)
		s.publishTerminalSnapshotInternal(ctx, &finalized)
		swept++
	}

	return swept, errors.Join(sweepErrs...)
}

// ResolveOrphanedQueuedActivities fails unowned queued work at startup.
// A gated row stays active until its owner completes or its lease expires.
func (s *ActivityService) ResolveOrphanedQueuedActivities(ctx context.Context, protectedIDs ...string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}

	failed, repairErr := s.failAbandonedActivitiesInternal(ctx, "", activity.StatusQueued, protectedIDs...)
	if repairErr != nil {
		return failed, repairErr
	}
	var queued []Activity
	if err := s.db.WithContext(ctx).
		Where("status = ?", activity.StatusQueued).
		Where("COALESCE(gate_scope, '') = ''").
		Where("type <> ?", activity.TypeJobRun).
		Where("id NOT IN ?", append(protectedIDs, "")).
		Find(&queued).Error; err != nil {
		return 0, fmt.Errorf("find orphaned queued activities: %w", err)
	}

	const message = "Queued activity was interrupted by an Arcane restart"
	errMessage := message
	var failErrs []error
	for i := range queued {
		if _, err := s.CompleteActivity(ctx, queued[i].ID, activity.StatusFailed, message, &errMessage); err != nil {
			failErrs = append(failErrs, fmt.Errorf("fail orphaned queued activity %s: %w", queued[i].ID, err))
			continue
		}
		failed++
	}

	return failed, errors.Join(failErrs...)
}

// ResolveStaleAutoUpdateActivities finalizes auto-update activities left
// running by a prior process lifetime. Gated runs wait for lease expiry.
// A triggered self-update is recorded as success; other work is failed.
func (s *ActivityService) ResolveStaleAutoUpdateActivities(ctx context.Context, protectedIDs ...string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}

	resolved, repairErr := s.failAbandonedActivitiesInternal(ctx, activity.TypeAutoUpdate, activity.StatusRunning, protectedIDs...)
	if repairErr != nil {
		return resolved, repairErr
	}
	var stale []Activity
	if err := s.db.WithContext(ctx).
		Where("type = ? AND status = ?", activity.TypeAutoUpdate, activity.StatusRunning).
		Where("COALESCE(gate_scope, '') = ''").
		Where("id NOT IN ?", append(protectedIDs, "")).
		Find(&stale).Error; err != nil {
		return 0, fmt.Errorf("find stale auto-update activities: %w", err)
	}

	var resolveErrs []error
	for i := range stale {
		status := activity.StatusFailed
		message := "Auto-update interrupted by Arcane restart"
		var errMessage *string
		if selfUpdate, _ := stale[i].Metadata["selfUpdateTriggered"].(bool); selfUpdate {
			status = activity.StatusSuccess
			message = "Auto-update completed — Arcane restarted with the updated image"
		} else {
			errMessage = new(message)
		}
		if _, err := s.CompleteActivity(ctx, stale[i].ID, status, message, errMessage); err != nil {
			resolveErrs = append(resolveErrs, fmt.Errorf("resolve stale auto-update activity %s: %w", stale[i].ID, err))
			continue
		}
		resolved++
	}

	return resolved, errors.Join(resolveErrs...)
}
