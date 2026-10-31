// Source: docklet-next/issues/302
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original host projection and standby liveness tests.

package environment

import (
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
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

// The stored incompatible verdict survives a tunnel drop, a failed Edge health
// probe and a hub restart, so later polls still project update-required.
func TestHostVerdictSurvivesTransportLossBeforePoll(t *testing.T) {
	for _, path := range []string{"disconnect", "restart", "health"} {
		t.Run(path, func(t *testing.T) {
			f := newAgentFixture(t, "edge", version.Info{AgentContractVersion: 10, AgentContractFingerprint: contract.AgentHubFingerprint})
			_, err := f.service.CheckAgentCompatibility(t.Context(), f.id)
			require.ErrorIs(t, err, ErrAgentUpdateRequired)
			edge.GetRegistry().Unregister(f.id)
			service := f.service
			switch path {
			case "disconnect":
				require.NoError(t, service.UpdateEnvironmentConnectionState(t.Context(), f.id, false))
			case "health":
				require.NoError(t, service.updateEnvironmentStatusInternal(t.Context(), f.id, string(EnvironmentStatusOffline)))
			default:
				service = NewEnvironmentService(f.service.db, f.service.httpClient, nil, nil, nil, nil)
				require.NoError(t, service.ReconcileEdgeStatusesOnStartup(t.Context()))
			}
			row, err := service.GetEnvironmentByID(t.Context(), f.id)
			require.NoError(t, err)
			require.Equal(t, "update-required", row.Status)
			edge.GetPollRuntimeRegistry().Update(f.id, time.Second, time.Now())
			detail, err := NewHandler(service, nil, nil, nil, nil, nil).GetEnvironment(t.Context(), &GetEnvironmentInput{ID: f.id})
			require.NoError(t, err)
			require.Equal(t, "update-required", detail.Body.Data.Status)
		})
	}
}
