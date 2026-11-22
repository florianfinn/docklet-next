// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original proofs that agent pairing uses the tailnet dialer while general clients keep normal routing.

package bootstrap

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet/tailnettest"
)

func TestAgentPairingUsesTheManagerDialer(t *testing.T) {
	tailnettest.HostileProxyEnv(t)
	hub := tailnettest.NewSocketHub(t, nil)
	cfg := &config.Config{AgentMode: true, EdgeAgent: true, AgentToken: "test-only", ManagerApiUrl: tailnettest.UnreachableManager, ManagerDialer: hub.Dial}
	require.NoError(t, handleAgentBootstrapPairing(t.Context(), cfg, http.DefaultClient))
	require.True(t, hub.Saw("POST /api/environments/pair"))
}

// Registry, mail and local requests use the general client, which never takes the manager dialer.
func TestGeneralHTTPClientKeepsNormalRouting(t *testing.T) {
	var dials atomic.Int64
	cfg := &config.Config{ManagerDialer: func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, net.ErrClosed
	}}
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	t.Cleanup(local.Close)
	resp, err := newConfiguredHTTPClientInternal(cfg).Get(local.URL) //nolint:noctx // Test request to a loopback server.
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Zero(t, dials.Load())
}
