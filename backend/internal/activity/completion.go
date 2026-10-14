// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; atomic terminal write and final message.

package activity

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
	"github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (
	s *ActivityService,
) CompleteActivity(
	ctx context.Context,
	activityID string,
	status activity.Status,
	finalMessage string,
	errMessage *string,
	finalStep ...string,
) (
	*activity.Activity,
	error,
) {
	if err := s.checkInitInternal(); err != nil {
		return nil, err
	}
	status = cmp.Or(status, activity.StatusSuccess)
	if status != activity.StatusSuccess && status != activity.StatusFailed && status != activity.StatusCancelled {
		status = activity.StatusSuccess
	}
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return nil, errors.New("activity id is required")
	}
	defer s.releaseCancelInternal(activityID)
	defer s.releaseSlotInternal(activityID)
	if errors.Is(context.Cause(ctx), activitylib.ErrCanceled) {
		status = activity.StatusCancelled
	}

	// Completion remains bounded when the worker returns on cancellation.
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), ownerWriteTimeout)
	defer cancel()

	now := s.nowInternal()
	var model Activity
	var final *ActivityMessage
	completed := false
	// Retry transient write contention so completion is persisted.
	const completeWriteAttempts = 3
	var writeErr error
	for attempt := 1; attempt <= completeWriteAttempts; attempt++ {
		writeErr = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			completed = false
			final = nil
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&model, "id = ?", activityID).Error; err != nil {
				return fmt.Errorf("failed to load activity: %w", err)
			}
			if !isTerminalActivityStatusInternal(model.Status) && !gatedWriterAllowedInternal(ctx, &model) {
				return ErrRunNotOwned
			}
			if model.GateScope != "" {
				if err := tx.Raw("SELECT NOW()").Scan(&now).Error; err != nil {
					return err
				}
			}
			if model.CancelRequestedAt != nil {
				status = activity.StatusCancelled
				if repair, _ := ctx.Value(startupRepairContextKey{}).(bool); !repair {
					finalMessage = cancelledMessageInternal
				}
				errMessage = nil
			}

			updates := completeActivityUpdatesInternal(model.StartedAt, status, finalMessage, errMessage, finalStep, now)
			if model.GateScope != "" {
				updates["ended_at"] = gorm.Expr("NOW()")
				updates["updated_at"] = gorm.Expr("NOW()")
				updates["duration_ms"] = gorm.Expr("GREATEST(0, EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000)::bigint")
			}
			code := string(activityCodeInternal(status))
			query := tx.Model(&Activity{}).Where("id = ? AND status IN ?", activityID, []activity.Status{activity.StatusQueued, activity.StatusRunning})
			result := query.Updates(updates)
			if result.Error != nil {
				return fmt.Errorf("failed to complete activity: %w", result.Error)
			}
			completed = result.RowsAffected > 0
			if completed && strings.TrimSpace(finalMessage) != "" {
				level := activity.MessageLevelSuccess
				switch status {
				case activity.StatusFailed:
					level = activity.MessageLevelError
				case activity.StatusCancelled:
					level = activity.MessageLevelWarning
				case activity.StatusQueued, activity.StatusRunning, activity.StatusSuccess:
				}
				step := ""
				if len(finalStep) > 0 {
					step = strings.TrimSpace(finalStep[0])
				}
				final = &ActivityMessage{ActivityID: activityID, Level: level, Message: strings.TrimSpace(finalMessage), Code: code, Step: step, CreatedAt: now}
				if err := tx.Create(final).Error; err != nil {
					return fmt.Errorf("failed to append final activity message: %w", err)
				}
			}
			if err := tx.First(&model, "id = ?", activityID).Error; err != nil {
				return fmt.Errorf("failed to load completed activity: %w", err)
			}
			return nil
		})
		if writeErr == nil {
			break
		}
		if attempt == completeWriteAttempts || !dbutil.IsRetryableWriteError(writeErr) {
			return nil, writeErr
		}
		slog.WarnContext(ctx, "retrying activity terminal status write", "activityId", activityID, "attempt", attempt, "error", writeErr)
		time.Sleep(250 * time.Millisecond * time.Duration(attempt))
	}
	if writeErr != nil {
		return nil, writeErr
	}

	if !completed {
		return new(activityToDTOInternal(&model)), nil
	}
	if final != nil {
		s.publishMessageInternal(model.EnvironmentID, model.Type, activityMessageToDTOInternal(final))
	}

	dto := s.publishTerminalSnapshotInternal(ctx, &model)
	return &dto, nil
}

// Re-read under the publish lock so the terminal event includes any
// message committed before it is streamed.
func (s *ActivityService) publishTerminalSnapshotInternal(ctx context.Context, model *Activity) activity.Activity {
	lock := s.publishLockInternal(model.ID)
	lock.Lock()
	defer lock.Unlock()

	var fresh Activity
	if err := s.db.WithContext(ctx).First(&fresh, "id = ?", model.ID).Error; err != nil {
		slog.DebugContext(ctx, "failed to reload activity for terminal publish", "activityId", model.ID, "error", err)
	} else {
		*model = fresh
	}
	dto := activityToDTOInternal(model)
	s.publishActivityInternal(dto)
	return dto
}

func completeActivityUpdatesInternal(startedAt time.Time, status activity.Status, finalMessage string, errMessage *string, finalStep []string, now time.Time) map[string]any {
	updates := map[string]any{
		"status":       status,
		"message_code": activityCodeInternal(status),
		"ended_at":     now,
		"duration_ms":  now.Sub(startedAt).Milliseconds(),
		"updated_at":   now,
	}
	if trimmed := strings.TrimSpace(finalMessage); trimmed != "" {
		updates["latest_message"] = trimmed
	}
	if len(finalStep) > 0 {
		if step := strings.TrimSpace(finalStep[0]); step != "" {
			updates["step"] = step
		}
	}
	if errMessage != nil && strings.TrimSpace(*errMessage) != "" {
		updates["error"] = strings.TrimSpace(*errMessage)
	}
	if status == activity.StatusSuccess {
		updates["progress"] = 100
	}
	return updates
}

func activityCodeInternal(status activity.Status) activity.Code {
	switch status {
	case activity.StatusQueued:
		return activity.CodeActivityQueued
	case activity.StatusRunning:
		return activity.CodeActivityRunning
	case activity.StatusSuccess:
		return activity.CodeActivitySuccess
	case activity.StatusFailed:
		return activity.CodeActivityFailed
	case activity.StatusCancelled:
		return activity.CodeActivityCancelled
	default:
		return activity.CodeActivityFailed
	}
}
