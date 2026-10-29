// Source: docklet-next/issues/245
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original health lifecycle wiring test for hub, agent and Edge agent routers.

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
)

func TestHealthFollowsAppLifecycleInAgentModes(t *testing.T) {
	for _, edgeMode := range []bool{false, true} {
		name := "agent"
		if edgeMode {
			name = "edge-agent"
		}
		t.Run(name, func(t *testing.T) {
			appCtx, cancelApp := context.WithCancel(t.Context())
			defer cancelApp()
			cfg := &config.Config{AgentMode: !edgeMode, EdgeAgent: edgeMode, AppUrl: "http://localhost:3552", Environment: config.AppEnvironmentTest}
			router, _ := newRouter(RouterParams{
				Context: appCtx, Lifecycle: fxtest.NewLifecycle(t), Config: cfg,
				HandlerDeps: api.HandlerDeps{
					Project:   project.New(&project.ProjectService{}, nil),
					Container: container.New(&container.ContainerService{}, nil, nil, nil),
					System:    system.New(&system.SystemService{}, nil, nil, nil, nil),
				},
				AuthMiddleware: auth.NewAuthMiddleware(nil, cfg), TunnelRegistry: edge.NewTunnelRegistry(),
			})
			status := func(method string) int {
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), method, "/api/health", nil))
				return recorder.Code
			}
			require.Equal(t, http.StatusOK, status(http.MethodGet))
			require.Equal(t, http.StatusOK, status(http.MethodHead))
			cancelApp()
			require.Equal(t, http.StatusServiceUnavailable, status(http.MethodGet))
			require.Equal(t, http.StatusServiceUnavailable, status(http.MethodHead))
		})
	}
}
