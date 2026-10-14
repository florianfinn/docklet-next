// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-14
// License: BSD-3-Clause
// Deviations: Local module namespace; gated cancellation fences the owner.

package activity

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

// CancelActivity requests cancellation of a running or queued activity. When the
// gated work records a durable request and its owner finalizes after stopping.
// Ungated local work finalizes itself; untracked ungated work is finalized here.
// Terminal or unknown activities return ErrActivityNotCancelable or record not found.
func (s *ActivityService) CancelActivity(ctx context.Context, environmentID, activityID, requestedBy string) (*activity.Activity, error) {
	if err := s.checkInitInternal(); err != nil {
		return nil, err
	}
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return nil, errors.New("activity id is required")
	}
	environmentID = cmp.Or(strings.TrimSpace(environmentID), "0")

	var model Activity
	if err := s.db.WithContext(ctx).Where("id = ? AND environment_id = ?", activityID, environmentID).First(&model).Error; err != nil {
		return nil, err
	}
	if model.Type == activity.TypeJobRun {
		return nil, ErrActivityNotCancelable
	}
	switch model.Status {
	case activity.StatusSuccess, activity.StatusFailed, activity.StatusCancelled:
		return nil, ErrActivityNotCancelable
	case activity.StatusQueued, activity.StatusRunning:
		// Active states — cancellation can proceed.
	}

	requestedBy = cmp.Or(strings.TrimSpace(requestedBy), "a user")
	if model.GateScope != "" {
		result := s.db.WithContext(ctx).Model(&Activity{}).
			Where("id = ? AND environment_id = ? AND gate_owner = ? AND status IN ?", activityID, environmentID, model.GateOwner, []activity.Status{activity.StatusQueued, activity.StatusRunning}).
			Update("cancel_requested_at", gorm.Expr("COALESCE(cancel_requested_at, NOW())"))
		if result.Error != nil {
			return nil, fmt.Errorf("request gated activity cancellation: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil, ErrActivityNotCancelable
		}
		s.RequestCancel(activityID)
		if err := s.db.WithContext(ctx).First(&model, "id = ?", activityID).Error; err != nil {
			return nil, err
		}
		return new(activityToDTOInternal(&model)), nil
	}
	writeCtx := utils.ActivityRuntimeContext(ctx, nil)
	if _, err := s.AppendMessage(writeCtx, activityID, AppendActivityMessageRequest{
		Level:   activity.MessageLevelWarning,
		Message: "Cancellation requested by " + requestedBy,
	}); err != nil {
		slog.DebugContext(ctx, "failed to append cancellation message", "activityId", activityID, "error", err)
	}

	if s.RequestCancel(activityID) {
		// The running work observes the cancelled context and writes its own
		// terminal status, which reaches clients via the activity stream. Return
		// the pre-cancel snapshot rather than reloading here: the worker has not
		// finished unwinding yet, so a reload would still report "running".
		return new(activityToDTOInternal(&model)), nil
	}

	// Ungated work without a local runner is finalized here.
	now := time.Now()
	var finalized Activity
	if err := s.db.WithContext(writeCtx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&finalized, "id = ? AND environment_id = ?", activityID, environmentID).Error; err != nil {
			return err
		}
		updates := completeActivityUpdatesInternal(finalized.StartedAt, activity.StatusCancelled, cancelledMessageInternal, nil, nil, now)
		result := tx.Model(&Activity{}).
			Where("id = ? AND status IN ? AND COALESCE(gate_owner, '') = ?", activityID, []activity.Status{activity.StatusQueued, activity.StatusRunning}, finalized.GateOwner).
			Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("failed to cancel activity: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrActivityNotCancelable
		}
		if err := tx.First(&finalized, "id = ? AND environment_id = ?", activityID, environmentID).Error; err != nil {
			return fmt.Errorf("failed to load cancelled activity: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	dto := s.publishTerminalSnapshotInternal(writeCtx, &finalized)
	s.stopGateHeartbeatInternal(activityID)
	s.releaseCancelInternal(activityID)
	return &dto, nil
}
