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
	"gorm.io/gorm/clause"
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
		lock := s.publishLockInternal(activityID)
		lock.Lock()
		defer lock.Unlock()
		var audit *ActivityMessage
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND environment_id = ?", activityID, environmentID).First(&model).Error; err != nil {
				return err
			}
			if isTerminalActivityStatusInternal(model.Status) {
				return ErrActivityNotCancelable
			}
			if model.CancelRequestedAt != nil {
				return nil
			}
			message := "Cancellation requested by " + requestedBy
			result := tx.Model(&Activity{}).
				Where("id = ? AND gate_owner = ? AND status IN ?", activityID, model.GateOwner, []activity.Status{activity.StatusQueued, activity.StatusRunning}).
				Updates(map[string]any{
					"cancel_requested_at": gorm.Expr("NOW()"),
					"latest_message":      message,
					"message_code":        activity.CodeActivityCancelRequested,
					"updated_at":          gorm.Expr("NOW()"),
				})
			if result.Error != nil {
				return fmt.Errorf("request gated activity cancellation: %w", result.Error)
			}
			if result.RowsAffected == 0 {
				return ErrLeaseLost
			}
			var databaseNow time.Time
			if err := tx.Raw("SELECT NOW()").Scan(&databaseNow).Error; err != nil {
				return err
			}
			audit = &ActivityMessage{ActivityID: activityID, Level: activity.MessageLevelWarning, Message: message, Code: string(activity.CodeActivityCancelRequested), CreatedAt: databaseNow}
			if err := tx.Create(audit).Error; err != nil {
				return err
			}
			return tx.First(&model, "id = ?", activityID).Error
		})
		if err != nil {
			return nil, err
		}
		if audit != nil {
			s.publishMessageInternal(model.EnvironmentID, model.Type, activityMessageToDTOInternal(audit))
			s.publishActivityInternal(activityToDTOInternal(&model))
		}
		s.RequestCancel(activityID)
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
