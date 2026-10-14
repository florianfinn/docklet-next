// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-14
// License: BSD-3-Clause
// Deviations: Local module namespace; durable cancellation request and local signal.

package activity

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CancelActivity records the first request and signals the local worker.
// The worker or startup repair writes the terminal state.
func (s *ActivityService) CancelActivity(ctx context.Context, environmentID, activityID, requestedBy string) (*activity.Activity, error) {
	if err := s.checkInitInternal(); err != nil {
		return nil, err
	}
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return nil, errors.New("activity id is required")
	}
	environmentID = cmp.Or(strings.TrimSpace(environmentID), "0")
	requestedBy = cmp.Or(strings.TrimSpace(requestedBy), "a user")

	lock := s.publishLockInternal(activityID)
	lock.Lock()
	defer lock.Unlock()
	s.runningMu.Lock()
	defer s.runningMu.Unlock()

	var model Activity
	var audit *ActivityMessage
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND environment_id = ?", activityID, environmentID).First(&model).Error; err != nil {
			return err
		}
		if model.Type == activity.TypeJobRun || isTerminalActivityStatusInternal(model.Status) {
			return ErrActivityNotCancelable
		}
		if model.CancelRequestedAt != nil {
			return nil
		}
		message := "Cancellation requested by " + requestedBy
		if err := tx.Model(&Activity{}).Where("id = ? AND status IN ? AND cancel_requested_at IS NULL", activityID, []activity.Status{activity.StatusQueued, activity.StatusRunning}).Updates(map[string]any{
			"cancel_requested_at": gorm.Expr("NOW()"),
			"latest_message":      message,
			"message_code":        activity.CodeActivityCancelRequested,
			"updated_at":          gorm.Expr("NOW()"),
		}).Error; err != nil {
			return fmt.Errorf("request activity cancellation: %w", err)
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
	if cancel := s.running[activityID]; cancel != nil {
		cancel(activitylib.ErrCanceled)
	}
	return new(activityToDTOInternal(&model)), nil
}
