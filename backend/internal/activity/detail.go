// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; run detail and complete step trace.

package activity

import (
	"context"
	"fmt"
	"slices"

	"github.com/florianfinn/docklet-next/types/activity"
)

func (s *ActivityService) GetActivityDetail(ctx context.Context, environmentID, activityID string, limit int) (*activity.Detail, error) {
	if err := s.checkInitInternal(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > defaultActivityMessages {
		limit = defaultActivityMessages
	}

	var model Activity
	if err := s.db.WithContext(ctx).
		Where("id = ? AND environment_id = ?", activityID, environmentID).
		First(&model).Error; err != nil {
		return nil, fmt.Errorf("failed to load activity: %w", err)
	}

	var messages []ActivityMessage
	if err := s.db.WithContext(ctx).
		Where("activity_id = ?", activityID).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to load activity messages: %w", err)
	}

	outMessages := make([]activity.Message, 0, len(messages))
	for _, v := range slices.Backward(messages) {
		outMessages = append(outMessages, activityMessageToDTOInternal(&v))
	}
	var stepMessages []ActivityMessage
	if err := s.db.WithContext(ctx).Model(&ActivityMessage{}).
		Select("id, step, code, created_at").Where("activity_id = ? AND step <> ''", activityID).
		Order("created_at ASC, id ASC").Find(&stepMessages).Error; err != nil {
		return nil, fmt.Errorf("failed to load activity steps: %w", err)
	}

	return &activity.Detail{
		Activity:  activityToDTOInternal(&model),
		Messages:  outMessages,
		StepTrace: stepTraceInternal(stepMessages, &model),
	}, nil
}
