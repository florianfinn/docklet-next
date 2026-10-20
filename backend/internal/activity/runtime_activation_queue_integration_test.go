// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Real Fx/SDK limiter contention and actual generic queued cancellation.

package activity_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationQueuedDeploymentAndGenericCancel(t *testing.T) {
	for _, transport := range []string{"local", "direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			setup := activationSetup{BeforeStart: func(f *runtimeServicesFixture, _ *activity.RuntimeHTTPService, _ *activity.RuntimeReceiver) {
				require.NoError(t, f.settings.SetIntSetting(t.Context(), "maxConcurrentActivities", 1))
			}}
			hub := activationApp(t, false, "", setup)
			executor := hub
			env := "0"
			if transport != "local" {
				executor = activationApp(t, true, uuid.NewString(), setup)
				env = activationRemote(t, hub, executor, transport)
			}
			firstSuffix, body := activationOperation(executor.services, activity.RuntimeDeployProject)
			executor.services.project("example-other", "example-other", "name: other_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			executor.services.http.onCall = func(_ http.ResponseWriter, r *http.Request) bool {
				if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/start") {
					once.Do(func() { close(entered) })
					<-release
				}
				return false
			}
			status, response := activationCall(t, hub, "/api/environments/"+env+firstSuffix, uuid.NewString(), "respond-async", body)
			require.Equal(t, 202, status, string(response))
			first := activationRunID(t, response)
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("limit-one deployment never reached its actual SDK worker")
			}
			status, response = activationCall(t, hub, "/api/environments/"+env+"/projects/example-other/up", uuid.NewString(), "respond-async", body)
			require.Equal(t, 202, status, string(response))
			second := activationRunID(t, response)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			require.NoError(t, hub.remote.WaitDispatch(ctx, first))
			require.NoError(t, hub.remote.WaitDispatch(ctx, second))
			reply, err := hub.server.Client().Get(hub.server.URL + "/api/environments/" + env + "/activities/" + second)
			require.NoError(t, err)
			data, err := io.ReadAll(reply.Body)
			require.NoError(t, err)
			require.NoError(t, reply.Body.Close())
			require.Equal(t, 200, reply.StatusCode, string(data))
			var queued struct {
				Data activitytypes.Detail `json:"data"`
			}
			require.NoError(t, json.Unmarshal(data, &queued))
			require.Equal(t, activitytypes.StatusQueued, queued.Data.Activity.Status)
			require.NotNil(t, queued.Data.Activity.RuntimeKnowledge)
			require.Equal(t, activitytypes.RuntimeOutcomeKnown, queued.Data.Activity.OutcomeState)
			status, response = activationCall(t, hub, "/api/environments/"+env+"/activities/"+second+"/cancel", "", "", nil)
			require.Equal(t, 200, status, string(response))
			ref, found, err := executor.services.runtime.Reference(ctx, "0", second)
			require.NoError(t, err)
			require.True(t, found)
			cancelled, err := executor.services.runtime.WaitLocal(ctx, ref)
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusCancelled, cancelled.Status)
			if env != "0" {
				ref, found, err = hub.services.runtime.Reference(ctx, env, second)
				require.NoError(t, err)
				require.True(t, found)
				cancelled, err = hub.remote.Wait(ctx, ref, nil)
				require.NoError(t, err)
				require.Equal(t, activitytypes.StatusCancelled, cancelled.Status)
			}
			var original activity.RuntimeReceipt
			require.NoError(t, executor.services.db.First(&original, "run_id = ?", first).Error)
			require.Nil(t, original.SettledAt)
			status, response = activationCall(t, hub, "/api/environments/"+env+"/projects/example-project/down", uuid.NewString(), "respond-async", nil)
			require.Equal(t, 409, status, string(response))
			require.Contains(t, string(response), first)
			unblock()
			activationJoinRun(t, hub, executor, env, first)
			effects := runtimeEffectEvents(executor.services.http.snapshot())
			require.Len(t, effects, 3, "only the first deployment created its network, container and start")
			require.True(t, strings.HasSuffix(effects[0].Path, "/networks/create"))
			require.True(t, strings.HasSuffix(effects[1].Path, "/containers/create"))
			require.Contains(t, effects[1].Query, "example_stack-app-1")
			require.True(t, strings.HasSuffix(effects[2].Path, "/"+runtimeContainerID+"/start"))
			var secondReceipt activity.RuntimeReceipt
			require.NoError(t, executor.services.db.First(&secondReceipt, "run_id = ?", second).Error)
			require.Nil(t, secondReceipt.ExecutorStartedAt)
		})
	}
}
