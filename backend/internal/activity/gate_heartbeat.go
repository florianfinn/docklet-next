// Source: florianfinn/docklet-next/backend/internal/activity
// Revision: 65c9e0dceb6f994ac884904e2a6b7348f75580fa
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: Durable liveness heartbeat for gated work.

package activity

import (
	"context"
	"log/slog"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
)

const gateHeartbeatInterval = 30 * time.Second

func (s *ActivityService) heartbeatGatedActivityInternal(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Model(&Activity{}).
		Where("id = ? AND status IN ?", id, []activitytypes.Status{activitytypes.StatusQueued, activitytypes.StatusRunning}).
		Update("updated_at", time.Now().UTC()).Error
}

func (s *ActivityService) startGateHeartbeatInternal(id string) {
	ctx, cancel := context.WithCancel(context.Background()) //nolint:forbidigo // Liveness outlives the starting request.
	s.gateHeartbeatMu.Lock()
	if s.gateHeartbeats == nil {
		s.gateHeartbeats = make(map[string]context.CancelFunc)
	}
	s.gateHeartbeats[id] = cancel
	s.gateHeartbeatMu.Unlock()
	go func() {
		ticker := time.NewTicker(gateHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.heartbeatGatedActivityInternal(ctx, id); err != nil && ctx.Err() == nil {
					slog.WarnContext(ctx, "failed to refresh gated activity heartbeat", "activityId", id, "error", err)
				}
			}
		}
	}()
}

func (s *ActivityService) stopGateHeartbeatInternal(id string) {
	s.gateHeartbeatMu.Lock()
	cancel := s.gateHeartbeats[id]
	delete(s.gateHeartbeats, id)
	s.gateHeartbeatMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
