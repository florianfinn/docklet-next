// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package environment

import (
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestAgentCompatibilityCachedPerSession(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newAgentFixture(t, mode, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
			changes, unsubscribe := f.service.SubscribeRuntimeChanges()
			defer unsubscribe()
			writes := 0
			require.NoError(t, f.service.db.Callback().Update().After("gorm:update").Register("test:compatibility-writes", func(*gorm.DB) { writes++ }))
			forward := func() {
				_, err := f.service.ExecuteRemoteRequest(t.Context(), f.id, "GET", "/api/environments/0/containers", nil)
				require.NoError(t, err)
			}
			forward()
			require.Equal(t, 1, writes)
			select {
			case <-changes:
			default:
				t.Fatal("initial verdict must publish state")
			}
			for range 5 {
				forward()
			}
			require.Equal(t, 1, f.count("GET /api/app-version"))
			require.Equal(t, 6, f.count("GET /api/environments/0/containers"))
			require.Equal(t, 1, writes, "cached requests must not update the database")
			select {
			case <-changes:
				t.Fatal("cached requests must not publish state")
			default:
			}
			f.service.apiKeys = apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
			env, err := f.service.GetEnvironmentByID(t.Context(), f.id)
			require.NoError(t, err)
			_, err = f.service.RegenerateEnvironmentApiKey(t.Context(), env, "operator", "operator")
			require.NoError(t, err)
			if mode == "edge" {
				env, err = f.service.GetEnvironmentByID(t.Context(), f.id)
				require.NoError(t, err)
				f.conn.mu.Lock()
				f.conn.closed = false
				f.conn.mu.Unlock()
				f.tunnel = edge.NewAgentTunnelWithConn(f.id, f.conn)
				f.tunnel.APIKeyID = *env.ApiKeyID
				f.tunnel.TokenHash = edge.AgentTokenHash(*env.AccessToken)
				edge.GetRegistry().Register(f.id, f.tunnel)
			}
			forward()
			require.Equal(t, 2, f.count("GET /api/app-version"), "rotation creates a new session verdict")
		})
	}
}

func TestAgentCompatibilityRefreshInvalidatesVerdict(t *testing.T) {
	f := newAgentFixture(t, "direct", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
	forward := func() error {
		_, err := f.service.ExecuteRemoteRequest(t.Context(), f.id, "GET", "/api/environments/0/containers", nil)
		return err
	}
	require.NoError(t, forward())
	f.info.AgentContractFingerprint = "different"
	_, err := f.service.CheckAgentCompatibility(t.Context(), f.id)
	require.ErrorIs(t, err, ErrAgentUpdateRequired)
	require.ErrorIs(t, forward(), ErrAgentUpdateRequired)
	require.Equal(t, 2, f.count("GET /api/app-version"))
	require.Equal(t, 1, f.count("GET /api/environments/0/containers"))
	f.info.AgentContractFingerprint = contract.AgentHubFingerprint
	status, err := f.service.TestConnection(t.Context(), f.id, nil)
	require.NoError(t, err)
	require.Equal(t, "online", status)
	require.NoError(t, forward())
	require.Equal(t, 3, f.count("GET /api/app-version"), "health checks refresh even a cached rejection")
	for _, update := range []map[string]any{{"access_token": "test-only"}, {"api_url": "http://replacement.example.com", "access_token": "test-only"}, {"api_key_id": "test-only"}} {
		_, err = f.service.UpdateEnvironment(t.Context(), f.id, update, nil, nil)
		require.NoError(t, err)
		require.NoError(t, forward())
	}
	require.Equal(t, 6, f.count("GET /api/app-version"))
}

func TestEdgeReconnectRefreshesCompatibility(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
	_, err := f.service.ExecuteRemoteRequest(t.Context(), f.id, "GET", "/api/environments/0/containers", nil)
	require.NoError(t, err)
	oldContext, cancel, err := f.service.AgentAccess(t.Context(), f.id, &f.token, "GET", "/api/environments/0/containers")
	require.NoError(t, err)
	defer cancel()
	// Reconnection is visible in the registry before its asynchronous version callback.
	send := f.conn.send
	f.conn = &agentMemoryConn{send: send}
	f.tunnel = edge.NewAgentTunnelWithConn(f.id, f.conn)
	f.tunnel.APIKeyID = "example"
	f.tunnel.TokenHash = edge.AgentTokenHash(f.token)
	edge.GetRegistry().Register(f.id, f.tunnel)
	f.info.AgentContractFingerprint = "different"
	_, err = f.service.ExecuteRemoteRequest(t.Context(), f.id, "GET", "/api/environments/0/containers", nil)
	require.ErrorIs(t, err, ErrAgentUpdateRequired)
	require.Equal(t, 2, f.count("GET /api/app-version"))
	require.Equal(t, 1, f.count("GET /api/environments/0/containers"))
	require.Error(t, edge.RequireSessionKey(oldContext, f.tunnel))
	require.False(t, f.conn.IsClosed(), "the rejected old request must leave the current connection open")
}
