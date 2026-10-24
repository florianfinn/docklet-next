// Source: docklet-next/issues/217
// Revision: 1c1597f131fdbd87d6043a44fa32b7d2149388f3
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Real project update name clash through HTTP and Edge; reuse storage transport fixture pattern.

package project

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	projectdetails "github.com/florianfinn/docklet-next/backend/internal/project/children/details"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestProjectUpdateVolumeConflictThroughDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", namespaceVolumeCompose)
			before, err := os.ReadFile(filepath.Join(row.Path, "compose.yaml"))
			require.NoError(t, err)
			reads := &namespaceVolumeReads{source: "alpha_data", target: "gamma_data", exists: map[string]bool{"alpha_data": true, "gamma_data": true}}
			projectConflictDocker(t, svc, reads)
			svc.details = projectdetails.New(svc.db, svc.dockerService, svc.imageService, svc.settingsService)
			call := projectConflictTransport(t, mode, projectConflictRouter(t, svc))
			status, body := call("PUT", "/api/environments/0/projects/example-project", "", []byte(`{"name":"gamma"}`))
			require.Equal(t, http.StatusConflict, status, string(body))
			var problem volumetypes.ExistsError
			require.NoError(t, json.Unmarshal(body, &problem))
			require.Equal(t, "volume.exists", problem.Code)
			require.Equal(t, http.StatusConflict, problem.Status)
			require.Equal(t, "A volume with this name already exists", problem.Detail)
			// The preflight conflict cannot rename files, change the database or write Docker state.
			stored, err := svc.GetProjectFromDatabaseByID(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, "alpha", stored.Name)
			require.Equal(t, row.Path, stored.Path)
			after, err := os.ReadFile(filepath.Join(row.Path, "compose.yaml"))
			require.NoError(t, err)
			require.Equal(t, before, after)
			_, err = os.Stat(filepath.Join(root, "gamma"))
			require.True(t, os.IsNotExist(err))
			reads.mu.Lock()
			defer reads.mu.Unlock()
			require.Contains(t, reads.names, "gamma_data")
			require.Zero(t, reads.writes)
		})
	}
}

// Compose negotiates its own read client in addition to the service SDK client.
// Both use this synthetic daemon; every mutating request is rejected and counted.
func projectConflictDocker(t *testing.T, svc *ProjectService, reads *namespaceVolumeReads) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.mu.Lock()
		defer reads.mu.Unlock()
		if r.URL.Path == "/_ping" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			w.Header().Set("API-Version", "1.51")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodGet {
			reads.writes++
			http.Error(w, "read only", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, name, ok := strings.Cut(r.URL.Path, "/volumes/"); ok {
			reads.names = append(reads.names, name)
			if !reads.exists[name] {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Name": name, "Driver": "local"})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		// Self-container metadata is optional and has no synthetic mount mapping.
		http.NotFound(w, r)
	}))
	t.Cleanup(func() { server.CloseClientConnections(); server.Close(); require.Zero(t, reads.writes) })
	cfg := &config.Config{DockerHost: strings.Replace(server.URL, "http://", "tcp://", 1)}
	svc.dockerService = docker.NewDockerClientService(t.Context(), nil, cfg, nil).WithClient(newTestDockerClient(t, server))
}

func projectConflictRouter(t *testing.T, svc *ProjectService) http.Handler {
	t.Helper()
	router := echo.New()
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			grants := authz.NewPermissionSet()
			grants.AddEnv("0", authz.PermProjectsUpdate)
			ctx := context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, grants)
			ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &user.Actor{ID: "example-user", Username: "example-user"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	config := huma.DefaultConfig("project", "test")
	config.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(typ reflect.Type, hint string) string {
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		return strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(typ.PkgPath()) + "_" + huma.DefaultSchemaNamer(typ, hint)
	})
	api := humaecho.NewWithGroup(router, router.Group("/api"), config)
	RegisterProjects(api, svc, nil, handlerutil.NewActivityAppContext(t.Context()))
	return router
}

type projectConflictCall func(method, path, query string, body []byte) (int, []byte)

// This fixture exercises the real command client, WebSocket tunnel, command
// validation and local Huma handler, with no fabricated producer JSON.
func projectConflictTransport(t *testing.T, mode string, handler http.Handler, contentTypes ...string) projectConflictCall {
	t.Helper()
	contentType := "application/json"
	if len(contentTypes) > 0 {
		contentType = contentTypes[0]
	}
	if mode == "direct" {
		return func(method, path, query string, body []byte) (int, []byte) {
			if query != "" {
				path += "?" + query
			}
			req := httptest.NewRequest(method, path, bytes.NewReader(body))
			req.Header.Set("Content-Type", contentType)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			return response.Code, response.Body.Bytes()
		}
	}
	registry := projectConflictTunnelRegistry(t, handler)
	tunnel, ok := registry.Get("example-storage").Get()
	require.True(t, ok)
	return func(method, path, query string, body []byte) (int, []byte) {
		status, _, response, err := edge.ProxyRequest(t.Context(), tunnel, method, path, query, map[string]string{"Content-Type": contentType}, body)
		require.NoError(t, err)
		return status, response
	}
}

func projectConflictTunnelRegistry(t *testing.T, handler http.Handler) *edge.TunnelRegistry {
	t.Helper()
	registry := edge.NewTunnelRegistry()
	connected := make(chan struct{})
	var once sync.Once
	server := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return "example-storage", nil }, func(_ context.Context, _ string, online bool) {
		if online {
			once.Do(func() { close(connected) })
		}
	})
	router := echo.New()
	router.GET("/api/tunnel/connect", server.HandleConnect)
	manager := httptest.NewServer(router)
	t.Cleanup(manager.Close)
	client := edge.NewTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: uuid.New().String()}, handler)
	life, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); client.StartWithErrorChan(life, nil) }()
	t.Cleanup(func() {
		cancel()
		registry.Unregister("example-storage")
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("tunnel cleanup failed")
		}
	})
	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("tunnel registration barrier failed")
	}
	return registry
}
