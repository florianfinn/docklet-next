// Source: docklet-next/issues/264
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original shared HTTP/gRPC listener route identity proof.

package bootstrap

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	tunnelpb "github.com/florianfinn/docklet-next/backend/proto/tunnel/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
)

// The production hub handler shares one TLS HTTP/2 listener between Echo and
// the tunnel gRPC server; environment paths must reach the guarded router
// whatever their content type, while tunnel endpoints stay gRPC.
func TestSharedListenerServesEnvironmentPathsThroughTheGuard(t *testing.T) {
	cfg := &config.Config{AppUrl: "https://example.com", Environment: config.AppEnvironmentTest}
	deps := api.HandlerDeps{
		Project:     project.New(&project.ProjectService{}, nil),
		Container:   container.New(&container.ContainerService{}, nil, nil, nil),
		System:      system.New(&system.SystemService{}, nil, nil, nil, nil),
		Environment: environment.New(&environment.EnvironmentService{}, nil, nil, nil, cfg, nil),
	}
	router, tunnel := newRouter(RouterParams{Context: t.Context(), Lifecycle: fxtest.NewLifecycle(t), Config: cfg, HandlerDeps: deps, AuthMiddleware: auth.NewAuthMiddleware(nil, cfg), TunnelRegistry: edge.NewTunnelRegistry()})
	require.NotNil(t, tunnel)
	handler, grpcServer := configureTunnelServerInternal(t.Context(), cfg, router, tunnel, nil, "")
	t.Cleanup(grpcServer.Stop)
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	post := func(t *testing.T, path string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+path, http.NoBody)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/grpc")
		response, err := server.Client().Do(req)
		require.NoError(t, err)
		_, _ = io.ReadAll(response.Body)
		require.NoError(t, response.Body.Close())
		require.Equal(t, 2, response.ProtoMajor)
		return response
	}
	echoStatus := func(path string) int {
		req := httptest.NewRequest(http.MethodPost, path, http.NoBody)
		req.Header.Set("Content-Type", "application/grpc")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/environments/example/containers/%63ounts", http.StatusBadRequest},
		{"/api/environments/example/containers/counts%20", http.StatusBadRequest},
		{"/api/environments/example/containers//counts", http.StatusNotFound},
		{"/api/environments/example/containers/../images", http.StatusNotFound},
		{"/api/environments/example" + tunnelpb.TunnelService_Connect_FullMethodName, -1},
		{"/api/environments/example/api/tunnel/connect", -1},
	} {
		t.Run(tc.path, func(t *testing.T) {
			want := tc.want
			if want < 0 {
				want = echoStatus(tc.path)
			}
			response := post(t, tc.path)
			require.Equal(t, want, response.StatusCode, "the shared listener answers like the guarded router")
			require.Empty(t, response.Trailer.Get("Grpc-Status"), "no environment path reaches the tunnel gRPC server")
		})
	}
	for _, path := range []string{"/api/tunnel/connect", tunnelpb.TunnelService_Connect_FullMethodName} {
		t.Run("tunnel "+path, func(t *testing.T) {
			response := post(t, path)
			require.NotEmpty(t, response.Header.Get("Grpc-Status")+response.Trailer.Get("Grpc-Status"), "tunnel endpoints stay gRPC")
		})
	}
}
