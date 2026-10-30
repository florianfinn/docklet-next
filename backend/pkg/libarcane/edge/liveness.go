// Source: docklet-next/issues/302
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original application-heartbeat liveness shared by projection and dispatch.

package edge

import "time"

// TunnelLiveAt checks application progress even when the transport stays open.
func TunnelLiveAt(tunnel *AgentTunnel, now time.Time) bool {
	if tunnel == nil || tunnel.Conn == nil || tunnel.Conn.IsClosed() {
		return false
	}
	heartbeat := tunnel.GetLastHeartbeat()
	return !heartbeat.IsZero() && now.Sub(heartbeat) <= TunnelStaleTimeout
}

func (t *AgentTunnel) initializeHeartbeat() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.LastHeartbeat.IsZero() {
		t.LastHeartbeat = time.Now()
	}
}

// CleanupSchedule injects sweep delivery for deterministic lifecycle tests. Set
// it before starting the server; production uses its own 15-second ticker.
type CleanupSchedule struct {
	Now        func() time.Time
	Ticks      <-chan time.Time
	AfterSweep func()
}

func (s *TunnelServer) SetCleanupSchedule(schedule *CleanupSchedule) { s.cleanupSchedule = schedule }
