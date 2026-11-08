// Source: docklet-next/issues/368
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original hub restart run proofs for a busy agent over real Direct and Edge transports.

package system

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

// startBusyEdgeAgent connects the fixture's real receiver through a real WebSocket tunnel.
func startBusyEdgeAgent(t *testing.T, f *restartTransportFixture) {
	t.Helper()
	registry := edge.NewTunnelRegistry()
	previous := edge.GetRegistry()
	edge.SetDefaultRegistry(registry)
	t.Cleanup(func() { edge.SetDefaultRegistry(previous) })
	connected := make(chan struct{}, 8)
	server := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return "example-host", nil }, func(_ context.Context, _ string, online bool) {
		if online {
			connected <- struct{}{}
		}
	})
	server.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return "example-key", nil })
	router := echo.New()
	router.GET("/api/tunnel/connect", server.HandleConnect)
	manager := httptest.NewServer(router)
	tunnelClient := edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: "example", EdgeReconnectInterval: 1}, f.agent)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); tunnelClient.StartWithErrorChan(ctx, nil) }()
	t.Cleanup(func() {
		cancel()
		registry.Unregister("example-host")
		<-done
		manager.Close()
	})
	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("Edge registration missing")
	}
}

// A timely agent 409 host.busy ends the hub run as failed host.busy: durable,
// reservation released, no Docker restart; once the backup ends a restart runs.
func TestRestartRefusedByBusyAgentRealDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newRestartTransportFixture(t)
			if mode == "direct" {
				agent := httptest.NewServer(f.agent)
				t.Cleanup(agent.Close)
				f.startHub(t, agent.URL, false)
			} else {
				startBusyEdgeAgent(t, f)
				f.startHub(t, "edge://example", true)
			}
			f.busy.Store(true)
			status, run := f.post(t)
			require.Equal(t, 202, status, "the hub accepts the request as a durable run")
			refused := f.await(t, run.RunID, "failed")
			require.Equal(t, "host.busy", refused.Code)
			require.NotNil(t, refused.SettledAt)
			require.EqualValues(t, 1, f.postCalls.Load())

			var row environment.AgentRestartRequest
			require.NoError(t, f.db.First(&row, "run_id=?", run.RunID).Error)
			require.Equal(t, "failed", row.State, "the refusal is durable")
			require.NotNil(t, row.Code)
			require.Equal(t, "host.busy", *row.Code)
			select {
			case <-f.restarted:
				t.Fatal("a refused restart reached Docker")
			case <-time.After(200 * time.Millisecond):
			}

			f.busy.Store(false)
			status, next := f.post(t)
			require.Equal(t, 202, status, "the refused run released the reservation")
			f.await(t, next.RunID, "awaiting")
			require.EqualValues(t, 2, f.postCalls.Load())
			close(f.release)
			select {
			case <-f.restarted:
			case <-time.After(5 * time.Second):
				t.Fatal("the restart after the backup did not reach Docker")
			}
			if mode == "direct" {
				f.await(t, next.RunID, "connected")
			}
		})
	}
}
