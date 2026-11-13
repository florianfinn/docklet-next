// Source: docklet-next/issues/366
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original replacement exclusion through public hub admission and real remote receivers.

package activity_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/stability/stabilitytest"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	a "github.com/florianfinn/docklet-next/types/activity"
	u "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type replacementClock struct {
	*stabilitytest.Clock
	entered, release chan struct{}
	once             sync.Once
}

func (c *replacementClock) Wait(ctx context.Context, until time.Time) error {
	c.once.Do(func() { close(c.entered); <-c.release })
	return c.Clock.Wait(ctx, until)
}
func replacementImageRoutes(hub *activationFixture) {
	api := humaecho.NewWithGroup(hub.router, hub.router.Group("/api"), huma.DefaultConfig("Replacement admission", "18"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		ctx = huma.WithValue(ctx, middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
		ctx = huma.WithValue(ctx, user.CurrentUserContextKey{}, &user.Actor{ID: "example-owner", Username: "Operator"})
		next(ctx)
	})
	for _, spec := range []struct {
		op     activity.RuntimeOperation
		suffix string
	}{{activity.RuntimeUpdateContainer, "update"}, {activity.RuntimeRestoreContainerImage, "previous-image/restore"}} {
		activity.RegisterRuntimeLifecycle(api, hub.services.activity, spec.op, huma.Operation{OperationID: string(spec.op), Method: http.MethodPost, Path: "/environments/{id}/containers/{containerId}/" + spec.suffix})
	}
}
func replacementAwait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("replacement phase barrier not reached")
	}
}
func replacementPolicyCommand(t *testing.T, hub *activationFixture) activity.RuntimeCommand {
	t.Helper()
	command := hub.services.command(activity.RuntimeUpdateContainer, activity.RuntimeTarget{Kind: "container", ID: recreateNewID, Name: new("app")})
	store := policy.Store{DB: hub.services.db.DB}
	require.NoError(t, hub.services.db.Exec("INSERT OR IGNORE INTO environments(id,name,api_url,enabled) VALUES(?,?,?,1)", "0", "Example", "https://agent.example.com").Error)
	p, err := store.Policy(t.Context(), "0", command.Store.RegistrationID, "app", "container")
	require.NoError(t, err)
	p.Mode, p.ActorID, p.TargetIdentity = "scheduled", command.Actor.ID, "app"
	p.SetWindow(&u.UpdateWindow{Kind: "daily", Time: "04:00"})
	require.NoError(t, store.SavePolicy(t.Context(), p, 0))
	p.Revision = 1
	child, _, err := store.PrepareChild(t.Context(), p, "example-window")
	require.NoError(t, err)
	command.ProposedRunID, command.Key = child.ID, child.ID
	command.Options.ContainerImage = &u.ContainerImageRequest{Policy: &u.PolicyBinding{PolicyID: p.ID, Revision: 1, JobRunID: child.JobRunID, ChildID: child.ID}}
	return command
}
func TestReplacementHubAdmissionClass(t *testing.T) {
	for _, phase := range []string{"created", "started", "observed"} {
		for _, op := range []string{"start", "stop", "restart", "recreate", "redeploy", "update", "previous-image/restore", "scheduled-update"} {
			t.Run(phase+"/"+op, func(t *testing.T) {
				daemon, registry := newContainerImageHTTP(t)
				clock := &replacementClock{Clock: stabilitytest.NewClock(), entered: make(chan struct{}), release: make(chan struct{})}
				unblock := sync.OnceFunc(func() { close(clock.release) })
				defer unblock()
				hub := activationApp(t, false, "", activationSetup{BeforeStart: func(f *runtimeServicesFixture, _ *activity.RuntimeHTTPService, _ *activity.RuntimeReceiver) {
					configureContainerImageFixture(t, f, daemon, registry)
					f.containers.ConfigureOperations(f.db.DB, f.store.RegistrationID, true, nil, nil)
					f.containers.UseObservationClock(clock)
				}})
				replacementImageRoutes(hub)
				suffix, body := "recreate", []byte(`{"edit":{}}`)
				entered := clock.entered
				if phase == "observed" {
					suffix, body = "update", []byte(`{"autoRestore":true}`)
				} else {
					daemon.base.hold, daemon.base.holdID = "start", recreateNewID
					if phase == "started" {
						daemon.base.hold, daemon.base.holdID = "remove", runtimeContainerID
					}
					daemon.base.entered, daemon.base.release = clock.entered, clock.release
				}
				root := "/api/environments/0/containers/"
				status, data := activationCall(t, hub, root+runtimeContainerID+"/"+suffix, uuid.NewString(), "respond-async", body)
				require.Equal(t, 202, status, string(data))
				run := activationRunID(t, data)
				replacementAwait(t, entered)
				replacement, present := daemon.base.copy(recreateNewID)
				require.True(t, present)
				require.Equal(t, run, replacement.Labels["io.docklet.run"])
				before := daemon.base.counts()
				var scheduled activity.RuntimeCommand
				if op == "scheduled-update" {
					scheduled = replacementPolicyCommand(t, hub)
					_, err := hub.services.runtime.Admit(t.Context(), scheduled)
					var gate *activity.GateConflict
					require.ErrorAs(t, err, &gate, "scheduled replacement must be refused busy")
					require.Equal(t, run, gate.RunID)
					require.Equal(t, "run.busy", gate.Code)
				} else {
					var body []byte
					if op == "recreate" {
						body = []byte(`{"edit":{}}`)
					}
					status, data = activationCall(t, hub, root+recreateNewID+"/"+op, uuid.NewString(), "respond-async", body)
					require.Equal(t, 409, status, "replacement operation must be busy: %s", string(data))
					require.Contains(t, string(data), "run.busy")
					require.Contains(t, string(data), run)
				}
				require.Equal(t, before, daemon.base.counts(), "competing replacement operation issued Docker effects")
				unblock()
				activationJoinRun(t, hub, hub, "0", run)
				hub.services.containers.UseObservationClock(stabilitytest.NewClock())
				var journal struct{ Phase string }
				require.NoError(t, hub.services.db.Table("container_recreate_journal").Where("run_id=?", run).Take(&journal).Error)
				require.Equal(t, "done", journal.Phase, "healthy recreate must complete without compensation")
				if op == "scheduled-update" {
					admitted, err := hub.services.runtime.Admit(t.Context(), scheduled)
					require.NoError(t, err, "settled recreate must release scheduled admission")
					require.NotEmpty(t, admitted.ActivityID)
				} else {
					var body []byte
					if op == "recreate" {
						body = []byte(`{"edit":{}}`)
					}
					status, data = activationCall(t, hub, root+recreateNewID+"/"+op, uuid.NewString(), "respond-async", body)
					require.Equal(t, 202, status, "settled recreate must release admission: %s", string(data))
				}
				require.NoError(t, hub.services.runtime.JoinLocal(t.Context()))
			})
		}
	}
}

// This response is captured from the real hub read for the adapter/UI proof.
// Default CI also checks the same producer fields without any evidence directory.
func replacementReadRefusal(t *testing.T, hub *activationFixture, env, run string) []byte {
	t.Helper()
	response, err := hub.server.Client().Get(hub.server.URL + "/api/environments/" + env + "/runtime-runs/" + run)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode, string(body))
	if output := os.Getenv("DOCKLET_REPLACEMENT_UI_EVIDENCE"); output != "" {
		require.NoError(t, os.WriteFile(output+"/"+env+".json", body, 0600))
	}
	activityRead := recreateDetail(t, hub, env, run)
	if output := os.Getenv("DOCKLET_REPLACEMENT_UI_EVIDENCE"); output != "" {
		encoded, err := json.Marshal(activityRead)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(output+"/"+env+"-activity.json", encoded, 0600))
	}
	require.Equal(t, "run.busy", activityRead.Activity.MessageCode, "activity read must retain busy refusal")
	var wire struct {
		Data struct {
			Activity a.RuntimeSnapshot
			Messages []struct{ Code string }
			Recovery struct{ Disposition, DispositionReason string }
		}
	}
	require.NoError(t, json.Unmarshal(body, &wire))
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", "data", "http", "replacement-refusal.test-support.ts"))
	require.NoError(t, err)
	_, raw, found := strings.Cut(string(fixture), "export const replacementRefusal = ")
	require.True(t, found)
	raw = strings.TrimSuffix(strings.TrimSpace(raw), " as const;")
	var golden struct {
		Data struct {
			Activity a.RuntimeSnapshot
			Messages []struct{ Code string }
		}
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &golden))
	require.Equal(t, golden.Data.Activity.MessageCode, wire.Data.Activity.MessageCode, "producer must match UI fixture refusal code")
	require.NotEmpty(t, wire.Data.Messages)
	require.Equal(t, golden.Data.Messages[len(golden.Data.Messages)-1].Code, wire.Data.Messages[len(wire.Data.Messages)-1].Code, "producer must match UI fixture no-effect message code")
	require.Equal(t, "run.busy", wire.Data.Activity.MessageCode, "remote refused read must project busy, not interrupted")
	require.Equal(t, a.RuntimeOutcomeKnown, wire.Data.Activity.OutcomeState)
	require.Equal(t, "refused", wire.Data.Recovery.Disposition)
	require.Equal(t, "run.busy", wire.Data.Recovery.DispositionReason)
	return body
}
func TestReplacementRemoteReceiverBusy(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			hub := activationApp(t, false, "")
			executor := activationApp(t, true, uuid.NewString())
			env := activationRemote(t, hub, executor, mode, mode)
			daemon := newRecreateHTTP(t)
			daemon.hold, daemon.holdID = "remove", runtimeContainerID
			daemon.entered, daemon.release = make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(daemon.release) })
			defer unblock()
			executor.services.http.onCall = daemon.serve
			root := "/api/environments/" + env + "/containers/"
			status, body := activationCall(t, hub, root+runtimeContainerID+"/recreate", uuid.NewString(), "respond-async", []byte(`{"edit":{}}`))
			require.Equal(t, 202, status, string(body))
			recreate := activationRunID(t, body)
			replacementAwait(t, daemon.entered)
			before := daemon.counts()
			status, body = activationCall(t, hub, root+recreateNewID+"/stop", uuid.NewString(), "respond-async", nil)
			require.Equal(t, 202, status, string(body))
			refused := activationRunID(t, body)
			require.NoError(t, hub.remote.WaitDispatch(t.Context(), refused))
			row := heldRecreateReceipt(t, hub, refused)
			require.NotNil(t, row.HubDisposition, "receiver must refuse the replacement")
			require.Equal(t, "refused", *row.HubDisposition)
			require.Equal(t, "run.busy", *row.HubDispositionReason)
			require.NotNil(t, row.SettledAt)
			require.Equal(t, before, daemon.counts(), "remote refusal must issue zero Docker effects")
			replacementReadRefusal(t, hub, env, refused)
			unblock()
			activationJoinRun(t, hub, executor, env, recreate)
			status, body = activationCall(t, hub, root+recreateNewID+"/stop", uuid.NewString(), "respond-async", nil)
			require.Equal(t, 202, status, string(body))
			activationJoinRun(t, hub, executor, env, activationRunID(t, body))
			require.Equal(t, before["stop"]+1, daemon.counts()["stop"])
		})
	}
}
