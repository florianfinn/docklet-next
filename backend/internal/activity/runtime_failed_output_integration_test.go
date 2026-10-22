// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Failed SDK workflow diagnostics through production local, Direct and Edge routes.

package activity_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeFailedComposeOutputThroughReceipts(t *testing.T) {
	for _, transport := range []string{"local", "direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			hub := activationApp(t, false, "")
			executor, env := hub, "0"
			if transport != "local" {
				executor = activationApp(t, true, uuid.NewString())
				env = activationRemote(t, hub, executor, transport)
			}
			suffix, _ := activationOperation(executor.services, activity.RuntimeDeployProject)
			const diagnostic = "manifest for registry.example.com/app:1 not found: manifest unknown"
			executor.services.http.mu.Lock()
			executor.services.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/create") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"message": diagnostic}))
					return true
				}
				return false
			}
			executor.services.http.mu.Unlock()
			path, key := "/api/environments/"+env+suffix, uuid.NewString()
			body := []byte(`{"pullPolicy":"always"}`)
			status, response := activationCall(t, hub, path, key, "respond-async", body)
			require.Equal(t, http.StatusAccepted, status, string(response))
			run := activationRunID(t, response)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			if env != "0" {
				require.NoError(t, hub.remote.WaitDispatch(ctx, run))
			}
			ref, found, err := executor.services.runtime.Reference(ctx, "0", run)
			require.NoError(t, err)
			require.True(t, found)
			settled, err := executor.services.runtime.WaitLocal(ctx, ref)
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusFailed, settled.Status)
			require.Equal(t, "runtime.failed", settled.MessageCode)
			require.NotNil(t, settled.Error)
			require.NotContains(t, *settled.Error, diagnostic, "receipt reason stays coded rather than copying SDK output")
			if env != "0" {
				ref, found, err = hub.services.runtime.Reference(ctx, env, run)
				require.NoError(t, err)
				require.True(t, found)
				refreshed, err := hub.remote.Refresh(ctx, ref)
				require.NoError(t, err)
				require.Equal(t, "runtime.failed", refreshed.MessageCode)
			}
			reply, err := hub.server.Client().Get(hub.server.URL + "/api/environments/" + env + "/runtime-runs/" + run)
			require.NoError(t, err)
			data, err := io.ReadAll(reply.Body)
			require.NoError(t, err)
			require.NoError(t, reply.Body.Close())
			require.Equal(t, http.StatusOK, reply.StatusCode, string(data))
			var detail struct {
				Data activitytypes.RuntimeHubDetail `json:"data"`
			}
			require.NoError(t, json.Unmarshal(data, &detail))
			require.Equal(t, "runtime.failed", detail.Data.Activity.MessageCode)
			require.Equal(t, activitytypes.RuntimeOutcomeKnown, detail.Data.Activity.OutcomeState)
			var diagnostics []activitytypes.OutputLine
			for _, line := range detail.Data.Output {
				if strings.Contains(line.Text, diagnostic) {
					diagnostics = append(diagnostics, line.OutputLine)
				}
			}
			require.NotEmpty(t, diagnostics, "returned Compose error must reach the actual run detail consumer")
			require.Equal(t, "stderr", diagnostics[len(diagnostics)-1].Stream)
			require.Equal(t, activitytypes.MessageLevelError, diagnostics[len(diagnostics)-1].Level)
			before := runtimeEffectEvents(executor.services.http.snapshot())
			status, response = activationCall(t, hub, path, key, "respond-async", body)
			require.Equal(t, http.StatusOK, status, string(response))
			require.Equal(t, run, activationRunID(t, response))
			require.Equal(t, before, runtimeEffectEvents(executor.services.http.snapshot()), "joining failed history must never replay effects")
		})
	}
}
