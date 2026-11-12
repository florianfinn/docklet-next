// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestBindEntryMatrixBeforeDocker(t *testing.T) {
	key := volumetypes.BindKey("/srv/app/")
	for _, mode := range []string{"hub-local", "agent-direct", "agent-edge", "hub-direct", "hub-edge"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
			defer server.Close()
			s := NewVolumeService(nil, docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeServiceTestDockerClientInternal(t, server)), nil, nil, nil, nil, nil, nil, nil, nil)
			call := maintenanceReadTransport(t, mode, bindTestRouter(t, s))
			for _, row := range []struct{ method, suffix, body string }{
				{"GET", "", ""}, {"DELETE", "", ""}, {"GET", "/usage", ""}, {"POST", "/rename", `{"name":"copy"}`},
				{"GET", "/workspace", ""}, {"GET", "/workspace/file", ""}, {"GET", "/workspace/file/download", ""},
				{"GET", "/backups", ""}, {"POST", "/backups", `{}`}, {"GET", "/backup-policy", ""}, {"PUT", "/backup-policy", `{"policies":[]}`},
				{"POST", "/backups/snapshot/restore", `{}`}, {"POST", "/backups/snapshot/restore-files", `{"paths":[],"selectAll":true,"search":""}`},
				{"POST", "/backups/upload", `{"uploadId":"upload"}`},
			} {
				status, body := call(row.method, "/api/environments/0/volumes/"+key+row.suffix, "relativePath=app.conf", []byte(row.body))
				require.Equal(t, 400, status, row.method+row.suffix+": "+string(body))
				require.Contains(t, string(body), "volume.bind.invalid-key")
			}
			for _, row := range []struct{ path, body string }{
				{"/volumes", fmt.Sprintf(`{"name":%q}`, key)},
				{"/volumes/app/rename", fmt.Sprintf(`{"name":%q}`, key)},
				{"/volumes/prune", fmt.Sprintf(`{"candidates":[{"name":%q,"createdAt":"2026-11-01T00:00:00Z"}]}`, key)},
				{"/volumes/app/backups/snapshot/restore", fmt.Sprintf(`{"mode":"new","newVolumeName":%q}`, key)},
				{"/volumes/app/backups/upload", fmt.Sprintf(`{"uploadId":"upload","mode":"new","newVolumeName":%q}`, key)},
			} {
				status, body := call("POST", "/api/environments/0"+row.path, "", []byte(row.body))
				require.Equal(t, 400, status, row.path+": "+string(body))
				require.Contains(t, string(body), "volume.bind.invalid-key")
			}
			require.Zero(t, calls.Load())
		})
	}
}
