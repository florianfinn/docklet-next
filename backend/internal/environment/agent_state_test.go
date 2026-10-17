// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package environment

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	environmenttypes "github.com/florianfinn/docklet-next/types/environment"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

func TestHostDisconnectReconnectAndRestartState(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	f := newAgentFixture(t, "edge", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
	_, err := f.service.CheckAgentCompatibility(ctx, f.id)
	require.NoError(t, err)
	connected, err := f.service.GetEnvironmentByID(ctx, f.id)
	require.NoError(t, err)
	require.NotNil(t, connected.LastSeen)
	key := hoststate.Key{HostID: f.id, List: "containers"}
	f.service.ReadSnapshots().Put(key, []byte(`[]`))
	edge.GetRegistry().Unregister(f.id)
	require.NoError(t, f.service.UpdateEnvironmentConnectionState(ctx, f.id, false))
	offline, err := f.service.GetEnvironmentByID(ctx, f.id)
	require.NoError(t, err)
	require.Equal(t, "offline", offline.Status)
	require.Equal(t, *connected.LastSeen, *offline.LastSeen)
	_, retained := f.service.ReadSnapshots().Get(key)
	require.True(t, retained)
	dto := environmenttypes.Environment{ID: f.id, Status: offline.Status, IsEdge: true}
	f.service.ApplyHostRuntimeState(ctx, &dto)
	require.NotNil(t, dto.AgentReport)
	require.Equal(t, "offline", dto.Status)
	f.conn.mu.Lock()
	f.conn.closed = false
	f.conn.mu.Unlock()
	f.tunnel = edge.NewAgentTunnelWithConn(f.id, f.conn)
	f.tunnel.APIKeyID = "example"
	f.tunnel.TokenHash = edge.AgentTokenHash(f.token)
	edge.GetRegistry().Register(f.id, f.tunnel)
	require.NoError(t, f.service.UpdateEnvironmentConnectionState(ctx, f.id, true))
	_, err = f.service.CheckAgentCompatibility(ctx, f.id)
	require.NoError(t, err)
	online, err := f.service.GetEnvironmentByID(ctx, f.id)
	require.NoError(t, err)
	require.Equal(t, "online", online.Status)
	require.False(t, online.LastSeen.Before(*offline.LastSeen))
	edge.GetRegistry().Unregister(f.id)
	restarted := NewEnvironmentService(f.service.db, f.service.httpClient, nil, nil, nil, nil)
	require.NoError(t, restarted.ReconcileEdgeStatusesOnStartup(ctx))
	_, retained = restarted.ReadSnapshots().Get(key)
	require.False(t, retained)
	row, err := restarted.GetEnvironmentByID(ctx, f.id)
	require.NoError(t, err)
	require.Equal(t, *online.LastSeen, *row.LastSeen)
	dto = environmenttypes.Environment{ID: f.id, Status: row.Status, IsEdge: true}
	restarted.ApplyHostRuntimeState(ctx, &dto)
	require.Nil(t, dto.AgentReport)
	require.Equal(t, "offline", dto.Status)
}

func TestHostRuntimeStateRejectsOldTunnelKey(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	f.tunnel.APIKeyID = "test-only"
	dto := environmenttypes.Environment{ID: f.id, Status: "online", IsEdge: true}
	f.service.ApplyHostRuntimeState(t.Context(), &dto)
	require.NotNil(t, dto.Connected)
	require.False(t, *dto.Connected)
	require.Equal(t, "offline", dto.Status)
}

func TestHostRuntimeStateRejectsOldTunnelToken(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	f.tunnel.TokenHash = edge.AgentTokenHash("test-only")
	dto := environmenttypes.Environment{ID: f.id, Status: "online", IsEdge: true}
	f.service.ApplyHostRuntimeState(t.Context(), &dto)
	require.False(t, *dto.Connected)
	require.Equal(t, "offline", dto.Status)
}
