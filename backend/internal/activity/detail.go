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

const maxStepTransitions = 256

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
	if err := s.db.WithContext(ctx).Raw(`
		SELECT id, step, code, created_at FROM (
			SELECT id, step, code, created_at,
				LAG(step) OVER (ORDER BY created_at, id) AS previous_step
			FROM activity_messages WHERE activity_id = ? AND step <> ''
		) AS transitions
		WHERE previous_step IS DISTINCT FROM step
		ORDER BY created_at DESC, id DESC LIMIT ?`, activityID, maxStepTransitions).Scan(&stepMessages).Error; err != nil {
		return nil, fmt.Errorf("failed to load activity steps: %w", err)
	}
	slices.Reverse(stepMessages)

	return &activity.Detail{
		Activity:        activityToDTOInternal(&model),
		Messages:        outMessages,
		StepTrace:       stepTraceInternal(stepMessages, &model),
		Output:          append([]activity.OutputLine{}, model.Output...),
		OutputTruncated: model.OutputTruncated,
	}, nil
}
