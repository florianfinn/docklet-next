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
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CancelActivity requests cancellation of a running or queued activity. When the
// activity's work is running in this process it interrupts it (the work finalizes
// its own terminal status); otherwise it marks the activity cancelled directly,
// but only if it is still active. Returns ErrActivityNotCancelable if the activity
// has already reached a terminal state, or gorm.ErrRecordNotFound if it is unknown.
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
	writeCtx := utils.ActivityRuntimeContext(ctx, nil)
	if model.GateOwner != "" {
		writeCtx = context.WithValue(writeCtx, gateOwnerContextKey{}, model.GateOwner)
	}
	if _, err := s.AppendMessage(writeCtx, activityID, AppendActivityMessageRequest{
		Level:   activity.MessageLevelWarning,
		Message: "Cancellation requested by " + requestedBy,
	}); err != nil {
		slog.DebugContext(ctx, "failed to append cancellation message", "activityId", activityID, "error", err)
	}

	if s.RequestCancel(activityID) && model.GateOwner == "" {
		// The running work observes the cancelled context and writes its own
		// terminal status, which reaches clients via the activity stream. Return
		// the pre-cancel snapshot rather than reloading here: the worker has not
		// finished unwinding yet, so a reload would still report "running".
		return new(activityToDTOInternal(&model)), nil
	}

	// Gated work is fenced immediately after cancellation. Ungated work with no
	// local runner is also finalized here.
	now := time.Now()
	var finalized Activity
	if err := s.db.WithContext(writeCtx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&finalized, "id = ? AND environment_id = ?", activityID, environmentID).Error; err != nil {
			return err
		}
		updates := completeActivityUpdatesInternal(finalized.StartedAt, activity.StatusCancelled, cancelledMessageInternal, nil, nil, now)
		if finalized.GateOwner != "" {
			updates["gate_owner"] = uuid.NewString()
		}
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
