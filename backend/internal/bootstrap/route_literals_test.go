// Source: docklet-next/issues/264
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original structural proof that route literals use unreserved characters only.

package bootstrap

import (
	"regexp"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
)

// unreservedSegment is RFC 3986 unreserved; "." and ".." are dot segments, not literals.
var unreservedSegment = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)

// RouteLiteralViolations returns route literal segments outside the unreserved set.
func routeLiteralViolations(path string) []string {
	var bad []string
	for _, segment := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") || strings.HasPrefix(segment, "{") {
			continue
		}
		if !unreservedSegment.MatchString(segment) || segment == "." || segment == ".." {
			bad = append(bad, segment)
		}
	}
	return bad
}

// With literal segments unreserved only, a permitted escape can never form or
// hide a literal, so the refused-escape rule gives one route identity (#264).
func TestRouteLiteralSegmentsAreUnreserved(t *testing.T) {
	for _, mode := range []string{"hub", "agent", "edge-agent"} {
		t.Run(mode, func(t *testing.T) {
			cfg := &config.Config{AgentMode: mode == "agent", EdgeAgent: mode == "edge-agent", AppUrl: "http://localhost:3552", Environment: config.AppEnvironmentTest}
			deps := api.HandlerDeps{
				Project:   project.New(&project.ProjectService{}, nil),
				Container: container.New(&container.ContainerService{}, nil, nil, nil),
				System:    system.New(&system.SystemService{}, nil, nil, nil, nil),
			}
			if mode == "hub" {
				deps.Environment = environment.New(&environment.EnvironmentService{}, nil, nil, nil, cfg, nil)
			}
			router, _ := newRouter(RouterParams{Context: t.Context(), Lifecycle: fxtest.NewLifecycle(t), Config: cfg, HandlerDeps: deps,
				AuthMiddleware: auth.NewAuthMiddleware(nil, cfg), TunnelRegistry: edge.NewTunnelRegistry()})
			checked := 0
			for _, route := range router.Router().Routes() {
				// Only proxied environment routes share identity between hub and agent.
				if !strings.HasPrefix(route.Path, "/api/environments/") {
					continue
				}
				checked++
				require.Empty(t, routeLiteralViolations(route.Path), "%s %s", route.Method, route.Path)
			}
			require.Greater(t, checked, 50, "the router registers its environment routes")
		})
	}
}

func TestRouteLiteralViolationsFindsReservedLiterals(t *testing.T) {
	require.Empty(t, routeLiteralViolations("/api/environments/:id/containers/:containerId/start"))
	require.Equal(t, []string{"a b", "x%2Fy", ".."}, routeLiteralViolations("/api/a b/x%2Fy/../{id}/*"))
}
