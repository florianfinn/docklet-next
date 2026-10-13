// Source: docklet-next/docs/design/core-adoption.md
// Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Original issue #8 Echo route surface test for both agent modes.

package bootstrap

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

const echoRouteHeader = "# Source: docklet-next/docs/design/core-adoption.md\n" +
	"# Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71\n" +
	"# Retrieved: 2026-10-13\n" +
	"# License: Apache-2.0\n" +
	"# Deviations: Echo route snapshot for issue #8.\n"

func TestAgentAndEdgeAgentEchoRoutes(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("testdata", "agent-echo-routes.txt"))
	require.NoError(t, err)
	for _, edgeMode := range []bool{false, true} {
		name := "agent"
		if edgeMode {
			name = "edge-agent"
		}
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{AgentMode: !edgeMode, EdgeAgent: edgeMode, AppUrl: "http://localhost:3552", Environment: config.AppEnvironmentTest}
			router, _ := newRouter(RouterParams{
				Context: t.Context(), Lifecycle: fxtest.NewLifecycle(t), Config: cfg,
				HandlerDeps: api.HandlerDeps{
					Project:   project.New(&project.ProjectService{}, nil),
					Container: container.New(&container.ContainerService{}, nil, nil, nil),
					System:    system.New(&system.SystemService{}, nil, nil, nil, nil),
				},
				AuthMiddleware: auth.NewAuthMiddleware(nil, cfg), TunnelRegistry: edge.NewTunnelRegistry(),
			})
			var routes []string
			for _, route := range router.Router().Routes() {
				routes = append(routes, route.Method+" "+route.Path)
			}
			slices.Sort(routes)
			actual := echoRouteHeader + strings.Join(routes, "\n") + "\n"
			if os.Getenv("UPDATE_AGENT_ECHO_ROUTES") == "1" {
				require.NoError(t, os.WriteFile(filepath.Join("testdata", "agent-echo-routes.txt"), []byte(actual), 0o644))
				return
			}
			require.Equal(t, string(want), actual)
		})
	}
}
