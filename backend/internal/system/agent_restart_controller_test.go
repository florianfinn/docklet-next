// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original real receiver, Direct HTTP and Edge reconnect controller proofs.

package system

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/contract"
	dto "github.com/florianfinn/docklet-next/types/environment"
	systemDTO "github.com/florianfinn/docklet-next/types/system"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

type restartTransportFixture struct {
	db        *database.DB
	service   *environment.EnvironmentService
	hub       *echo.Echo
	agent     http.Handler
	release   chan struct{}
	restarted chan struct{}
	postCalls atomic.Int32
	versions  atomic.Int32
	instance  atomic.Value
	changes   <-chan struct{}
}

func restartAPI(router *echo.Echo, title string) huma.API {
	cfg := huma.DefaultConfig(title, "0.0.0")
	cfg.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(typ reflect.Type, hint string) string {
		return typ.PkgPath() + "." + huma.DefaultSchemaNamer(typ, hint)
	})
	return humaecho.NewWithGroup(router, router.Group("/api"), cfg)
}
func newRestartTransportFixture(t *testing.T) *restartTransportFixture {
	t.Helper()
	f := &restartTransportFixture{release: make(chan struct{}), restarted: make(chan struct{}, 1)}
	f.instance.Store("example-old")
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), database.MigrationOptions{})
	require.NoError(t, err)
	f.db = db
	t.Cleanup(func() { require.NoError(t, f.db.Close()) })
	dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"Id": "example-self"}))
			return
		}
		require.Equal(t, "POST", r.Method)
		require.Contains(t, r.URL.Path, "/containers/example-self/restart")
		f.instance.Store("example-new")
		f.restarted <- struct{}{}
		w.WriteHeader(204)
	}))
	t.Cleanup(dockerServer.Close)
	dockerClient, err := client.New(client.WithHost(dockerServer.URL), client.WithAPIVersion("1.54"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, dockerClient.Close()) })
	router := echo.New()
	api := restartAPI(router, "Restart agent")
	cfg := &config.Config{AgentMode: true, AgentToken: "example", AuthMode: "local"}
	authService := auth.NewAuthService(nil, nil, nil, nil, cfg)
	api.UseMiddleware(auth.NewHumaMiddleware(api, authService, nil, nil, nil, cfg))
	h := &agentRestartHandler{appCtx: t.Context(), selfID: func() (string, error) { return "example-self", nil }, client: func(context.Context) (*client.Client, error) { return dockerClient, nil }, wait: func(ctx context.Context, delay time.Duration) bool {
		if delay != time.Second {
			panic("wrong restart delay")
		}
		select {
		case <-ctx.Done():
			return false
		case <-f.release:
			return true
		}
	}}
	h.register(api)
	huma.Get(api, "/health", func(context.Context, *struct{}) (*struct{ Body systemDTO.HealthResponse }, error) {
		return &struct{ Body systemDTO.HealthResponse }{Body: systemDTO.HealthResponse{Status: "UP", InstanceID: f.instance.Load().(string)}}, nil
	})
	huma.Get(api, "/app-version", func(context.Context, *struct{}) (*struct{ Body version.Info }, error) {
		f.versions.Add(1)
		return &struct{ Body version.Info }{Body: version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}}, nil
	})
	huma.Head(api, "/environments/{id}/system/health", func(context.Context, *struct {
		ID string `path:"id"`
	}) (*struct{}, error) {
		return nil, nil
	}, func(op *huma.Operation) { op.Security = handlerutil.DefaultOperationSecurity() })
	f.agent = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/system/agent/restart" {
			f.postCalls.Add(1)
		}
		router.ServeHTTP(w, r)
	})
	return f
}
func (f *restartTransportFixture) startHub(t *testing.T, endpoint string, isEdge bool) {
	t.Helper()
	var key *string
	if isEdge {
		key = new("example-key")
		require.NoError(t, f.db.Exec("INSERT INTO api_keys(id,name,key_hash,key_prefix,kind) VALUES('example-key','Example key','example-hash','example','environment')").Error)
	}
	require.NoError(t, f.db.Create(&environment.Environment{ID: "example-host", Name: "Example host", ApiUrl: endpoint, IsEdge: isEdge, Enabled: true, AccessToken: new("example"), ApiKeyID: key}).Error)
	f.service = environment.NewEnvironmentService(f.db, http.DefaultClient, nil, nil, nil, nil)
	life, cancel := context.WithCancel(t.Context())
	require.NoError(t, f.service.StartAgentRestarts(life))
	t.Cleanup(func() { cancel(); f.service.StopAgentRestarts() })
	var unsubscribe func()
	f.changes, unsubscribe = f.service.SubscribeRuntimeChanges()
	t.Cleanup(unsubscribe)
	f.hub = echo.New()
	f.hub.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(ctx context.Context, id string) (string, *string, bool, error) {
		env, err := f.service.GetEnvironmentByID(ctx, id)
		if err != nil {
			return "", nil, false, err
		}
		return env.ApiUrl, env.AccessToken, env.Enabled, nil
	}, nil, nil, edge.GetRegistry(), nil, middleware.EnvironmentProxySecurity{AgentAccess: f.service.AgentAccess}))
	api := restartAPI(f.hub, "Restart hub")
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	environment.RegisterEnvironments(api, environment.NewHandler(f.service, nil, nil, nil, &config.Config{}, nil))
}
func (f *restartTransportFixture) post(t *testing.T) (int, dto.AgentRestartRun) {
	t.Helper()
	rec := httptest.NewRecorder()
	f.hub.ServeHTTP(rec, httptest.NewRequest("POST", "/api/environments/example-host/agent/restart", nil))
	var out base.ApiResponse[dto.AgentRestartRun]
	if rec.Code == 202 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	}
	return rec.Code, out.Data
}
func (f *restartTransportFixture) read(t *testing.T, id string) dto.AgentRestartRun {
	t.Helper()
	rec := httptest.NewRecorder()
	f.hub.ServeHTTP(rec, httptest.NewRequest("GET", "/api/environments/example-host/agent/restarts/"+id, nil))
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var out base.ApiResponse[dto.AgentRestartRun]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out.Data
}
func (f *restartTransportFixture) await(t *testing.T, id, state string) dto.AgentRestartRun {
	t.Helper()
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	for {
		run := f.read(t, id)
		if run.State == state {
			return run
		}
		select {
		case <-f.changes:
		case <-timeout.C:
			t.Fatalf("restart state %s, wanted %s", run.State, state)
		}
	}
}
func TestRestartControllerRealDirectReceiver(t *testing.T) {
	f := newRestartTransportFixture(t)
	agent := httptest.NewServer(f.agent)
	defer agent.Close()
	f.startHub(t, agent.URL, false)
	status, run := f.post(t)
	require.Equal(t, 202, status)
	f.await(t, run.RunID, "awaiting")
	require.EqualValues(t, 1, f.postCalls.Load())
	require.Equal(t, "awaiting", f.read(t, run.RunID).State)
	close(f.release)
	select {
	case <-f.restarted:
	case <-time.After(5 * time.Second):
		t.Fatal("real receiver did not call Docker")
	}
	f.await(t, run.RunID, "connected")
	require.EqualValues(t, 1, f.postCalls.Load())
}
func TestRestartControllerRealEdgeDisconnectReconnect(t *testing.T) {
	f := newRestartTransportFixture(t)
	registry := edge.NewTunnelRegistry()
	previous := edge.GetRegistry()
	edge.SetDefaultRegistry(registry)
	defer edge.SetDefaultRegistry(previous)
	connected := make(chan struct{}, 8)
	server := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return "example-host", nil }, func(_ context.Context, _ string, online bool) {
		if online {
			connected <- struct{}{}
		}
	})
	server.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return "example-key", nil })
	router := echo.New()
	router.GET("/api/tunnel/connect", server.HandleConnect)
	manager := httptest.NewServer(router)
	defer manager.Close()
	start := func(tunnelClient *edge.TunnelClient) (context.CancelFunc, <-chan struct{}) {
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { defer close(done); tunnelClient.StartWithErrorChan(ctx, nil) }()
		select {
		case <-connected:
		case <-time.After(5 * time.Second):
			t.Fatal("Edge registration missing")
		}
		return cancel, done
	}
	newClient := func() *edge.TunnelClient {
		return edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: "example", EdgeReconnectInterval: 1}, f.agent)
	}
	oldClient := newClient()
	cancel, done := start(oldClient)
	defer func() {
		cancel()
		registry.Unregister("example-host")
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Edge client did not join")
		}
	}()
	f.startHub(t, "edge://example", true)
	status, run := f.post(t)
	require.Equal(t, 202, status)
	f.await(t, run.RunID, "awaiting")
	require.EqualValues(t, 1, f.postCalls.Load())
	oldTunnel, _ := registry.Get("example-host").Get()
	old := oldTunnel.MetadataSnapshot()
	close(f.release)
	select {
	case <-f.restarted:
	case <-time.After(5 * time.Second):
		t.Fatal("receiver effect missing")
	}
	registry.Unregister("example-host")
	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("same process reconnect missing")
	}
	sameTunnel, _ := registry.Get("example-host").Get()
	same := sameTunnel.MetadataSnapshot()
	require.NotEqual(t, old.SessionID, same.SessionID)
	require.Equal(t, old.AgentInstance, same.AgentInstance)
	// Wait until the controller has observed the reconnected session before asserting.
	observed := f.versions.Load()
	require.Eventually(t, func() bool {
		return f.versions.Load() >= observed+2 || f.read(t, run.RunID).SettledAt != nil
	}, 10*time.Second, 20*time.Millisecond)
	require.Equal(t, "awaiting", f.read(t, run.RunID).State, "same instance reconnect cannot complete")
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("old Edge process did not join")
	}
	registry.Unregister("example-host")
	cancel, done = start(newClient())
	replacement, _ := registry.Get("example-host").Get()
	require.NotEqual(t, old.AgentInstance, replacement.MetadataSnapshot().AgentInstance)
	f.await(t, run.RunID, "connected")
	require.EqualValues(t, 1, f.postCalls.Load())
}

type restartRaceResolver struct{}

func (restartRaceResolver) ResolveRuntimeTarget(_ context.Context, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
	return activity.RuntimeResolvedTarget{Target: target}, nil
}
func TestRestartReservationRaceWithOrdinaryAndPolicyRuntime(t *testing.T) {
	for _, kind := range []string{"ordinary", "policy"} {
		for _, winner := range []string{"restart", "runtime", "race"} {
			t.Run(kind+"/"+winner, func(t *testing.T) {
				f := newRestartTransportFixture(t)
				agent := httptest.NewServer(f.agent)
				defer agent.Close()
				f.startHub(t, agent.URL, false)
				require.NoError(t, f.db.Exec("INSERT INTO users(id,username,password_hash) VALUES('example-owner','Operator','example-hash')").Error)
				runtime := activity.NewRuntimeService(activity.NewActivityService(f.db, nil), restartRaceResolver{})
				generation, err := runtime.InitializeGeneration(t.Context())
				require.NoError(t, err)
				command := activity.RuntimeCommand{Store: activity.RuntimeStoreBinding{EnvironmentID: "example-host", RegistrationID: "example-host", ManagerID: "example-manager", StoreGeneration: generation, ExecutorGeneration: generation}, Key: uuid.NewString(), Actor: userDTO.Actor{ID: "example-owner", Username: "Operator"}, Operation: activity.RuntimeDeployProject, Target: activity.RuntimeTarget{Kind: "stack", ID: "example-stack"}}
				if kind == "policy" {
					store := policy.Store{DB: f.db.DB}
					one, err := store.Policy(t.Context(), "example-host", "example-host", "example-stack")
					require.NoError(t, err)
					one.Mode = "apply"
					one.ActorID = "example-owner"
					one.TargetIdentity = strings.Repeat("a", 64)
					require.NoError(t, store.SavePolicy(t.Context(), one, 0))
					one.Revision++
					child, _, err := store.PrepareChild(t.Context(), one, "example-job")
					require.NoError(t, err)
					command.Key = child.ID
					command.ProposedRunID = child.ID
					command.Operation = activity.RuntimeUpdateProject
					command.Options.Lifecycle = &updaterDTO.LifecycleIntent{Policy: &updaterDTO.PolicyBinding{PolicyID: one.ID, Revision: one.Revision, ChildID: child.ID, JobRunID: child.JobRunID}, Request: updaterDTO.LifecycleRequest{ExpectedRevision: strings.Repeat("b", 64)}, TargetIdentity: one.TargetIdentity, SavedRevision: strings.Repeat("b", 64), ObservedRevision: strings.Repeat("c", 64), BaselineRevision: strings.Repeat("d", 64)}
				}
				var status int
				var run dto.AgentRestartRun
				var runtimeErr error
				restart := func() { status, run = f.post(t) }
				admit := func() { _, runtimeErr = runtime.Admit(t.Context(), command) }
				switch winner {
				case "restart":
					restart()
					admit()
				case "runtime":
					admit()
					restart()
				default:
					gate := make(chan struct{})
					a, b := make(chan struct{}), make(chan struct{})
					go func() { defer close(a); <-gate; restart() }()
					go func() { defer close(b); <-gate; admit() }()
					close(gate)
					<-a
					<-b
				}
				if status == 202 {
					var p *dto.HostProblem
					require.ErrorAs(t, runtimeErr, &p)
					require.Equal(t, "host.restarting", p.Code)
					f.await(t, run.RunID, "awaiting")
					require.EqualValues(t, 1, f.postCalls.Load())
					_, err = runtime.Admit(t.Context(), command)
					require.ErrorAs(t, err, &p)
					require.Equal(t, "host.restarting", p.Code)
					var receipts int64
					require.NoError(t, f.db.Table("runtime_receipts").Count(&receipts).Error)
					require.Zero(t, receipts)
				} else {
					require.Equal(t, 409, status)
					require.NoError(t, runtimeErr)
					require.Zero(t, f.postCalls.Load())
					var active int64
					require.NoError(t, f.db.Model(&environment.AgentRestartRequest{}).Where("settled_at IS NULL").Count(&active).Error)
					require.Zero(t, active)
				}
			})
		}
	}
}
