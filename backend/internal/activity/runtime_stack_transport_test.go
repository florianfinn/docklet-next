// Source: docklet-next/issues/316
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original public local/Direct/Edge stop and unchanged down producer/consumer proof.
package activity_test

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	dto "github.com/florianfinn/docklet-next/types/project"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestStackStopAndDownRealTransportsAndDefinitionListing(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		for _, operation := range []string{"stop", "down"} {
			t.Run(mode+"/"+operation, func(t *testing.T) {
				hub := activationApp(t, false, "")
				executor, env := hub, "0"
				if mode != "local" {
					executor = activationApp(t, true, uuid.NewString())
					env = activationRemote(t, hub, executor, mode)
				}
				executor.services.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n  worker:\n    image: registry.example.com/worker:1\n")
				executor.services.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
				executor.services.http.setComposeContainer(true)
				path := "/api/environments/" + env + "/projects/example-project/" + operation
				status, body := activationCall(t, hub, path, uuid.NewString(), "respond-async", nil)
				require.Equal(t, 202, status, string(body))
				run := activationRunID(t, body)
				activationJoinRun(t, hub, executor, env, run)
				var row activity.RuntimeReceipt
				require.NoError(t, executor.services.db.First(&row, "run_id=?", run).Error)
				require.Equal(t, "success", *row.ExecutorState)
				stop, remove := 0, 0
				for _, event := range executor.services.http.snapshot() {
					if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/stop") {
						stop++
					}
					if event.Method == http.MethodDelete && strings.Contains(event.Path, "/containers/") {
						remove++
					}
				}
				require.Equal(t, 1, stop)
				if operation == "stop" {
					require.Zero(t, remove)
					require.Equal(t, new("stop-project"), row.OperationID)
				} else {
					require.Equal(t, 1, remove)
				}
				if operation == "down" {
					response, err := hub.server.Client().Get(hub.server.URL + "/api/environments/" + env + "/projects")
					require.NoError(t, err)
					defer response.Body.Close()
					body, err := io.ReadAll(response.Body)
					require.NoError(t, err)
					require.Equal(t, 200, response.StatusCode, string(body))
					var listed struct{ Data []dto.Details }
					require.NoError(t, json.Unmarshal(body, &listed))
					require.Len(t, listed.Data, 1)
					require.Len(t, listed.Data[0].RuntimeServices, 2)
					for _, service := range listed.Data[0].RuntimeServices {
						require.Equal(t, dto.ServiceNotCreated, service.Status)
						require.Empty(t, service.ContainerID)
					}
				}
			})
		}
	}
}
