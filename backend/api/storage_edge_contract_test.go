// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 60d2471b62b12e1af64c54060a50102a2e39d1c1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original exhaustive storage route coverage for issue #179.
package api

import (
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/stretchr/testify/require"
)

func TestStorageOperationsHaveEdgeCommandsAndAgentRoutes(t *testing.T) {
	hub := SetupAPIForSpec()
	for _, cfg := range []*config.Config{{AgentMode: true}, {EdgeAgent: true}} {
		agent := setupAPIForSpecMode(cfg)
		count := 0
		for path, item := range hub.OpenAPI().Paths {
			if !strings.HasPrefix(path, "/environments/{id}/volumes") && !strings.Contains(path, "/workspace") {
				continue
			}
			for method, op := range storageOperations(item) {
				if op == nil {
					continue
				}
				count++
				actual := "/api" + path
				for _, param := range []string{"id", "volumeName", "projectId", "backupId"} {
					actual = strings.ReplaceAll(actual, "{"+param+"}", "example")
				}
				command, ok := edge.ResolveEdgeCommandName(method, actual, false).Get()
				require.True(t, ok, "%s %s (%s)", method, path, op.OperationID)
				require.True(t, edge.ValidateEdgeCommand(command, method, actual, false), op.OperationID)
				agentItem := agent.OpenAPI().Paths[path]
				require.NotNil(t, agentItem, op.OperationID)
				require.NotNil(t, storageOperations(agentItem)[method], op.OperationID)
			}
		}
		require.Equal(t, 30, count, "every current volume and project workspace operation must be checked")
	}
	require.False(t, edge.ValidateEdgeCommand("volume.backup.upload", "POST", "/api/environments/0/volumes/backups/example/upload", false))
}

func storageOperations(item *huma.PathItem) map[string]*huma.Operation {
	return map[string]*huma.Operation{"GET": item.Get, "POST": item.Post, "PUT": item.Put, "PATCH": item.Patch, "DELETE": item.Delete}
}
