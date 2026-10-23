// Source: docklet-next/docs/design/network-management.md
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package network

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
)

var updateNetworkFixtures = flag.Bool("update-network-fixtures", false, "Update network handler response fixtures")

// Only Docker is replaced: the hub environment middleware, Direct HTTP proxy,
// command client, WebSocket tunnel and agent-side handler/service are real.
func networkTransport(t *testing.T, mode string, handler http.Handler) http.Handler {
	t.Helper()
	registry := edge.NewTunnelRegistry()
	target := ""
	token := uuid.New().String()
	if mode == "direct" {
		agent := httptest.NewServer(handler)
		t.Cleanup(agent.Close)
		target = agent.URL
	} else {
		connected := make(chan struct{})
		var once sync.Once
		tunnelServer := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return "example-host", nil }, func(_ context.Context, _ string, online bool) {
			if online {
				once.Do(func() { close(connected) })
			}
		})
		router := echo.New()
		router.GET("/api/tunnel/connect", tunnelServer.HandleConnect)
		manager := httptest.NewServer(router)
		t.Cleanup(manager.Close)
		agent := edge.NewTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: token}, handler)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { defer close(done); agent.StartWithErrorChan(ctx, nil) }()
		t.Cleanup(func() {
			cancel()
			registry.Unregister("example-host")
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("tunnel cleanup did not join")
			}
		})
		select {
		case <-connected:
		case <-time.After(5 * time.Second):
			t.Fatal("tunnel registration barrier failed")
		}
		target = "edge://example-host"
	}
	grants := authz.NewPermissionSet()
	grants.AddEnv("example-host", authz.PermNetworksCreate, authz.PermNetworksDelete, authz.PermNetworksConnect, authz.PermNetworksDisconnect)
	matcher := authz.NewPermissionMatcher()
	matcher.Add("POST", "/networks", authz.PermNetworksCreate)
	matcher.Add("DELETE", "/networks/{networkId}", authz.PermNetworksDelete)
	matcher.Add("POST", "/networks/{networkId}/connect", authz.PermNetworksConnect)
	matcher.Add("POST", "/networks/{networkId}/disconnect", authz.PermNetworksDisconnect)
	hub := echo.New()
	hub.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id",
		func(context.Context, string) (string, *string, bool, error) { return target, &token, true, nil },
		func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
			return grants, &actor, true
		}, matcher, registry, nil))
	hub.Any("/api/environments/:id/*", func(c *echo.Context) error { return c.NoContent(500) })
	return hub
}

func TestNetworkMembershipThroughDirectAndEdge(t *testing.T) {
	snapshots := map[string]json.RawMessage{}
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			running := disconnectedContainer()
			running.State = &container.State{Running: true}
			stopped := stoppedReference()
			f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}, containers: []container.InspectResponse{running, stopped}}
			call := networkTransport(t, mode, networkRouter(t, f.service(t), true))
			for _, tc := range []struct {
				name, method, path, body string
				status                   int
				code                     string
			}{
				{"taken-stopped", "POST", "/net1/connect", `{"containerId":"container1","ipv4Address":"192.0.2.7"}`, 400, "network.invalid-ip"},
				{"outside", "POST", "/net1/connect", `{"containerId":"container1","ipv4Address":"198.51.100.7"}`, 400, "network.invalid-ip"},
				{"already-stopped", "POST", "/net1/connect", `{"containerId":"stopped"}`, 409, "network.already-connected"},
				{"missing-network", "POST", "/missing/connect", `{"containerId":"container1"}`, 404, "network.not-found"},
				{"missing-container", "POST", "/net1/connect", `{"containerId":"missing"}`, 404, "network.not-found"},
				{"not-connected", "POST", "/net1/disconnect", `{"containerId":"container1"}`, 409, "network.not-connected"},
				{"delete-stopped", "DELETE", "/net1", ``, 409, "network.in-use"},
				{"create-invalid", "POST", "", `{"name":"new-example","options":{"driver":"overlay"}}`, 400, "network.invalid-network"},
				{"connect", "POST", "/net1/connect", `{"containerId":"container1","ipv4Address":"192.0.2.8","ipv6Address":"2001:db8::8","aliases":["example","example"]}`, 200, ""},
				{"disconnect", "POST", "/net1/disconnect", `{"containerId":"stopped","force":false}`, 200, ""},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rec := networkRequest(call, tc.method, "/api/environments/example-host/networks"+tc.path, tc.body)
					require.Equal(t, tc.status, rec.Code, rec.Body.String())
					if tc.code != "" {
						var problem struct{ Code string }
						require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
						require.Equal(t, tc.code, problem.Code)
					}
					require.True(t, json.Valid(rec.Body.Bytes()))
					var payload map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
					// Huma's schema link depends on the transport request origin.
					delete(payload, "$schema")
					body, err := json.Marshal(payload)
					require.NoError(t, err)
					if mode == "direct" {
						snapshots[tc.name] = append(json.RawMessage(nil), body...)
					} else {
						require.JSONEq(t, string(snapshots[tc.name]), string(body))
					}
				})
			}
			require.Equal(t, []string{"connect", "disconnect"}, f.writes)
			require.Equal(t, []string{"example"}, f.connected.EndpointConfig.Aliases)
			require.Equal(t, "192.0.2.8", f.connected.EndpointConfig.IPAMConfig.IPv4Address.String())
			require.Equal(t, "2001:db8::8", f.connected.EndpointConfig.IPAMConfig.IPv6Address.String())
		})
	}
	path := "../../api/testdata/network-write-responses.json"
	actual, err := json.MarshalIndent(snapshots, "", "  ")
	require.NoError(t, err)
	if *updateNetworkFixtures {
		require.NoError(t, os.WriteFile(path, append(actual, '\n'), 0600))
	}
	expected, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(actual))
}
