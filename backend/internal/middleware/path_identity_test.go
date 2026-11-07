// Source: docklet-next/issues/264
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original route identity guard proofs over real Direct and Edge transports.

package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	requestDTO "github.com/florianfinn/docklet-next/types/request"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

// identityAgent records every request that reaches the agent.
type identityAgent struct {
	calls atomic.Int32
	mu    sync.Mutex
	paths []string
	query []string
}

func (a *identityAgent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.calls.Add(1)
	a.mu.Lock()
	a.paths = append(a.paths, r.URL.Path)
	a.query = append(a.query, r.URL.RawQuery)
	a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
}

func identityNotFound(c *echo.Context) error {
	return c.JSON(http.StatusNotFound, map[string]any{"success": false, "error": "API endpoint not found: " + c.Request().URL.Path})
}

// identityHub mounts the guard in front of the real environment middleware,
// which forwards to a real Direct HTTP agent or through a real Edge tunnel.
func identityHub(t *testing.T, mode string, agent *identityAgent) *echo.Echo {
	t.Helper()
	m := newTestEnvironmentMiddleware()
	switch mode {
	case "direct":
		server := httptest.NewServer(agent)
		t.Cleanup(server.Close)
		m.resolver = func(context.Context, string) (string, *string, bool, error) { return server.URL, nil, true, nil }
	case "edge":
		registry := edge.NewTunnelRegistry()
		connected := make(chan struct{}, 4)
		tunnelServer := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return "example", nil }, func(_ context.Context, _ string, online bool) {
			if online {
				connected <- struct{}{}
			}
		})
		tunnelServer.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return "example-key", nil })
		router := echo.New()
		router.GET("/api/tunnel/connect", tunnelServer.HandleConnect)
		manager := httptest.NewServer(router)
		client := edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: "example", EdgeReconnectInterval: 1}, agent)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { defer close(done); client.StartWithErrorChan(ctx, nil) }()
		t.Cleanup(func() { cancel(); registry.Unregister("example"); <-done; manager.Close() })
		select {
		case <-connected:
		case <-time.After(10 * time.Second):
			t.Fatal("Edge registration missing")
		}
		m.registry = registry
		m.resolver = func(context.Context, string) (string, *string, bool, error) { return "edge://example", nil, true, nil }
	}
	hub := servedRouter()
	hub.Use(PathIdentityGuard(identityNotFound))
	attachMiddleware(hub, m).Any("/environments/:id/*", func(c *echo.Context) error { return c.NoContent(418) })
	return hub
}

// Every crafted form is answered before the environment lookup, any hub check
// or forwarding; ordinary escapes still reach the served route on D and E.
func TestPathIdentityGuardOverRealTransports(t *testing.T) {
	refused := []string{
		"/api/environments/example/containers/%63ounts",
		"/api/environments/example/%63ontainers/counts",
		"/api/environments/%65xample/containers/counts",
		"/api/environments/example/containers/abc%2Fdef",
		"/api/environments/example/containers/abc%2fdef",
		"/api/environments/example/containers/abc%3Fx",
		"/api/environments/example/containers/abc%23x",
		"/api/environments/example/containers/abc%25x",
		"/api/environments/example/containers/abc%252F",
		"/api/environments/example/containers/abc%5Cx",
		"/api/environments/example/containers/%2e%2e",
		"/api/environments/example/containers/counts%20",
		"/api/environments/example/containers/counts%C2%A0",
		"/api/environments/example/containers/abc%0A",
		"/api/environments/example/containers/abc%09",
		"/api/environments/example/volumes/abc%FF",
		"/api/environments/example/volumes/abc%80",
		"/api/environments/example/volumes/abc%C3%28",
	}
	unmatched := []string{
		"/api/environments/example/containers/counts/",
		"/api/environments/example//containers/counts",
		"/api/environments/example/containers//counts",
		"/api/environments/example/containers/./counts",
		"/api/environments/example/containers/../images",
		"/api/environments/example/agent/restarts/R/",
		"/api/environments/example/vulnerabilities/runs/R/",
	}
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			agent := &identityAgent{}
			hub := identityHub(t, mode, agent)
			for _, target := range refused {
				rec := httptest.NewRecorder()
				hub.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
				require.Equal(t, http.StatusBadRequest, rec.Code, "%s: %s", target, rec.Body.String())
				var problem requestDTO.Problem
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem), target)
				require.Equal(t, requestDTO.PathEncoded, problem.Code, target)
				require.Equal(t, http.StatusBadRequest, problem.Status, target)
			}
			for _, target := range unmatched {
				rec := httptest.NewRecorder()
				hub.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
				require.Equal(t, http.StatusNotFound, rec.Code, "%s: %s", target, rec.Body.String())
				require.Contains(t, rec.Body.String(), "API endpoint not found", target)
			}
			require.Zero(t, agent.calls.Load(), "no refused request reaches the agent")

			// Ordinary escapes reach the agent with the decoded identity the hub checked.
			for _, tc := range []struct{ target, path, query string }{
				{"/api/environments/example/containers/counts", "/api/environments/0/containers/counts", ""},
				{"/api/environments/example/images/sha256%3A" + strings.Repeat("a", 64), "/api/environments/0/images/sha256:" + strings.Repeat("a", 64), ""},
				{"/api/environments/example/volumes/caf%C3%A9-data", "/api/environments/0/volumes/caf\u00e9-data", ""},
				{"/api/environments/example/volumes/%EF%BF%BD", "/api/environments/0/volumes/\uFFFD", ""},
				{"/api/environments/example/containers/counts?name=%2F%20x", "/api/environments/0/containers/counts", "name=%2F%20x"},
			} {
				before := agent.calls.Load()
				rec := httptest.NewRecorder()
				hub.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))
				require.Equal(t, http.StatusOK, rec.Code, "%s: %s", tc.target, rec.Body.String())
				require.Equal(t, before+1, agent.calls.Load(), "%s reaches the agent", tc.target)
				agent.mu.Lock()
				path, query := agent.paths[len(agent.paths)-1], agent.query[len(agent.query)-1]
				agent.mu.Unlock()
				require.Equal(t, tc.path, path, tc.target)
				require.Equal(t, tc.query, query, tc.target)
			}
		})
	}
}

// Paths outside the environment prefix are untouched by the guard.
func TestPathIdentityGuardIgnoresOtherPaths(t *testing.T) {
	hub := servedRouter()
	hub.Use(PathIdentityGuard(identityNotFound))
	hub.Any("/*", func(c *echo.Context) error { return c.NoContent(204) })
	for _, target := range []string{"/api/auth/%6Cogin", "/api/settings/", "/assets/a%20b.js"} {
		rec := httptest.NewRecorder()
		hub.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		require.Equal(t, http.StatusNoContent, rec.Code, target)
	}
}
