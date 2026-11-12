// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original restart route registration boundary tests.

package api

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAgentRestartRegisteredOnlyInAgentModes(t *testing.T) {
	for _, cfg := range []*config.Config{{}, {AgentMode: true}, {EdgeAgent: true}} {
		schema := setupAPIForSpecMode(cfg).OpenAPI()
		route := schema.Paths["/system/agent/restart"]
		if !cfg.AgentMode && !cfg.EdgeAgent {
			require.Nil(t, route)
			continue
		}
		require.NotNil(t, route)
		require.NotNil(t, route.Post)
		command, ok := edge.ResolveEdgeCommandName("POST", "/api/system/agent/restart", false).Get()
		require.True(t, ok)
		require.Equal(t, "system.agent.restart", command)
	}
}
