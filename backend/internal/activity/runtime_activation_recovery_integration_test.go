// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Registered unknown responses and acknowledged stale-local recovery.

package activity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationUnknownLegacyAndAdministratorRecovery(t *testing.T) {
	for _, operation := range []activity.RuntimeOperation{activity.RuntimeDeployProject, activity.RuntimeDownProject, activity.RuntimeRestartProject, activity.RuntimeRedeployProject, activity.RuntimeStartContainer, activity.RuntimeStopContainer, activity.RuntimeRestartContainer} {
		t.Run(string(operation), func(t *testing.T) {
			var run, key, suffix string
			var body []byte
			setup := activationSetup{BeforeStart: func(f *runtimeServicesFixture, _ *activity.RuntimeHTTPService, _ *activity.RuntimeReceiver) {
				suffix, body = activationOperation(f, operation)
				target := activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID}
				options := activity.RuntimeOptions{}
				if strings.HasSuffix(string(operation), "project") {
					target = activity.RuntimeTarget{Kind: "stack", ID: "example-project", Name: new("example_stack")}
					if operation == activity.RuntimeDeployProject || operation == activity.RuntimeRedeployProject {
						options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
					}
				}
				previous := activity.NewRuntimeService(activity.NewActivityService(f.db, f.settings), activity.RuntimeResolverFunc(func(_ context.Context, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
					return activity.RuntimeResolvedTarget{Target: target}, nil
				}))
				store := f.store
				store.ExecutorGeneration = uuid.NewString()
				var original struct{ ID string }
				require.NoError(t, f.db.Table("users").Where("username = ?", "arcane").Select("id").Scan(&original).Error)
				require.NotEmpty(t, original.ID)
				command := activity.RuntimeCommand{Store: store, Key: uuid.NewString(), Actor: usertypes.Actor{ID: original.ID, Username: "Historical operator"}, Operation: operation, Target: target, Options: options}
				admitted, err := previous.Admit(t.Context(), command)
				require.NoError(t, err)
				run, key = admitted.ActivityID, command.Key
			}}
			hub := activationApp(t, false, "", setup)
			path := "/api/environments/0" + suffix
			status, response := activationCall(t, hub, path, key, "", body)
			if operation == activity.RuntimeDeployProject || operation == activity.RuntimeRedeployProject {
				require.Equal(t, 200, status, string(response))
				require.Contains(t, string(response), `"activityId":"`+run+`"`)
				require.NotContains(t, string(response), `{"done":true}`)
			} else {
				require.Equal(t, 503, status, string(response))
				require.Contains(t, string(response), `"activityId":"`+run+`"`)
			}
			require.Contains(t, string(response), string(activitytypes.CodeRuntimeOutcomeUnknown))
			require.Empty(t, runtimeEffectEvents(hub.services.http.snapshot()))
			releasePath := "/api/environments/0/runtime-runs/" + run + "/release"
			status, response = activationCall(t, hub, releasePath, "", "", []byte(`{"reason":"Executor stopped and Docker state inspected","acknowledged":false}`))
			require.Equal(t, 409, status, string(response))
			status, response = activationCall(t, hub, releasePath, "", "", []byte(`{"reason":"Executor stopped and Docker state inspected","acknowledged":true}`))
			require.Equal(t, 200, status, string(response))
			var released struct {
				Data activitytypes.RuntimeSnapshot `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response, &released))
			require.Equal(t, run, released.Data.ID)
			require.Equal(t, activitytypes.StatusFailed, released.Data.Status)
			require.Equal(t, activitytypes.RuntimeOutcomeUnknown, released.Data.OutcomeState)
			ref, found, err := hub.services.runtime.Reference(t.Context(), "0", run)
			require.NoError(t, err)
			require.True(t, found)
			detail, err := hub.services.runtime.GetRuntimeHubDetail(t.Context(), ref)
			require.NoError(t, err)
			require.Equal(t, new("released"), detail.Recovery.Disposition)
			require.True(t, detail.Recovery.ReleaseAcknowledged)
			require.NotNil(t, detail.Recovery.DispositionBy)
			require.Equal(t, detail.Activity.StartedBy.UserID, *detail.Recovery.DispositionBy)
			require.Equal(t, "Historical operator", detail.Activity.StartedBy.Username, "manual disposition preserves original attribution")
			require.Nil(t, detail.Recovery.ExecutorSettledAt)
			status, response = activationCall(t, hub, path, uuid.NewString(), "respond-async", body)
			require.Equal(t, 202, status, string(response))
			fresh := activationRunID(t, response)
			require.NotEqual(t, run, fresh)
			activationJoinRun(t, hub, hub, "0", fresh)
		})
	}
}

func TestRuntimeActivationManualRecoveryRefusesLiveLocalOwner(t *testing.T) {
	hub := activationApp(t, false, "")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	hub.services.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start") {
			once.Do(func() { close(entered) })
			<-release
			w.WriteHeader(204)
			return true
		}
		return false
	}
	status, response := activationCall(t, hub, "/api/environments/0/containers/"+runtimeContainerID+"/start", uuid.NewString(), "respond-async", nil)
	require.Equal(t, 202, status, string(response))
	run := activationRunID(t, response)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("actual SDK child did not enter")
	}
	// Inject loss of durable knowledge while the actual application worker is
	// still known to be live; an acknowledgment cannot erase this ownership fact.
	require.NoError(t, hub.services.db.Model(&activity.RuntimeReceipt{}).Where("run_id = ?", run).Updates(map[string]any{"executor_state": "unknown", "unknown_reason": "synthetic storage-continuity loss"}).Error)
	status, response = activationCall(t, hub, "/api/environments/0/runtime-runs/"+run+"/release", "", "", []byte(`{"reason":"Operator acknowledgment","acknowledged":true}`))
	require.Equal(t, 409, status, string(response))
	var receipt activity.RuntimeReceipt
	require.NoError(t, hub.services.db.First(&receipt, "run_id = ?", run).Error)
	require.Nil(t, receipt.SettledAt)
	require.Nil(t, receipt.HubDisposition)
	require.NoError(t, hub.services.db.Model(&activity.RuntimeReceipt{}).Where("run_id = ?", run).Updates(map[string]any{"executor_state": "executing", "unknown_reason": nil}).Error)
	unblock()
	activationJoinRun(t, hub, hub, "0", run)
}

func TestRuntimeActivationUnmatchedAcceptedAndExecutingStartup(t *testing.T) {
	for _, phase := range []string{"accepted", "executing"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			var run string
			hub := activationApp(t, false, "", activationSetup{BeforeStart: func(f *runtimeServicesFixture, _ *activity.RuntimeHTTPService, _ *activity.RuntimeReceiver) {
				previous := activity.NewRuntimeService(activity.NewActivityService(f.db, f.settings), activity.RuntimeResolverFunc(func(_ context.Context, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
					return activity.RuntimeResolvedTarget{Target: target}, nil
				}))
				store := f.store
				store.ExecutorGeneration = uuid.NewString()
				admitted, err := previous.Admit(ctx, activity.RuntimeCommand{Store: store, Key: uuid.NewString(), Actor: usertypes.Actor{ID: "historical-owner", Username: "Historical operator"}, Operation: activity.RuntimeStartContainer, Target: activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID}})
				require.NoError(t, err)
				run = admitted.ActivityID
				require.NoError(t, f.db.WithContext(ctx).Model(&activity.RuntimeReceipt{}).Where("run_id = ?", run).Updates(map[string]any{"executor_state": phase, "dispatch_state": "accepted", "dispatched_at": time.Now().UTC()}).Error)
			}})
			ref, found, err := hub.services.runtime.Reference(ctx, "0", run)
			require.NoError(t, err)
			require.True(t, found)
			detail, err := hub.services.runtime.GetRuntimeHubDetail(ctx, ref)
			require.NoError(t, err)
			require.Equal(t, activitytypes.RuntimeOutcomeUnknown, detail.Activity.OutcomeState)
			require.Equal(t, new("executor-continuity"), detail.Recovery.UnknownReason)
			require.Equal(t, "unknown", detail.Recovery.ExecutorState)
			require.Nil(t, detail.Recovery.ExecutorSettledAt)
			status, body := activationCall(t, hub, "/api/environments/0/runtime-runs/"+run+"/release", "", "", []byte(`{"reason":"Prior executor stopped and actual state inspected","acknowledged":true}`))
			require.Equal(t, 200, status, string(body))
			detail, err = hub.services.runtime.GetRuntimeHubDetail(ctx, ref)
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusFailed, detail.Activity.Status)
			require.Equal(t, activitytypes.RuntimeOutcomeUnknown, detail.Activity.OutcomeState)
			require.Equal(t, new("released"), detail.Recovery.Disposition)
			require.True(t, detail.Recovery.ReleaseAcknowledged)
			require.Empty(t, runtimeEffectEvents(hub.services.http.snapshot()), "ended historical executor is never replayed")
		})
	}
}
