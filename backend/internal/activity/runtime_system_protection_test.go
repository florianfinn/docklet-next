// Source: docklet-next/issues/369
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original system-labelled descriptor, admission, metadata and executor parity.

package activity_test

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/capability"
	c "github.com/florianfinn/docklet-next/types/container"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	u "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	dc "github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestSystemWorkloadPublicProtection(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			hub := activationApp(t, false, "")
			executor, env := hub, "0"
			if mode != "local" {
				executor = activationApp(t, true, uuid.NewString())
				env = activationRemote(t, hub, executor, mode)
				if mode == "direct" {
					_, err := hub.env.TestConnection(t.Context(), env, nil)
					require.NoError(t, err)
				}
			}
			replacementImageRoutes(hub)
			daemon := newRecreateHTTP(t)
			executor.services.http.onCall = daemon.serve
			for _, role := range []string{"agent", "hub", "ordinary"} {
				t.Run(role, func(t *testing.T) {
					labels := map[string]string{}
					if role != "ordinary" {
						labels["io.docklet.system"] = role
					}
					daemon.mu.Lock()
					daemon.containers = map[string]recreateObserved{runtimeContainerID: {ID: runtimeContainerID, Name: "app", Labels: labels, Running: true}}
					daemon.mu.Unlock()
					response, err := hub.server.Client().Get(hub.server.URL + "/api/capabilities?kind=container&id=" + env + "~" + runtimeContainerID)
					require.NoError(t, err)
					data, err := io.ReadAll(response.Body)
					response.Body.Close()
					require.NoError(t, err)
					require.Equal(t, 200, response.StatusCode, string(data))
					var wire base.ApiResponse[[]capability.Descriptor]
					require.NoError(t, json.Unmarshal(data, &wire))
					found := false
					for _, descriptor := range wire.Data {
						if descriptor.OperationID == "recreateContainer" {
							found = true
							require.Equal(t, role == "ordinary", descriptor.Available, "system-labelled descriptor must be disabled")
							if role != "ordinary" {
								require.Equal(t, "container.origin-locked", descriptor.DisabledReasonCode)
							}
						}
					}
					require.True(t, found)
					response, err = hub.server.Client().Get(hub.server.URL + "/api/environments/" + env + "/containers/" + runtimeContainerID)
					require.NoError(t, err)
					data, err = io.ReadAll(response.Body)
					response.Body.Close()
					require.NoError(t, err)
					require.Equal(t, 200, response.StatusCode, string(data))
					var detail base.ApiResponse[c.Details]
					require.NoError(t, json.Unmarshal(data, &detail))
					require.Equal(t, role != "ordinary", detail.Data.RedeployDisabled, "details protection must match descriptor")
					before := daemon.counts()
					ops := []string{"recreate", "redeploy", "update", "previous-image/restore"}
					if role == "ordinary" {
						ops = []string{"recreate"}
					}
					for _, op := range ops {
						var body []byte
						if op == "recreate" {
							body = []byte(`{"edit":{}}`)
						}
						status, data := activationCall(t, hub, "/api/environments/"+env+"/containers/"+runtimeContainerID+"/"+op, uuid.NewString(), "respond-async", body)
						if role == "ordinary" {
							require.Equal(t, 202, status, string(data))
							activationJoinRun(t, hub, executor, env, activationRunID(t, data))
						} else {
							require.Equal(t, 409, status, "system workload must refuse before dispatch: %s", string(data))
							require.Contains(t, string(data), "container.origin-locked")
						}
					}
					if role != "ordinary" {
						require.Equal(t, before, daemon.counts(), "protected workload issued Docker effects")
					}
				})
			}
		})
	}
}
func TestSystemWorkloadDirectExecutorAndPolicy(t *testing.T) {
	for _, role := range []string{"agent", "hub"} {
		t.Run(role, func(t *testing.T) {
			hub := activationApp(t, false, "")
			f := hub.services
			daemon, registry := newContainerImageHTTP(t)
			configureContainerImageFixture(t, f, daemon, registry)
			labels := map[string]string{"io.docklet.system": role}
			old := daemon.base.containers[runtimeContainerID]
			old.Labels = labels
			daemon.base.containers[runtimeContainerID] = old
			for _, op := range []activity.RuntimeOperation{activity.RuntimeRecreateContainer, activity.RuntimeRedeployContainer, activity.RuntimeUpdateContainer, activity.RuntimeRestoreContainerImage} {
				command := f.command(op, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID, Name: new("app")})
				command.ProposedRunID = uuid.NewString()
				if op == activity.RuntimeRecreateContainer {
					command.Options.Recreate = &c.Edit{}
				}
				err := f.adapter.ExecuteRuntime(t.Context(), command, io.Discard)
				var problem *c.Problem
				require.ErrorAs(t, err, &problem, "direct executor must refuse system workload")
				require.Equal(t, "container.origin-locked", problem.Code)
				require.Empty(t, daemon.base.counts(), "system workload effect guard must issue zero effects")
			}
			target, err := f.adapter.ResolveRuntimeTarget(t.Context(), activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
			require.NoError(t, err)
			require.True(t, target.Protected, "resolved metadata must protect Docklet workloads")
			details, err := f.containers.GetContainerDetails(t.Context(), runtimeContainerID)
			require.NoError(t, err)
			require.True(t, details.RedeployDisabled)
			// Force a successful, nonmatching current-process identity at the real list producer.
			rows := f.containers.BuildSummaries(t.Context(), []dc.Summary{{ID: runtimeContainerID, Names: []string{"/app"}, Labels: labels, Image: daemon.ref}}, nil, runtimeSecondContainerID, nil)
			require.Len(t, rows, 1)
			require.True(t, rows[0].RedeployDisabled, "list protection must be independent of cgroup failures")
			events := event.NewEventService(f.db, hub.cfg, hub.server.Client())
			service, err := updater.NewUpdaterService(f.db, f.settings, f.docker, f.projects, nil, nil, events, f.images, nil, f.activity, hub.cfg, nil, nil, hub.roles)
			require.NoError(t, err)
			service.ConfigureImagePolicies(hub.env, nil, f.containers)
			require.NoError(t, hub.services.db.Exec("INSERT OR IGNORE INTO environments(id,name,api_url,enabled) VALUES(?,?,?,1)", "0", "Example", "https://agent.example.com").Error)
			var admin user.User
			require.NoError(t, f.db.First(&admin, "username=?", "arcane").Error)
			_, err = service.SaveImagePolicy(t.Context(), "0", "app", u.UpdatePolicyInput{Mode: "scheduled", Window: &u.UpdateWindow{Kind: "daily", Time: "04:00"}}, admin.ID, "", "container")
			require.Error(t, err)
			if role == "hub" {
				require.Equal(t, "container.origin-locked", imageDTO.Neutral(err, "unexpected", 500).Problem.Code, "system workload must be ineligible for policy updates")
			}
		})
	}
}
