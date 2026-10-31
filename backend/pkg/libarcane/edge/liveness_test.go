// Source: docklet-next/issues/302
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original liveness boundary and replacement-session sweep tests.

package edge

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

func TestHeartbeatLivenessBoundaryAndDispatch(t *testing.T) {
	registry := GetRegistry()
	id := "example-liveness"
	conn := &livenessConn{}
	tunnel := NewAgentTunnelWithConn(id, conn)
	registry.Register(id, tunnel)
	defer registry.Unregister(id)
	start := tunnel.GetLastHeartbeat()
	require.True(t, TunnelLiveAt(tunnel, start.Add(90*time.Second)))
	require.False(t, TunnelLiveAt(tunnel, start.Add(90*time.Second+time.Nanosecond)))
	tunnel.mu.Lock()
	tunnel.LastHeartbeat = time.Now().Add(-91 * time.Second)
	tunnel.mu.Unlock()
	require.False(t, HasActiveTunnel(id))
	require.True(t, GetTunnelRuntimeState(id).IsAbsent())
	_, _, _, err := ProxyRequest(t.Context(), tunnel, "GET", "/api/health", "", nil, nil)
	require.Error(t, err)
	require.Zero(t, conn.sent.Load(), "a stale peer must receive zero dispatches")
}

func TestHeartbeatSweepKeepsHealthyAndReplacedSessions(t *testing.T) {
	r := NewTunnelRegistry()
	id := "example-replacement"
	now := time.Now()
	old := NewAgentTunnelWithConn(id, NewGRPCManagerTunnelConn(nil))
	old.SessionID = "old"
	old.LastHeartbeat = now.Add(-91 * time.Second)
	r.Register(id, old)
	// A scan retained the old session; a fresh registration wins before removal.
	candidates := []*AgentTunnel{old}
	fresh := NewAgentTunnelWithConn(id, NewGRPCManagerTunnelConn(nil))
	fresh.SessionID = "new"
	r.Register(id, fresh)
	require.Empty(t, r.cleanupStaleCandidates(t.Context(), TunnelStaleTimeout, now.Add(105*time.Second), candidates))
	current, ok := r.Get(id).Get()
	require.True(t, ok)
	require.Same(t, fresh, current)
	require.False(t, fresh.Conn.IsClosed())
	future := now.Add(5 * time.Minute)
	fresh.mu.Lock()
	fresh.LastHeartbeat = future
	fresh.mu.Unlock()
	require.Empty(t, r.CleanupStaleAt(t.Context(), TunnelStaleTimeout, future.Add(90*time.Second)))
	removed := r.CleanupStaleAt(t.Context(), TunnelStaleTimeout, future.Add(105*time.Second))
	require.Equal(t, []*AgentTunnel{fresh}, removed)
	require.True(t, fresh.Conn.IsClosed())
	require.True(t, r.Get(id).IsAbsent())
	require.NoError(t, r.Stop(context.WithoutCancel(t.Context())))
}

type livenessConn struct {
	sent   atomic.Int32
	closed atomic.Bool
}

func (c *livenessConn) Send(*TunnelMessage) error {
	c.sent.Add(1)
	return errors.New("synthetic send rejected")
}
func (c *livenessConn) Receive() (*TunnelMessage, error)  { return nil, io.EOF }
func (c *livenessConn) IsExpectedReceiveError(error) bool { return true }
func (c *livenessConn) IsClosed() bool                    { return c.closed.Load() }
func (c *livenessConn) Close() error                      { c.closed.Store(true); return nil }
func (c *livenessConn) Transport() string                 { return EdgeTransportWebSocket }
