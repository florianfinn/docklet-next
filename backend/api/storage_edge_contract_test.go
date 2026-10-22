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
	expectedCommands := map[string]string{
		"get-project-workspace":           "project.workspace.list",
		"update-project-workspace":        "project.workspace.update",
		"get-project-workspace-file":      "project.workspace.read_file",
		"download-project-workspace-file": "project.workspace.download",
		"list-volumes":                    "volume.list",
		"create-volume":                   "volume.create",
		"delete-volume-backup":            "volume.backup.delete",
		"download-volume-backup":          "volume.backup.download",
		"list-backup-files":               "volume.backup.files",
		"browse-volume-backup-files":      "volume.backup.files_browse",
		"backup-has-path":                 "volume.backup.has_path",
		"get-volume-usage-counts":         "volume.counts",
		"plan-volume-prune":               "volume.prune.plan",
		"prune-volumes":                   "volume.prune",
		"get-volume-sizes":                "volume.sizes",
		"remove-volume":                   "volume.delete",
		"get-volume":                      "volume.inspect",
		"get-volume-backup-policy":        "volume.backup_policy.get",
		"update-volume-backup-policy":     "volume.backup_policy.update",
		"list-volume-backups":             "volume.backup.list",
		"create-volume-backup":            "volume.backup.create",
		"upload-volume-backup":            "volume.backup.import",
		"restore-volume-backup":           "volume.backup.restore",
		"restore-volume-backup-files":     "volume.backup.restore_files",
		"rename-volume":                   "volume.rename",
		"get-volume-usage":                "volume.usage",
		"get-volume-workspace":            "volume.workspace.list",
		"update-volume-workspace":         "volume.workspace.update",
		"get-volume-workspace-file":       "volume.workspace.read_file",
		"download-volume-workspace-file":  "volume.workspace.download",
	}
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
				require.Contains(t, expectedCommands, op.OperationID)
				require.Equal(t, expectedCommands[op.OperationID], command, "%s %s (%s)", method, path, op.OperationID)
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
