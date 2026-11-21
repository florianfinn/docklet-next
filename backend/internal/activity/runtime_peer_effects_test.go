// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531. M34 #547: unreadable peers never block; a peer duplicate refuses only name-addressed lookups.

package activity_test

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	a "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
)

const s4MissingService = "services:\n  app:\n    image: registry.example.com/app:1\n    env_file: [service.env]\n"

func s4SetRuntimeContainer(f *runtimeServicesFixture) {
	f.http.setLabels(map[string]string{"com.docker.compose.project": "target_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
	f.http.setComposeContainer(true)
}
func s4RuntimeProblem(t *testing.T, err error) *imageDTO.RuntimeIdentityBlocker {
	t.Helper()
	var problem *imageDTO.OperationError
	require.ErrorAs(t, err, &problem)
	require.NotNil(t, problem.Problem.Blocker)
	return problem.Problem.Blocker
}
func TestS4DistinctDegradedPeerActualEffectsHubAndAgent(t *testing.T) {
	for _, mode := range []string{"hub", "direct", "edge"} {
		for _, op := range []activity.RuntimeOperation{activity.RuntimeStopProject, activity.RuntimeRestartProject, activity.RuntimeRedeployProject} {
			t.Run(mode+"/"+string(op), func(t *testing.T) {
				var f *runtimeServicesFixture
				var remote *remoteIntegration
				if mode == "hub" {
					f = newRuntimeServicesFixture(t)
				} else {
					remote = newRemoteIntegration(t, mode == "edge")
					f = remote.agent
				}
				f.project("example-target", "target", "name: target_stack\n"+identitySDKServices)
				f.project("example-peer", "peer", "name: distinct_peer\n"+s4MissingService)
				s4SetRuntimeContainer(f)
				c := f.command(op, activity.RuntimeTarget{Kind: "stack", ID: "example-target"})
				if op == activity.RuntimeRedeployProject {
					c.Options.Deploy = &projectDTO.DeployOptions{PullPolicy: "never"}
				}
				if remote == nil {
					f.run(c)
				} else {
					c.Store = remote.store
					admitted, err := remote.hub.Admit(t.Context(), c)
					require.NoError(t, err, "agent admission automatically passes proven distinct peers")
					ref := activity.RuntimeRunReference{Store: remote.store, RunID: admitted.ActivityID}
					_, err = remote.control.Start(t.Context(), ref, c.Options)
					require.NoError(t, err)
					ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
					defer cancel()
					require.NoError(t, remote.control.WaitDispatch(ctx, ref.RunID))
					result, err := f.waitLocal(ref.RunID)
					require.NoError(t, err)
					require.Equal(t, a.StatusSuccess, result.Status, f.debugEvents())
				}
				require.NotEmpty(t, identitySDKEffects(f.http.snapshot()), "successful stop/restart/redeploy must reach actual Docker HTTP effects")
			})
		}
	}
}

type s4HeldExecutor struct {
	adapter          *activity.RuntimeServiceAdapter
	entered, release chan struct{}
}

func (e *s4HeldExecutor) ExecuteRuntime(ctx context.Context, c activity.RuntimeCommand, w io.Writer) error {
	close(e.entered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.release:
	}
	return e.adapter.ExecuteRuntime(ctx, c, w)
}

func TestS4PreEffectRecheckIgnoresPeersButNotTheTarget(t *testing.T) {
	for _, change := range []string{"unchanged", "new-peer", "changed-reason", "duplicate", "recreated", "target", "archived-target"} {
		t.Run(change, func(t *testing.T) {
			f := newRuntimeServicesFixture(t, false)
			targetPath := f.project("example-target", "target", "name: target_stack\n"+identitySDKServices)
			peer := f.project("example-peer", "peer", "not: [valid")
			s4SetRuntimeContainer(f)
			target := activity.RuntimeTarget{Kind: "stack", ID: "example-target"}
			_, err := f.adapter.ResolveRuntimeTarget(t.Context(), target)
			require.NoError(t, err, "an unreadable peer never blocks admission")
			held := &s4HeldExecutor{adapter: f.adapter, entered: make(chan struct{}), release: make(chan struct{})}
			var once sync.Once
			release := func() { once.Do(func() { close(held.release) }) }
			defer release()
			runtime := activity.NewRuntimeService(f.activity, f.adapter)
			generation, err := runtime.InitializeGeneration(t.Context())
			require.NoError(t, err)
			store := activity.RuntimeStoreBinding{EnvironmentID: "0", StoreGeneration: generation, ExecutorGeneration: generation, ManagerID: "example-manager", RegistrationID: "example-registration"}
			require.NoError(t, runtime.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: store, Executor: held}))
			c := f.command(activity.RuntimeStopProject, target)
			c.Store = store
			admitted, err := runtime.Admit(t.Context(), c)
			require.NoError(t, err)
			select {
			case <-held.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("executor seam not reached")
			}
			switch change {
			case "new-peer":
				f.project("example-later", "later", "not: [valid")
			case "changed-reason":
				require.NoError(t, os.Mkdir(filepath.Join(peer, "nested"), 0700))
				require.NoError(t, os.WriteFile(filepath.Join(peer, "nested", "compose.yaml"), []byte(identitySDKServices), 0600))
				require.NoError(t, os.WriteFile(filepath.Join(peer, "compose.yaml"), []byte(identitySDKServices), 0600))
				require.NoError(t, os.WriteFile(filepath.Join(peer, ".env"), []byte("COMPOSE_FILE=nested/compose.yaml\n"), 0600))
			case "duplicate":
				require.NoError(t, os.WriteFile(filepath.Join(peer, "compose.yaml"), []byte("name: target_stack\n"+identitySDKServices), 0600))
			case "target":
				require.NoError(t, os.WriteFile(filepath.Join(targetPath, "compose.yaml"), []byte("name: changed_target\n"+identitySDKServices), 0600))
			case "recreated":
				require.NoError(t, f.db.Model(&project.Project{}).Where("id = ?", "example-peer").Update("id", "example-recreated").Error)
			case "archived-target":
				require.NoError(t, f.db.Model(&project.Project{}).Where("id = ?", "example-target").Update("is_archived", true).Error)
			}
			before := len(f.http.snapshot())
			release()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			result, err := runtime.WaitLocal(ctx, activity.RuntimeRunReference{Store: store, RunID: admitted.ActivityID})
			require.NoError(t, err)
			effects := identitySDKEffects(f.http.snapshot()[before:])
			if change == "target" || change == "archived-target" {
				require.Empty(t, effects, "a changed target must refuse before every Docker mutation")
				require.Equal(t, a.StatusFailed, result.Status)
			} else {
				require.Equal(t, a.StatusSuccess, result.Status, "peer changes, including a new duplicate, never stop a project-ID action")
				require.NotEmpty(t, effects)
			}
			var receipt activity.RuntimeReceipt
			require.NoError(t, f.db.First(&receipt, "run_id = ?", admitted.ActivityID).Error)
			require.Empty(t, receipt.AcknowledgedUncertainty)
		})
	}
}

func TestS4UnreadablePeerDirectEdgeRunsWithoutAcknowledgement(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			f := newRemoteIntegration(t, isEdge)
			f.agent.project("example-target", "target", "name: target_stack\n"+identitySDKServices)
			f.agent.project("example-peer", "peer", "name: ${PEER_NAME:?required}\n"+identitySDKServices)
			s4SetRuntimeContainer(f.agent)
			req := a.RuntimeTargetRequest{Registration: a.RuntimeExecutorIdentity{ManagerID: f.store.ManagerID, RegistrationID: f.store.RegistrationID, ExecutorGeneration: f.store.ExecutorGeneration}, Target: activity.RuntimeTarget{Kind: "stack", ID: "example-target"}}
			metadata, err := f.sender.Resolve(t.Context(), req)
			require.NoError(t, err, "the agent skips an unreadable peer")
			require.Nil(t, metadata.PeerOverrides)
			req.AcknowledgedUncertainty = "invalid"
			_, invalidErr := f.sender.Resolve(t.Context(), req)
			var invalidProblem *imageDTO.OperationError
			require.ErrorAs(t, invalidErr, &invalidProblem)
			require.Equal(t, "runtime.invalid-acknowledgement", invalidProblem.Problem.Code, "contract 20 validation stays")
			require.Equal(t, 400, invalidProblem.Status)
			req.AcknowledgedUncertainty = ""
			mux := s4PeerHubRoute(t, f, req)
			post := func(digest string) *httptest.ResponseRecorder {
				body := ""
				if digest != "" {
					body = `{"acknowledgedUncertainty":"` + digest + `"}`
				}
				request := httptest.NewRequest(http.MethodPost, "/environments/example-remote/projects/example-target/stop", strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Idempotency-Key", uuid.NewString())
				request.Header.Set("Prefer", "respond-async")
				recorder := httptest.NewRecorder()
				mux.ServeHTTP(recorder, request)
				return recorder
			}
			// A digest from an older hub's earlier confirmation is accepted and ignored.
			for _, digest := range []string{"", strings.Repeat("a", 64)} {
				admission := post(digest)
				require.Equal(t, 202, admission.Code, admission.Body.String())
				var admitted struct{ Data a.RuntimeAdmission }
				require.NoError(t, json.Unmarshal(admission.Body.Bytes(), &admitted))
				ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.Data.ActivityID}
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				require.NoError(t, f.control.WaitDispatch(ctx, ref.RunID))
				result, err := f.agent.waitLocal(ref.RunID)
				require.NoError(t, err)
				require.Equal(t, a.StatusSuccess, result.Status)
				_, err = f.control.Wait(ctx, ref, nil)
				cancel()
				require.NoError(t, err)
				var hubRun activity.Activity
				require.NoError(t, f.db.First(&hubRun, "id = ?", ref.RunID).Error)
				require.Equal(t, a.StatusSuccess, hubRun.Status)
				var count int64
				require.NoError(t, f.db.Table("activity_messages").Where("activity_id = ? AND code = ?", ref.RunID, "runtime.peer-acknowledged").Count(&count).Error)
				require.Zero(t, count, "nothing was overridden, so nothing is audited")
			}
			require.NotEmpty(t, identitySDKEffects(f.agent.http.snapshot()))
		})
	}
}

func TestS4UnsafeFileNamesOmittedAndStrictBoundaries(t *testing.T) {
	for _, path := range []string{"../external.env", "C:/external.env"} {
		t.Run(strings.ReplaceAll(path, "/", "-"), func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			f.project("example-target", "target", "name: target_stack\n"+identitySDKServices)
			f.project("example-peer", "peer", "name: target_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n    env_file: ['"+path+"']\n")
			s4SetRuntimeContainer(f)
			_, err := f.adapter.ResolveRuntimeTarget(t.Context(), activity.RuntimeTarget{Kind: "stack", ID: "example-target"})
			require.NoError(t, err, "a project-ID action proceeds despite the duplicate")
			_, err = f.adapter.ResolveRuntimeTarget(t.Context(), activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
			b := s4RuntimeProblem(t, err)
			require.Equal(t, "compose.duplicate", b.Reason, "the container's parent name is ambiguous")
			require.False(t, b.Confirmable)
			require.Empty(t, b.File, "unsafe paths must never be displayed")
		})
	}
}

func s4PeerHubRoute(t *testing.T, f *remoteIntegration, registration a.RuntimeTargetRequest) http.Handler {
	t.Helper()
	prepare := func(ctx context.Context, _ string, target activity.RuntimeTarget, digest string) (activity.RuntimeResolvedTarget, error) {
		req := registration
		req.Target = target
		req.AcknowledgedUncertainty = digest
		metadata, err := f.sender.Resolve(ctx, req)
		return activity.RuntimeResolvedTarget{Target: metadata.Target, PeerOverrides: metadata.PeerOverrides, ComposeManaged: metadata.ComposeManaged}, err
	}
	h, err := activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, PrepareAcknowledgedTarget: prepare, PrepareTarget: func(ctx context.Context, id string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
		return prepare(ctx, id, target, "")
	}})
	require.NoError(t, err)
	h.SetReady(true)
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("runtime", "test"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
		actor := f.command(activity.RuntimeStopProject, registration.Target).Actor
		request = context.WithValue(request, usertypes.CurrentUserContextKey{}, &actor)
		next(huma.WithContext(ctx, request))
	})
	activity.RegisterRuntimeLifecycle(api, f.hubActivity, activity.RuntimeStopProject, huma.Operation{OperationID: "stop-peer-test", Method: http.MethodPost, Path: "/environments/{id}/projects/{projectId}/stop"})
	return mux
}
