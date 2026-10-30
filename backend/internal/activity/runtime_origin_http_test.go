// Source: docklet-next/issues/314
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original origin assignment, removal and late-completion API guards.

package activity_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOriginAssignmentAPIRefusesPartialComposeStandalone(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			hub := activationApp(t, false, "")
			executor, env := hub, "0"
			if mode != "local" {
				executor = activationApp(t, true, uuid.NewString())
				env = activationRemote(t, hub, executor, mode)
			}
			executor.services.http.setLabels(map[string]string{"com.docker.compose.project": ""})
			path := hub.server.URL + "/api/environments/" + env + "/containers/" + runtimeContainerID + "/origin"
			call := func(body string) (int, string) {
				request, err := http.NewRequest(http.MethodPut, path, bytes.NewBufferString(body))
				require.NoError(t, err)
				request.Header.Set("Content-Type", "application/json")
				response, err := hub.server.Client().Do(request)
				require.NoError(t, err)
				defer response.Body.Close()
				data, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				return response.StatusCode, string(data)
			}
			status, body := call(`{"origin":"standalone","dockerId":"` + runtimeContainerID + `"}`)
			require.Equal(t, 409, status, body)
			require.Contains(t, body, "container.origin-locked")
			status, body = call(`{"origin":"unraid","dockerId":"` + runtimeContainerID + `"}`)
			require.Equal(t, 200, status, body)
			require.Contains(t, body, `"originSource":"manual"`)
			status, body = call(`{"origin":"unraid","dockerId":"different"}`)
			require.Equal(t, 409, status, body)
		})
	}
}
func TestOriginRemovalAndReregistrationCannotResurrectAssignment(t *testing.T) {
	hub, executor := activationApp(t, false, ""), activationApp(t, true, uuid.NewString())
	environment := activationRemote(t, hub, executor, "direct")
	store := &origin.Store{DB: hub.services.db.DB, LocalRegistration: "hub"}
	id := origin.Identity{ID: "old", Name: "app", Labels: map[string]string{"com.docker.compose.service": "app"}}
	require.NoError(t, store.Assign(t.Context(), environment, id, dto.OriginAssignment{Origin: dto.OriginStandalone, DockerID: "old"}, "operator"))
	require.NoError(t, hub.env.DeleteEnvironment(t.Context(), environment, nil, nil))
	replacement := activationApp(t, true, uuid.NewString())
	registration := activationRemote(t, hub, replacement, "direct", uuid.NewString())
	require.NotEqual(t, environment, registration)
	require.NoError(t, origin.Move(hub.services.db.DB, environment, &dto.RecreateResult{OldID: "old", NewID: "new", OldName: "app", NewName: "renamed"}))
	var rows []origin.Override
	require.NoError(t, hub.services.db.Find(&rows).Error)
	require.Empty(t, rows)
	body, err := json.Marshal(rows)
	require.NoError(t, err)
	require.JSONEq(t, "[]", string(body))
}
