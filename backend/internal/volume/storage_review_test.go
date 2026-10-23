// Source: docklet-next/docs/design/storage-workspace.md
// Revision: ce3c4ea4af5b0bc1cdb2bb287d30281bbe20f0a8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original R1 regression coverage for issue #179.
package volume

import (
	"encoding/json"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/base"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/volume"
	"github.com/stretchr/testify/require"
)

// Reviewer test: every non-running container state must count as usage, in
// both the plan and the execution re-check, over Direct and the real Edge tunnel.
func TestReview179NonRunningStatesBlockPlanAndExecute(t *testing.T) {
	states := []container.ContainerState{container.StateCreated, container.StateExited, container.StatePaused, container.StateDead, container.StateRestarting}
	for _, transport := range []string{"direct", "edge"} {
		for _, state := range states {
			t.Run(transport+"/"+string(state), func(t *testing.T) {
				f := &pruneDockerFixture{volumes: []volume.Volume{pruneVolume("held"), pruneVolume("free")}}
				f.containers = []container.Summary{pruneConsumer("held", state)}
				call := pruneTransport(t, transport, pruneRouter(t, f.service(t)))
				status, body := call("GET", "/api/environments/0/volumes/prune", "all=true", nil)
				require.Equal(t, 200, status, string(body))
				var plan base.ApiResponse[volumetypes.PrunePlan]
				require.NoError(t, json.Unmarshal(body, &plan))
				names := []string{}
				for _, c := range plan.Data.Candidates {
					names = append(names, c.Name)
				}
				require.Equal(t, []string{"free"}, names)

				// A crafted request naming the held volume must still be refused.
				request, err := json.Marshal(volumetypes.PruneRequest{Candidates: []volumetypes.PruneCandidate{{Name: "held", CreatedAt: pruneCreated}, {Name: "free", CreatedAt: pruneCreated}}})
				require.NoError(t, err)
				status, body = call("POST", "/api/environments/0/volumes/prune", "", request)
				require.Equal(t, 200, status, string(body))
				var result base.ApiResponse[volumetypes.PruneResult]
				require.NoError(t, json.Unmarshal(body, &result))
				require.Equal(t, []string{"free"}, result.Data.Removed)
				require.Equal(t, []volumetypes.PruneSkipped{{Name: "held", Reason: "in-use"}}, result.Data.Skipped)
				require.Equal(t, []string{"free"}, f.removed)
			})
		}
	}
}

// Reviewer test: the static Edge routes must resolve to their own commands and
// never to the {volumeName} wildcard neighbours.
func TestReview179EdgeStorageCommandNames(t *testing.T) {
	for _, tc := range []struct{ method, path, command string }{
		{"GET", "/api/environments/7/volumes/prune", "volume.prune.plan"},
		{"POST", "/api/environments/7/volumes/prune", "volume.prune"},
		{"GET", "/api/environments/7/volumes/prune-me", "volume.inspect"},
		{"POST", "/api/environments/7/volumes/data/rename", "volume.rename"},
		{"POST", "/api/environments/7/volumes/data/backups/upload", "volume.backup.import"},
		{"GET", "/api/environments/7/volumes/data/workspace/file/download", "volume.workspace.download"},
		{"PUT", "/api/environments/7/projects/p/workspace", "project.workspace.update"},
	} {
		got, ok := edge.ResolveEdgeCommandName(tc.method, tc.path, false).Get()
		require.True(t, ok, tc.path)
		require.Equal(t, tc.command, got, tc.method+" "+tc.path)
	}
	_, ok := edge.ResolveEdgeCommandName("POST", "/api/environments/7/volumes/backups/b/upload", false).Get()
	require.False(t, ok, "removed S3 upload route must not resolve")
}

// Reviewer test: a real Edge rename that passes validation reaches the agent's
// rename service (protected backup-volume rule), proving the body is forwarded.
func TestReview179EdgeRenameForwardsBody(t *testing.T) {
	svc := &VolumeService{backupVolumeName: "docklet-backups"}
	call := pruneTransport(t, "edge", pruneRouter(t, svc))
	status, body := call("POST", "/api/environments/0/volumes/data/rename", "", []byte(`{"name":"docklet-backups"}`))
	require.NotEqual(t, 200, status, string(body))
	require.Contains(t, string(body), "internal volumes cannot be renamed", string(body))
}
