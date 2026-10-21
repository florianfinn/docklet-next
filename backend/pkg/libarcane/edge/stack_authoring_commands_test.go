// Source: docklet-next/docs/design/stack-authoring.md
// Revision: b0f06b6a169842c03597bbb9a7c177ef75d4eea1
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original Edge coverage guard for the stack editor routes (#152).

package edge

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Every request the production stack editor sends must cross the Edge tunnel;
// Direct hosts forward plain HTTP, so a gap here only shows on Edge.
func TestStackAuthoringRoutesResolveOnEdge(t *testing.T) {
	t.Parallel()
	routes := []struct{ method, path, command string }{
		{"POST", "/api/environments/0/projects", "project.create"},
		{"GET", "/api/environments/0/projects/p1/compose", "project.compose"},
		{"PUT", "/api/environments/0/projects/p1", "project.update"},
		{"PATCH", "/api/environments/0/projects/p1/tags", "project.tags.update"},
		{"POST", "/api/environments/0/projects/p1/up", "project.up"},
		{"POST", "/api/environments/0/system/convert", "system.convert"},
	}
	for _, route := range routes {
		command, ok := ResolveEdgeCommandName(route.method, route.path, false).Get()
		require.True(t, ok, "%s %s", route.method, route.path)
		require.Equal(t, route.command, command)
		require.True(t, ValidateEdgeCommand(route.command, route.method, route.path, false), "agent accepts %s", route.command)
	}
}
