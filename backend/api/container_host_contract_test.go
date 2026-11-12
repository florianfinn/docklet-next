// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original shared M14 schema registration guard; S2 agent restart receiver mode boundary.

package api

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestContainerHostContractSchemas(t *testing.T) {
	api := SetupAPIForSpec()
	for _, name := range []string{"ContainerOriginAssignment", "ContainerRecreateRequest", "ContainerProblem", "ContainerShellQuery", "ContainerShellClose", "ContainerShellResize", "ContainerShellExit", "EnvironmentAgentRestartResponse", "EnvironmentAgentRestartRun", "EnvironmentHostProblem"} {
		require.Contains(t, api.OpenAPI().Components.Schemas.Map(), name)
	}
}

func TestContainerHostM14RouteModes(t *testing.T) {
	for _, mode := range []struct {
		name string
		cfg  *config.Config
		hub  bool
	}{{"hub", &config.Config{}, true}, {"agent", &config.Config{AgentMode: true}, false}, {"edge", &config.Config{EdgeAgent: true}, false}} {
		t.Run(mode.name, func(t *testing.T) {
			api := setupAPIForSpecMode(mode.cfg)
			paths := api.OpenAPI().Paths
			origin := paths["/environments/{id}/containers/{containerId}/origin"]
			if mode.hub {
				require.NotNil(t, origin)
				require.NotNil(t, origin.Put)
				require.NotNil(t, origin.Delete)
			} else {
				require.Nil(t, origin)
			}
			for _, path := range []string{"/environments/{id}/containers/{containerId}/recreate", "/environments/{id}/containers/{containerId}/redeploy", "/environments/{id}/projects/{projectId}/stop"} {
				require.NotNil(t, paths[path], path)
				require.NotNil(t, paths[path].Post, path)
			}
			restart := paths["/system/agent/restart"]
			if mode.hub {
				require.Nil(t, restart)
			} else {
				require.NotNil(t, restart)
				require.NotNil(t, restart.Post)
			}
		})
	}
}
