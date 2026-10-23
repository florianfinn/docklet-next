// Source: docklet-next/issues/194
// Revision: fdbb8c1fae620d9f97a485983cdaabb3fa53bca2
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original real producer not-found transport regressions.

package volume

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

// Hub modes pass through the production environment proxy, using a real HTTP
// agent or the actual WebSocket tunnel. Local modes use shared route registration.
func maintenanceReadTransport(t *testing.T, mode string, handler http.Handler) pruneCall {
	t.Helper()
	if mode == "hub-local" || mode == "agent-direct" {
		return pruneTransport(t, "direct", handler)
	}
	if mode == "agent-edge" {
		return pruneTransport(t, "edge", handler)
	}
	registry := edge.NewTunnelRegistry()
	apiURL := "edge://example-storage"
	var httpClient *http.Client
	if mode == "hub-direct" {
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		apiURL, httpClient = server.URL, server.Client()
	} else {
		registry = pruneTunnelRegistry(t, handler)
	}
	router := echo.New()
	router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id",
		func(context.Context, string) (string, *string, bool, error) {
			token := "example"
			return apiURL, &token, true, nil
		},
		nil, nil, registry, nil, middleware.EnvironmentProxySecurity{HTTPClient: httpClient}))
	router.GET("/api/environments/:id/volumes/missing", func(c *echo.Context) error { return c.NoContent(500) })
	router.GET("/api/environments/:id/volumes/missing/usage", func(c *echo.Context) error { return c.NoContent(500) })
	call := pruneTransport(t, "direct", router)
	return func(method, path, query string, body []byte) (int, []byte) {
		return call(method, strings.Replace(path, "/environments/0/", "/environments/example-storage/", 1), query, body)
	}
}

func TestMissingVolumeUsageProblemThroughHubAndAgents(t *testing.T) {
	for _, mode := range []string{"hub-local", "agent-direct", "agent-edge", "hub-direct", "hub-edge"} {
		t.Run(mode, func(t *testing.T) {
			var status atomic.Int32
			status.Store(404)
			engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.True(t, strings.HasSuffix(r.URL.Path, "/volumes/missing"), r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(int(status.Load()))
				_, _ = w.Write([]byte(`{"message":"volume unavailable"}`))
			}))
			t.Cleanup(engine.Close)
			svc := &VolumeService{dockerService: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeServiceTestDockerClientInternal(t, engine))}
			call := maintenanceReadTransport(t, mode, pruneRouter(t, svc))
			get := func(suffix string, want int) huma.ErrorModel {
				code, body := call("GET", "/api/environments/0/volumes/missing"+suffix, "", nil)
				require.Equal(t, want, code, string(body))
				var problem huma.ErrorModel
				require.NoError(t, json.Unmarshal(body, &problem))
				require.Equal(t, want, problem.Status)
				return problem
			}
			detail := get("", 404)
			usage := get("/usage", 404)
			require.Equal(t, detail, usage, "missing usage has the same problem shape and detail as get-volume")
			status.Store(500)
			failed := get("/usage", 500)
			require.Equal(t, "Internal Server Error", failed.Title)
			require.Contains(t, failed.Detail, "Failed to get volume usage")
		})
	}
}
