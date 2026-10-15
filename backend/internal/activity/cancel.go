// Source: getarcaneapp/arcane/backend/internal/activity/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-14
// License: BSD-3-Clause
// Deviations: Local module namespace; in-memory cancellation holds the gate until completion.

package activity

import (
	"cmp"
	"context"
	"errors"
	"strings"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/types/activity"
)

// CancelActivity signals local work; untracked active work completes directly.
func (s *ActivityService) CancelActivity(ctx context.Context, environmentID, activityID, _ string) (*activity.Activity, error) {
	if err := s.checkInitInternal(); err != nil {
		return nil, err
	}
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return nil, errors.New("activity id is required")
	}
	environmentID = cmp.Or(strings.TrimSpace(environmentID), "0")
	s.runningMu.Lock()
	var model Activity
	err := s.db.WithContext(ctx).Where("id = ? AND environment_id = ?", activityID, environmentID).First(&model).Error
	if err != nil {
		s.runningMu.Unlock()
		return nil, err
	}
	if model.Type == activity.TypeJobRun || isTerminalActivityStatusInternal(model.Status) {
		s.runningMu.Unlock()
		return nil, ErrActivityNotCancelable
	}
	if cancel, tracked := s.running[activityID]; tracked {
		if s.cancelling == nil {
			s.cancelling = make(map[string]bool)
		}
		s.cancelling[activityID] = true
		cancel(activitylib.ErrCanceled)
		s.runningMu.Unlock()
		return new(activityToDTOInternal(&model)), nil
	}
	s.runningMu.Unlock()

	// No local worker can finish this row; the terminal status guard handles a race.
	return s.CompleteActivity(context.WithValue(ctx, startupRepairContextKey{}, true), activityID, activity.StatusCancelled, cancelledMessageInternal, nil)
}
