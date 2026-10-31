// Source: docklet-next/issues/302
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original host projection and standby liveness tests.

package environment

import (
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestHostProjectionRejectsStaleHeartbeatAndPreservesStandby(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	now := f.tunnel.GetLastHeartbeat()
	for _, tc := range []struct {
		age       time.Duration
		status    string
		connected bool
	}{{90 * time.Second, "online", true}, {91 * time.Second, "offline", false}} {
		projected := dto.Environment{ID: f.id, IsEdge: true, Status: "online"}
		f.service.ApplyHostRuntimeStateAt(t.Context(), &projected, now.Add(tc.age))
		require.Equal(t, tc.status, projected.Status)
		require.Equal(t, tc.connected, *projected.Connected)
	}
	edge.GetPollRuntimeRegistry().Update(f.id, time.Hour, now)
	projected := dto.Environment{ID: f.id, IsEdge: true, Status: "online"}
	f.service.ApplyHostRuntimeStateAt(t.Context(), &projected, now.Add(91*time.Second))
	require.Equal(t, "standby", projected.Status)
	require.False(t, *projected.Connected)
}

// A polling agent below the minimum protocol keeps update-required instead of
// standby, as the tunnel branch already does (UP1, #100).
func TestHostProjectionKeepsUpdateRequiredForPollingAgent(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	now := f.tunnel.GetLastHeartbeat()
	edge.GetPollRuntimeRegistry().Update(f.id, time.Hour, now)
	projected := dto.Environment{ID: f.id, IsEdge: true, Status: "update-required"}
	f.service.ApplyHostRuntimeStateAt(t.Context(), &projected, now.Add(91*time.Second))
	require.Equal(t, "update-required", projected.Status)
	require.False(t, *projected.Connected)
	require.NotNil(t, projected.LastPollAt)
}
