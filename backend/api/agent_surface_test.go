// Source: docklet-next/docs/design/core-adoption.md
// Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Original issue #8 agent route contract test.

package api

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"
)

const routeSnapshotHeader = "# Source: docklet-next/docs/design/core-adoption.md\n" +
	"# Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71\n" +
	"# Retrieved: 2026-10-13\n" +
	"# License: Apache-2.0\n" +
	"# Deviations: Agent route snapshot for issue #8.\n"

func TestAgentAndEdgeAgentRouteAllowlist(t *testing.T) {
	wantModules := []string{
		"container", "diagnostics", "health", "image", "image-update",
		"network", "port", "project", "settings", "system", "updater", "upload",
		"version", "volume", "vulnerability",
	}
	wantRoutes, err := os.ReadFile(filepath.Join("testdata", "agent-routes.txt"))
	require.NoError(t, err)
	for _, mode := range []struct {
		name string
		cfg  *config.Config
	}{
		{"agent", &config.Config{AgentMode: true}},
		{"edge-agent", &config.Config{EdgeAgent: true}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			modules := registeredModulesForMode(mode.cfg)
			names := make([]string, 0, len(modules))
			for _, module := range modules {
				names = append(names, module.name)
			}
			slices.Sort(names)
			require.Equal(t, wantModules, names)

			api := setupAPIForSpecMode(mode.cfg)
			var routes []string
			for path, item := range api.OpenAPI().Paths {
				for _, route := range []struct {
					method  string
					present bool
				}{
					{"DELETE", item.Delete != nil}, {"GET", item.Get != nil},
					{"HEAD", item.Head != nil}, {"PATCH", item.Patch != nil},
					{"POST", item.Post != nil}, {"PUT", item.Put != nil},
				} {
					if route.present {
						routes = append(routes, route.method+" "+path)
					}
				}
			}
			slices.Sort(routes)
			actual := strings.Join(routes, "\n") + "\n"
			if os.Getenv("UPDATE_AGENT_ROUTE_SNAPSHOT") == "1" {
				require.NoError(t, os.WriteFile(filepath.Join("testdata", "agent-routes.txt"), []byte(routeSnapshotHeader+actual), 0o644))
				return
			}
			require.Equal(t, routeSnapshotHeader+actual, string(wantRoutes))
		})
	}
}
