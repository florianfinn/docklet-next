// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/contract"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	versionDTO "github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

const testDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type updateTransportFixture struct {
	hub, agent   *Service
	db           *database.DB
	remote       environment.Environment
	daemon       *updateDaemon
	version      atomic.Value
	posts        atomic.Int32
	loseReply    atomic.Bool
	post         chan struct{}
	handler      http.Handler
	holdCheck    atomic.Bool
	checkEntered chan struct{}
	checkRelease chan struct{}
}
type updateDaemon struct {
	mu        sync.Mutex
	data      string
	created   chan struct{}
	creates   []map[string]any
	mutations []string
	envFile   string // optional Compose environment_file label
}

func (d *updateDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Api-Version", "1.56")
	path := r.URL.Path
	if strings.HasPrefix(path, "/v1.") {
		_, path, _ = strings.Cut(path[1:], "/")
		path = "/" + path
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		d.mutations = append(d.mutations, r.Method+" "+path)
	}
	switch {
	case path == "/_ping":
		_, _ = w.Write([]byte("OK"))
	case path == "/containers/current/json":
		labels := map[string]string{api.ProjectLabel: "example", api.ServiceLabel: "agent", api.WorkingDirLabel: "/opt/example", api.ConfigFilesLabel: "/opt/example/compose.yaml"}
		if d.envFile != "" {
			labels[api.EnvironmentFileLabel] = d.envFile
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": "current", "Config": map[string]any{"Image": repository("agent") + ":1.0.0", "Labels": labels}, "Mounts": []any{map[string]any{"Type": "bind", "Source": "/host/docker.sock", "Destination": "/var/run/docker.sock", "RW": true}, map[string]any{"Type": "bind", "Source": "/host/data", "Destination": d.data, "RW": true}}})
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/containers/docklet-self-update-"):
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": "helper", "State": map[string]any{"Running": true}})
	case path == "/images/create":
		_, _ = w.Write([]byte("{}\n"))
	case path == "/containers/create":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		d.creates = append(d.creates, body)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"Id":"helper","Warnings":[]}`))
	case r.Method == http.MethodPost && path == "/containers/helper/start":
		w.WriteHeader(204)
		select {
		case d.created <- struct{}{}:
		default:
		}
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/containers/docklet-self-update-"):
		w.WriteHeader(204)
	default:
		http.Error(w, "unexpected Engine request", 500)
	}
}
func updateDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "updates.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}
func newUpdateTransport(t *testing.T, isEdge bool) *updateTransportFixture {
	t.Helper()
	f := &updateTransportFixture{db: updateDB(t), post: make(chan struct{}, 10), checkEntered: make(chan struct{}), checkRelease: make(chan struct{})}
	agentDB := updateDB(t)
	cfg := &config.Config{AgentMode: true, AgentToken: "example", DockerHost: "unix:///var/run/docker.sock", AuthMode: "local", UIConfigurationDisabled: true}
	f.daemon = &updateDaemon{data: t.TempDir(), created: make(chan struct{}, 5)}
	engine := httptest.NewServer(f.daemon)
	t.Cleanup(engine.Close)
	cli, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(engine.URL, "http://")), client.WithAPIVersion("1.56"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	docker := dockerInternal.NewDockerClientService(t.Context(), agentDB, nil, nil).WithClient(cli)
	f.agent = NewService(agentDB, docker, nil, cfg)
	f.agent.data = f.daemon.data
	f.agent.selfID = func() (string, error) { return "current", nil }
	f.version.Store("1.0.0")
	f.agent.runningVersion = func() string { return f.version.Load().(string) }
	require.NoError(t, f.agent.Start(t.Context()))
	t.Cleanup(f.agent.Stop)
	setting, err := settings.NewSettingsService(t.Context(), agentDB)
	require.NoError(t, err)
	t.Cleanup(func() { _ = setting.Stop(context.WithoutCancel(t.Context())) })
	users := user.NewUserService(agentDB, role.NewRoleService(agentDB), session.RevokeAllUserSessionsExceptInDB)
	authService := auth.NewAuthService(users, setting, nil, session.NewSessionService(agentDB), cfg)
	agentRouter := echo.New()
	agentAPI := humaecho.NewWithGroup(agentRouter, agentRouter.Group("/api"), huma.DefaultConfig("Update receiver", "1.0.0"))
	agentAPI.UseMiddleware(auth.NewHumaMiddleware(agentAPI, authService, nil, nil, nil, cfg))
	agentAPI.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		if ctx.Header("X-Example-Nonadmin") == "true" {
			next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, &authz.PermissionSet{})))
			return
		}
		next(ctx)
	})
	RegisterAgentRoutes(agentAPI, NewHandler(f.agent, nil, cfg, t.Context()))
	huma.Get(agentAPI, "/app-version", func(context.Context, *struct{}) (*struct{ Body versionDTO.Info }, error) {
		return &struct{ Body versionDTO.Info }{Body: versionDTO.Info{AgentContractVersion: contract.MinimumAgentVersion - 1, AgentContractFingerprint: "old-contract"}}, nil
	})
	huma.Head(agentAPI, "/environments/{id}/system/health", func(context.Context, *struct {
		ID string `path:"id"`
	}) (*struct{}, error) {
		return nil, nil
	}, func(op *huma.Operation) { op.Security = handlerutil.DefaultOperationSecurity() })
	f.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/environments/0/system/upgrade/check" && f.holdCheck.CompareAndSwap(true, false) {
			capture := httptest.NewRecorder()
			agentRouter.ServeHTTP(capture, r)
			close(f.checkEntered)
			select {
			case <-f.checkRelease:
			case <-r.Context().Done():
				return
			}
			for key, values := range capture.Header() {
				w.Header()[key] = values
			}
			w.WriteHeader(capture.Code)
			_, _ = w.Write(capture.Body.Bytes())
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/environments/0/system/upgrade" {
			f.posts.Add(1)
			if f.loseReply.CompareAndSwap(true, false) {
				capture := httptest.NewRecorder()
				agentRouter.ServeHTTP(capture, r)
				w.WriteHeader(503)
				select {
				case f.post <- struct{}{}:
				default:
				}
				return
			}
		}
		agentRouter.ServeHTTP(w, r)
	})
	direct := httptest.NewServer(f.handler)
	t.Cleanup(direct.Close)
	registry := edge.NewTunnelRegistry()
	previous := edge.GetRegistry()
	edge.SetDefaultRegistry(registry)
	t.Cleanup(func() { edge.SetDefaultRegistry(previous) })
	f.remote = environment.Environment{BaseModel: database.BaseModel{ID: "example-remote"}, Name: "Example host", ApiUrl: direct.URL, IsEdge: isEdge, Enabled: true, AccessToken: new(cfg.AgentToken), ApiKeyID: new("example-key"), CredentialRevision: 1}
	if isEdge {
		connected := make(chan struct{})
		var once sync.Once
		server := edge.NewTunnelServerWithRegistry(registry, func(_ context.Context, token string) (string, error) {
			if token != cfg.AgentToken {
				return "", errors.New("invalid synthetic credential")
			}
			return f.remote.ID, nil
		}, func(_ context.Context, _ string, online bool) {
			if online {
				once.Do(func() { close(connected) })
			}
		})
		server.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return "example-key", nil })
		router := echo.New()
		router.GET("/api/tunnel/connect", server.HandleConnect)
		manager := httptest.NewServer(router)
		t.Cleanup(manager.Close)
		life, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		agent := edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: cfg.AgentToken, EdgeReconnectInterval: 1}, f.handler)
		go func() { defer close(done); agent.StartWithErrorChan(life, nil) }()
		t.Cleanup(func() {
			cancel()
			registry.Unregister(f.remote.ID)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("Edge worker did not join")
			}
		})
		select {
		case <-connected:
		case <-time.After(5 * time.Second):
			t.Fatal("Edge registration missing")
		}
		f.remote.ApiUrl = "edge://example-remote"
	}
	require.NoError(t, f.db.Exec("INSERT INTO api_keys(id,name,key_hash,key_prefix,kind) VALUES(?,?,?,?,?)", "example-key", "Example key", "example-hash", "example", "environment").Error)
	require.NoError(t, f.db.Create(&f.remote).Error)
	environments := environment.NewEnvironmentService(f.db, http.DefaultClient, nil, nil, nil, nil)
	f.hub = NewService(f.db, nil, nil, &config.Config{})
	f.hub.SetEnvironments(environments)
	f.hub.runningVersion = func() string { return "1.0.1" }
	require.NoError(t, f.hub.Start(t.Context()))
	t.Cleanup(f.hub.Stop)
	return f
}
func (f *updateTransportFixture) hubRouter(t *testing.T) http.Handler {
	t.Helper()
	router := echo.New()
	router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) {
		return f.remote.ApiUrl, f.remote.AccessToken, true, nil
	}, func(context.Context, *echo.Context) (*authz.PermissionSet, *userDTO.Actor, bool) {
		return authz.SudoPermissionSet(), &userDTO.Actor{ID: "operator", Username: "operator"}, true
	}, nil, edge.GetRegistry(), nil))
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("Update owner", "1.0.1"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		value := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
		value = context.WithValue(value, userDTO.CurrentUserContextKey{}, &userDTO.Actor{ID: "operator", Username: "operator"})
		next(huma.WithContext(ctx, value))
	})
	RegisterRoutes(api, NewHandler(f.hub, f.hub.environments, &config.Config{}, t.Context()))
	return router
}
