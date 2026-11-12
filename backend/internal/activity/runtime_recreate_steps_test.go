// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original recreate run steps through the hub activity detail on every transport.

package activity_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Committed journal phases appear as named steps in the activity the browser
// reads from the hub, including runs executed by a Direct or Edge agent.
func TestRecreateStepsReachHubActivityDetail(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		for _, createFails := range []bool{false, true} {
			name := mode + "/completes"
			want := []string{"Renaming container", "Creating replacement", "Starting replacement", "Removing previous container"}
			if createFails {
				name = mode + "/compensates"
				want = []string{"Renaming container", "Creating replacement", "Restoring previous container"}
			}
			t.Run(name, func(t *testing.T) {
				hub := activationApp(t, false, "")
				executor, environment := hub, "0"
				if mode != "local" {
					executor = activationApp(t, true, uuid.NewString())
					environment = activationRemote(t, hub, executor, mode)
				}
				daemon := newRecreateHTTP(t)
				daemon.fail["create"] = createFails
				executor.services.http.onCall = daemon.serve
				path := "/api/environments/" + environment + "/containers/" + runtimeContainerID
				status, body := activationCall(t, hub, path+"/recreate", uuid.NewString(), "respond-async", []byte(`{"edit":{}}`))
				require.Equal(t, 202, status, string(body))
				run := activationRunID(t, body)
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				// A remote dispatch reaches the executor after the hub accepted the run.
				var ref activity.RuntimeRunReference
				require.Eventually(t, func() bool {
					var found bool
					var err error
					ref, found, err = executor.services.runtime.Reference(ctx, "0", run)
					return err == nil && found
				}, 10*time.Second, 10*time.Millisecond)
				_, err := executor.services.runtime.WaitLocal(ctx, ref)
				require.NoError(t, err)
				// Synchronise on the hub's settled projection, then compare the steps.
				var detail activitytypes.Detail
				require.Eventually(t, func() bool {
					detail = recreateDetail(t, hub, environment, run)
					return detail.Activity.EndedAt != nil
				}, 10*time.Second, 20*time.Millisecond)
				names := []string{}
				for _, step := range detail.StepTrace {
					names = append(names, step.Name)
				}
				require.Equal(t, want, names)
			})
		}
	}
}

func recreateDetail(t *testing.T, hub *activationFixture, environment, run string) activitytypes.Detail {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, hub.server.URL+"/api/environments/"+environment+"/activities/"+run, nil)
	require.NoError(t, err)
	response, err := hub.server.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode, string(data))
	var detail base.ApiResponse[activitytypes.Detail]
	require.NoError(t, json.Unmarshal(data, &detail))
	return detail.Data
}
