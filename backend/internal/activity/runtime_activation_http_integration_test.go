// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Actual Fx runtime construction, registered Huma/Echo APIs and Direct/Edge SDK matrix.

package activity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/di"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

type activationFixture struct {
	services *runtimeServicesFixture
	env      *environment.EnvironmentService
	registry *edge.TunnelRegistry
	router   *echo.Echo
	server   *httptest.Server
	http     *activity.RuntimeHTTPService
	receiver *activity.RuntimeReceiver
	remote   *activity.RuntimeRemoteCoordinator
	cfg      *config.Config
}

type activationSetup struct {
	AdmissionBudget    time.Duration
	BeforeRouter       func(*echo.Echo)
	Configure          func(*config.Config)
	BeforeStart        func(*runtimeServicesFixture, *activity.RuntimeHTTPService, *activity.RuntimeReceiver)
	ExpectedStartError string
}

func activationApp(t *testing.T, agent bool, token string, setup ...activationSetup) *activationFixture {
	t.Helper()
	f := newRuntimeServicesFixture(t, false)
	cfg := &config.Config{AgentMode: agent, AgentToken: token, AuthMode: "none", Environment: config.AppEnvironmentTest}
	var options activationSetup
	if len(setup) > 0 {
		options = setup[0]
	}
	if options.Configure != nil {
		options.Configure(cfg)
	}
	roles := role.NewRoleService(f.db)
	require.NoError(t, roles.EnsureBuiltInRoles(t.Context()))
	users := user.NewUserService(f.db, roles, session.RevokeAllUserSessionsExceptInDB)
	require.NoError(t, users.CreateDefaultAdmin(t.Context()))
	keys := apikey.NewApiKeyService(f.db, users, roles)
	authentication := auth.NewAuthService(users, f.settings, nil, session.NewSessionService(f.db), cfg)
	if !agent {
		require.NoError(t, authentication.InitializeSignIn(t.Context()))
	}
	client := &http.Client{Timeout: 30 * time.Second}
	environments := environment.NewEnvironmentService(f.db, client, f.docker, nil, f.settings, keys)
	registry := edge.NewTunnelRegistry()
	local := f.activity
	var runtime *activity.RuntimeService
	var httpService *activity.RuntimeHTTPService
	var receiver *activity.RuntimeReceiver
	var remote *activity.RuntimeRemoteCoordinator
	app := fx.New(fx.NopLogger, fx.Supply(f.db, local, f.projects, f.containers, environments, f.settings, cfg, client, registry),
		fx.Provide(func() context.Context { return t.Context() }), di.RuntimeOptions, fx.Populate(&runtime, &httpService, &receiver, &remote))
	require.NoError(t, app.Err())
	activity.UseImmediateRuntimeReconcileScheduleForTest(remote)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		require.NoError(t, app.Stop(ctx))
	})
	f.activity, f.runtime, f.store = local, runtime, runtime.LocalStore()
	if options.BeforeStart != nil {
		options.BeforeStart(f, httpService, receiver)
	}
	startErr := app.Start(t.Context())
	if options.ExpectedStartError != "" {
		require.ErrorContains(t, startErr, options.ExpectedStartError)
	} else {
		require.NoError(t, startErr)
	}
	router := echo.New()
	budget := options.AdmissionBudget
	if budget == 0 {
		budget = activity.RuntimeAdmissionLimit
	}
	if options.BeforeRouter != nil {
		options.BeforeRouter(router)
	}
	router.Use(activity.RuntimeReceiverMiddleware(agent, budget))
	group := router.Group("/api")
	group.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(ctx context.Context, id string) (string, *string, bool, error) {
		env, err := environments.GetEnvironmentByID(ctx, id)
		if err != nil {
			return "", nil, false, err
		}
		return env.ApiUrl, env.AccessToken, env.Enabled, nil
	}, nil, nil, registry, nil, middleware.EnvironmentProxySecurity{AgentAccess: environments.AgentAccess}))
	api.SetupAPI(router, group, handlerutil.NewActivityAppContext(t.Context()), cfg, api.HandlerDeps{
		Runtime: runtime, RuntimeHTTP: httpService, RuntimeReceiver: receiver,
		Auth: auth.New(authentication, users, f.settings), Role: role.New(roles), ApiKey: apikey.New(keys),
		Activity:    activity.New(local, activity.EnvironmentDependencies{}),
		Environment: environment.New(environments, f.settings, keys, nil, cfg, local),
		Project:     project.New(f.projects, local), Container: container.New(f.containers, f.docker, f.settings, local),
		System:  system.New(nil, f.docker, environments, local, cfg),
		Version: version.NewVersionService(client, true, "0.0.0", "", nil, f.docker, nil, f.settings),
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return &activationFixture{f, environments, registry, router, server, httpService, receiver, remote, cfg}
}

func activationRemote(t *testing.T, hub, agent *activationFixture, transport string, registration ...string) string {
	t.Helper()
	id := "example-host"
	if len(registration) > 0 {
		id = registration[0]
	}
	token := agent.cfg.AgentToken
	endpoint := agent.server.URL
	if transport == "edge" {
		endpoint = "edge://example"
	}
	env := &environment.Environment{ID: id, Name: "Example", Enabled: true, ApiUrl: endpoint, IsEdge: transport == "edge", AccessToken: &token, Status: "offline"}
	require.NoError(t, hub.services.db.Create(env).Error)
	roles := role.NewRoleService(hub.services.db)
	users := user.NewUserService(hub.services.db, roles, session.RevokeAllUserSessionsExceptInDB)
	keys := apikey.NewApiKeyService(hub.services.db, users, roles)
	key, err := keys.CreateEnvironmentApiKey(t.Context(), id)
	require.NoError(t, err)
	require.NoError(t, hub.services.db.Model(env).Update("api_key_id", key.ID).Error)
	if transport != "edge" {
		return id
	}
	edge.SetDefaultRegistry(hub.registry)
	t.Cleanup(func() { edge.ClearDefaultRegistry(hub.registry) })
	connected := make(chan struct{})
	var once sync.Once
	server := edge.NewTunnelServerWithRegistry(hub.registry, func(_ context.Context, credential string) (string, error) {
		if credential != token {
			return "", errors.New("invalid synthetic credential")
		}
		return id, nil
	}, func(_ context.Context, _ string, online bool) {
		if online {
			once.Do(func() { close(connected) })
		}
	})
	server.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return key.ID, nil })
	router := echo.New()
	router.GET("/api/tunnel/connect", server.HandleConnect)
	manager := httptest.NewServer(router)
	t.Cleanup(manager.Close)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	client := edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, AgentToken: token, ManagerApiUrl: manager.URL}, agent.router)
	go func() { defer close(done); client.StartWithErrorChan(ctx, nil) }()
	t.Cleanup(func() {
		cancel()
		hub.registry.Unregister(id)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Edge runtime client did not join")
		}
	})
	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("actual Edge tunnel did not connect")
	}
	return id
}

func activationOperation(f *runtimeServicesFixture, op activity.RuntimeOperation) (string, []byte) {
	if strings.Contains(string(op), "project") {
		f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
		f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
		suffix := map[activity.RuntimeOperation]string{activity.RuntimeDeployProject: "up", activity.RuntimeDownProject: "down", activity.RuntimeRestartProject: "restart", activity.RuntimeRedeployProject: "redeploy"}[op]
		if op == activity.RuntimeDownProject || op == activity.RuntimeRestartProject {
			f.http.setComposeContainer(true)
		}
		var body []byte
		if op == activity.RuntimeDeployProject || op == activity.RuntimeRedeployProject {
			body = []byte(`{"pullPolicy":"never"}`)
		}
		return "/projects/example-project/" + suffix, body
	}
	return "/containers/" + runtimeContainerID + "/" + strings.TrimSuffix(string(op), "-container"), nil
}

func activationCall(t *testing.T, f *activationFixture, path, key, prefer string, body []byte) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, f.server.URL+path, bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	if prefer != "" {
		request.Header.Set("Prefer", prefer)
	}
	response, err := f.server.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, data
}

func TestRuntimeActivationRegisteredSevenModesAndTransports(t *testing.T) {
	for _, transport := range []string{"local", "direct", "edge"} {
		for _, op := range []activity.RuntimeOperation{activity.RuntimeDeployProject, activity.RuntimeDownProject, activity.RuntimeRestartProject, activity.RuntimeRedeployProject, activity.RuntimeStartContainer, activity.RuntimeStopContainer, activity.RuntimeRestartContainer} {
			for _, mode := range []string{"legacy", "async"} {
				t.Run(transport+"/"+string(op)+"/"+mode, func(t *testing.T) {
					hub := activationApp(t, false, "")
					executor := hub
					env := "0"
					if transport != "local" {
						executor = activationApp(t, true, uuid.NewString())
						env = activationRemote(t, hub, executor, transport)
						_, compatibilityErr := hub.env.CheckAgentCompatibility(t.Context(), env)
						require.NoError(t, compatibilityErr)
					}
					suffix, body := activationOperation(executor.services, op)
					prefer := ""
					if mode == "async" {
						prefer = "respond-async"
					}
					key := uuid.NewString()
					status, data := activationCall(t, hub, "/api/environments/"+env+suffix, key, prefer, body)
					var runID string
					if mode == "async" {
						require.Equal(t, 202, status, string(data))
						var admitted struct {
							Success bool                           `json:"success"`
							Data    activitytypes.RuntimeAdmission `json:"data"`
						}
						require.NoError(t, json.Unmarshal(data, &admitted))
						require.True(t, admitted.Success)
						runID = admitted.Data.ActivityID
						require.Equal(t, runID, admitted.Data.Activity.ID)
						require.Equal(t, activitytypes.CodeRunStarted, admitted.Data.Code)
					} else if op == activity.RuntimeDeployProject || op == activity.RuntimeRedeployProject {
						require.Equal(t, 200, status, string(data))
						lines := strings.Split(strings.TrimSpace(string(data)), "\n")
						var started struct {
							ActivityID string `json:"activityId"`
						}
						require.NoError(t, json.Unmarshal([]byte(lines[0]), &started))
						runID = started.ActivityID
						require.Equal(t, `{"done":true}`, lines[len(lines)-1], string(data))
						require.NotContains(t, string(data), `"code":"runtime.failed"`)
					} else {
						require.Equal(t, 200, status, string(data))
						var response struct {
							Success bool `json:"success"`
							Data    struct {
								ActivityID string `json:"activityId"`
								Message    string `json:"message"`
							} `json:"data"`
						}
						require.NoError(t, json.Unmarshal(data, &response))
						require.True(t, response.Success)
						require.NotEmpty(t, response.Data.Message)
						runID = response.Data.ActivityID
					}
					require.NotEmpty(t, runID)
					ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
					defer cancel()
					ref, found, err := hub.services.runtime.Reference(ctx, env, runID)
					require.NoError(t, err)
					require.True(t, found)
					if transport == "local" {
						snapshot, err := hub.services.runtime.WaitLocal(ctx, ref)
						require.NoError(t, err)
						require.Equal(t, activitytypes.StatusSuccess, snapshot.Status, executor.services.debugEvents())
					} else {
						require.NoError(t, hub.remote.WaitDispatch(ctx, runID))
						localRef, found, err := executor.services.runtime.Reference(ctx, "0", runID)
						require.NoError(t, err)
						require.True(t, found)
						snapshot, err := executor.services.runtime.WaitLocal(ctx, localRef)
						require.NoError(t, err)
						require.Equal(t, activitytypes.StatusSuccess, snapshot.Status, executor.services.debugEvents())
						_, err = hub.remote.Refresh(ctx, ref)
						require.NoError(t, err)
					}
					effects := runtimeEffectEvents(executor.services.http.snapshot())
					require.NotEmpty(t, effects, executor.services.debugEvents())
					retryStatus, retry := activationCall(t, hub, "/api/environments/"+env+suffix, key, "respond-async", body)
					require.Equal(t, 200, retryStatus, string(retry))
					require.Contains(t, string(retry), runID)
					if transport != "local" {
						require.NoError(t, hub.remote.WaitDispatch(ctx, runID))
					}
					require.Equal(t, effects, runtimeEffectEvents(executor.services.http.snapshot()), "a retained HTTP retry must not execute again")
				})
			}
		}
	}
}

func TestRuntimeActivationOfflineRetainedContainerAndChangedIntent(t *testing.T) {
	hub := activationApp(t, false, "")
	path := "/api/environments/0/containers/" + runtimeContainerID + "/start"
	key := uuid.NewString()
	status, body := activationCall(t, hub, path, key, "respond-async", nil)
	require.Equal(t, 202, status, string(body))
	var admitted struct {
		Data activitytypes.RuntimeAdmission `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &admitted))
	ref, found, err := hub.services.runtime.Reference(t.Context(), "0", admitted.Data.ActivityID)
	require.NoError(t, err)
	require.True(t, found)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	snapshot, err := hub.services.runtime.WaitLocal(ctx, ref)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, snapshot.Status)
	before := hub.services.http.snapshot()
	hub.services.http.server.Close()
	status, body = activationCall(t, hub, path, key, "respond-async", nil)
	require.Equal(t, 200, status, string(body))
	require.Contains(t, string(body), ref.RunID)
	status, body = activationCall(t, hub, strings.TrimSuffix(path, "start")+"restart", key, "respond-async", nil)
	require.Equal(t, 409, status, string(body))
	require.Contains(t, string(body), "run.request-mismatch")
	require.NotContains(t, string(body), ref.RunID)
	require.Equal(t, before, hub.services.http.snapshot(), "retained compatibility lookup must precede every fresh Docker inspection")
}

func TestRuntimeActivationFreshParentRelabelIsNotKeyMismatch(t *testing.T) {
	hub := activationApp(t, false, "")
	f := hub.services
	f.project("example-alpha", "alpha", "name: alpha\nservices:\n  app:\n    image: registry.example.com/app:1\n")
	f.project("example-beta", "beta", "name: beta\nservices:\n  app:\n    image: registry.example.com/app:1\n")
	f.http.setLabels(map[string]string{"com.docker.compose.project": "alpha", "com.docker.compose.service": "app"})
	var inspected atomic.Int32
	f.http.mu.Lock()
	f.http.onCall = func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/"+runtimeContainerID+"/json") && inspected.Add(1) == 1 {
			f.http.setLabels(map[string]string{"com.docker.compose.project": "beta", "com.docker.compose.service": "app"})
		}
		return false
	}
	f.http.mu.Unlock()
	status, body := activationCall(t, hub, "/api/environments/0/containers/"+runtimeContainerID+"/start", uuid.NewString(), "respond-async", nil)
	require.Equal(t, 409, status, string(body))
	require.Contains(t, string(body), "runtime.target-changed")
	require.NotContains(t, string(body), "run.request-mismatch")
	var receipts int64
	require.NoError(t, f.db.Model(&activity.RuntimeReceipt{}).Count(&receipts).Error)
	require.Zero(t, receipts)
	require.Empty(t, runtimeEffectEvents(f.http.snapshot()))
	require.EqualValues(t, 2, inspected.Load())
}
