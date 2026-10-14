// Source: florianfinn/docklet-next/backend/internal/activity
// Revision: 65c9e0dceb6f994ac884904e2a6b7348f75580fa
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: Durable liveness heartbeat for gated work.

package activity

import (
	"context"
	"errors"
	"log/slog"
	"time"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
)

const gateHeartbeatInterval = 30 * time.Second

func (s *ActivityService) heartbeatGatedActivityInternal(ctx context.Context, id, owner string) error {
	var result struct{ CancelRequestedAt *time.Time }
	write := s.db.WithContext(ctx).Raw("UPDATE activities SET gate_lease_at = NOW() WHERE id = ? AND gate_owner = ? AND status IN ('queued', 'running') RETURNING cancel_requested_at", id, owner).Scan(&result)
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected == 0 {
		return ErrLeaseLost
	}
	if result.CancelRequestedAt != nil {
		s.cancelTrackedInternal(id, activitylib.ErrCanceled)
	}
	return nil
}

func (s *ActivityService) startGateHeartbeatInternal(workCtx context.Context, id, owner string) {
	ctx, cancel := context.WithCancel(workCtx)
	s.gateHeartbeatMu.Lock()
	if s.gateHeartbeats == nil {
		s.gateHeartbeats = make(map[string]context.CancelFunc)
	}
	s.gateHeartbeats[id] = cancel
	s.gateHeartbeatMu.Unlock()
	go func() {
		ticks := s.gateHeartbeatTicks
		if ticks == nil {
			interval := s.gateHeartbeatInterval
			if interval <= 0 {
				interval = gateHeartbeatInterval
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			ticks = ticker.C
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticks:
				if err := s.heartbeatGatedActivityInternal(ctx, id, owner); err != nil && ctx.Err() == nil {
					if errors.Is(err, ErrLeaseLost) {
						s.cancelTrackedInternal(id, ErrLeaseLost)
						return
					}
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

func (s *ActivityService) hasGateHeartbeatInternal(id string) bool {
	s.gateHeartbeatMu.Lock()
	defer s.gateHeartbeatMu.Unlock()
	_, ok := s.gateHeartbeats[id]
	return ok
}
