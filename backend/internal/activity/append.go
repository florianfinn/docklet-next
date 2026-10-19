// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-15
// License: BSD-3-Clause
// Deviations: Extracted batch append path; configurable raw output persistence with coded messages.

package activity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/florianfinn/docklet-next/types/activity"
)

// AppendMessages persists messages and raw lines together, then publishes the
// committed activity snapshot.
func (s *ActivityService) AppendMessages(ctx context.Context, activityID string, reqs []AppendActivityMessageRequest) ([]activity.Message, error) {
	if err := s.checkInitInternal(); err != nil {
		return nil, err
	}
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return nil, errors.New("activity id is required")
	}
	ctx, cancel := ownerWriteContextInternal(ctx)
	defer cancel()
	for _, req := range reqs {
		if code := strings.TrimSpace(req.Code); code != "" && !activity.Valid(code) {
			return nil, fmt.Errorf("unknown activity message code: %s", code)
		}
	}

	messages, updates := buildAppendBatchInternal(activityID, reqs, s.nowInternal())
	var output []activity.OutputLine
	for _, req := range reqs {
		if req.Output != nil {
			output = append(output, *req.Output)
		}
	}
	retainOutput := s.settings == nil || s.settings.GetBoolSetting(ctx, "keepActionOutput", true)
	if len(messages) == 0 && len(output) == 0 {
		return nil, nil
	}
	if updates == nil {
		updates = make(map[string]any, 2)
	}

	// Hold the per-activity publish lock across commit and publish so this
	// batch's snapshot cannot be overtaken on the stream by a concurrent
	// UpdateActivity that committed later — subscribers must see the two
	// snapshots in commit order.
	lock := s.publishLockInternal(activityID)
	lock.Lock()
	defer lock.Unlock()

	current, err := s.appendBatchWithRetryInternal(ctx, activityID, messages, updates, output, retainOutput)
	if err != nil {
		return nil, err
	}

	out := make([]activity.Message, 0, len(messages))
	for _, message := range messages {
		dto := activityMessageToDTOInternal(message)
		s.publishMessageInternal(current.EnvironmentID, current.Type, dto)
		out = append(out, dto)
	}
	s.publishActivityInternal(activityToDTOInternal(current))
	return out, nil
}
