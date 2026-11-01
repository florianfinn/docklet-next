// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package bootstrap

import (
	"context"
	"encoding/json"
	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/api/ws"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	wsHub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	apiKeyDTO "github.com/florianfinn/docklet-next/types/apikey"
	authDTO "github.com/florianfinn/docklet-next/types/auth"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/contract"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type m15Transport struct {
	fixture signInRoutingFixture
	hosts   *environment.EnvironmentService
	hub     *echoRouterForM15
	failure atomic.Bool
	empty   atomic.Bool
	reads   atomic.Int32
}

// A named handler keeps the transport fixture independent of Echo context internals.
type echoRouterForM15 struct{ http.Handler }

func newM15Transport(t *testing.T, mode string) *m15Transport {
	t.Helper()
	f := newSignInRoutingFixture(t)
	x := &m15Transport{fixture: f}
	var keyID string
	require.NoError(t, f.db.Table("api_keys").Select("id").Where("environment_id = ?", "example").Limit(1).Scan(&keyID).Error)
	token := "example"
	agentHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(middleware.HeaderAgentToken) != token && r.Header.Get(middleware.HeaderApiKey) != token {
			http.Error(w, "missing machine authority", 401)
			return
		}
		require.Empty(t, r.Header.Get("Cookie"))
		require.Empty(t, r.Header.Get("Authorization"))
		if r.URL.Path == "/api/app-version" {
			_ = json.NewEncoder(w).Encode(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/runtime") {
			x.reads.Add(1)
			if x.empty.Load() {
				_ = json.NewEncoder(w).Encode(base.ApiResponse[projectDTO.Details]{Success: true, Data: projectDTO.Details{ID: "project-uuid"}})
				return
			}
			if x.failure.Load() {
				http.Error(w, "runtime read unavailable", 503)
				return
			}
			_ = json.NewEncoder(w).Encode(base.ApiResponse[projectDTO.Details]{Success: true, Data: projectDTO.Details{ID: "project-uuid", RuntimeServices: []projectDTO.RuntimeService{{Name: "web", ContainerID: "canonical", ContainerName: "web-one"}}}})
			return
		}
		if r.URL.Path == "/api/environments/0/projects" {
			_ = json.NewEncoder(w).Encode(base.Paginated[projectDTO.Details]{Success: true, Data: []projectDTO.Details{{ID: "foreign", Name: "Alpha"}, {ID: "project-uuid", Name: "Zebra"}}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/vulnerabilities/scan") {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(base.ApiResponse[map[string]any]{Success: true, Data: body})
			return
		}
		if strings.Contains(r.URL.Path, "/ws/") {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
			if err != nil {
				return
			}
			defer conn.CloseNow()
			if conn.Write(r.Context(), websocket.MessageText, []byte("ready")) != nil {
				return
			}
			for {
				kind, data, err := conn.Read(r.Context())
				if err != nil {
					return
				}
				if strings.HasSuffix(r.URL.Path, "/terminal") && wsHub.IsTerminalClose(kind, data) {
					return
				}
				if conn.Write(r.Context(), kind, data) != nil {
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	})
	agent := httptest.NewServer(agentHandler)
	t.Cleanup(agent.Close)
	registry := edge.NewTunnelRegistry()
	endpoint := agent.URL
	if mode == "edge" {
		ready := make(chan struct{})
		var once sync.Once
		tunnelServer := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return "example", nil }, func(_ context.Context, _ string, online bool) {
			if online {
				once.Do(func() { close(ready) })
			}
		})
		tunnelServer.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return keyID, nil })
		e := echo.New()
		e.GET("/api/tunnel/connect", tunnelServer.HandleConnect)
		manager := httptest.NewServer(e)
		t.Cleanup(manager.Close)
		life, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		client := edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: token, EdgeReconnectInterval: 1, Port: strconv.Itoa(agent.Listener.Addr().(*net.TCPAddr).Port), Listen: "127.0.0.1"}, agentHandler)
		go func() { defer close(done); client.StartWithErrorChan(life, nil) }()
		t.Cleanup(func() {
			cancel()
			registry.Unregister("example")
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("Edge client did not stop")
			}
		})
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			t.Fatal("Edge registration missing")
		}
		endpoint = "edge://example"
	}
	previous := edge.GetRegistry()
	edge.SetDefaultRegistry(registry)
	t.Cleanup(func() { edge.SetDefaultRegistry(previous) })
	require.NoError(t, f.db.Model(&environment.Environment{}).Where("id = ?", "example").Updates(map[string]any{"api_url": endpoint, "is_edge": mode == "edge", "enabled": true, "access_token": token, "api_key_id": keyID}).Error)
	hosts := environment.NewEnvironmentService(f.db, agent.Client(), nil, nil, nil, f.keys)
	x.hosts = hosts
	deps := api.HandlerDeps{Auth: auth.New(f.svc, nil, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles), Environment: environment.New(hosts, nil, f.keys, nil, f.cfg, nil)}
	controller := resourceAccessController(deps)
	router := echo.New()
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.SetRequest(c.Request().WithContext(access.WithController(c.Request().Context(), controller)))
			return next(c)
		}
	})
	matcher := authz.NewPermissionMatcher()
	matcher.CollectFromHumaAPI(api.SetupAPIForSpec())
	ws.AddProxiedPermissions(matcher)
	authMiddleware := auth.NewAuthMiddleware(f.svc, f.cfg).WithApiKeyValidator(f.keys).WithPermissionResolver(f.roles)
	router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) { return endpoint, &token, true, nil }, createAuthValidatorInternal(deps), matcher, registry, middleware.WebSocketOriginValidator(f.cfg, authMiddleware.ValidateNonCookieCredential), middleware.EnvironmentProxySecurity{Snapshots: hosts.ReadSnapshots(), VulnerabilityEnabled: func(context.Context) bool { return true }, VulnerabilityIgnoreUnfixed: func() bool { return false }}))
	router.Any("/api/environments/:id/*", func(c *echo.Context) error { return c.NoContent(599) })
	x.hub = &echoRouterForM15{router}
	return x
}
func TestResourceRightsRealDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x := newM15Transport(t, mode)
			f := x.fixture
			u, err := f.users.CreateUser(t.Context(), &user.User{Username: "viewer"})
			require.NoError(t, err)
			require.NoError(t, f.users.SetAppAssignments(t.Context(), u.ID, []userDTO.AppAssignment{{EnvironmentID: "example", ProjectID: "project-uuid", Level: "restricted"}}))
			tokens, err := f.svc.CompleteLogin(t.Context(), u, authDTO.SessionMeta{}, "local")
			require.NoError(t, err)
			request := func(path string) *httptest.ResponseRecorder {
				r := httptest.NewRequest("GET", "/api/environments/example/"+path, nil)
				r.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: tokens.BrowserToken})
				w := httptest.NewRecorder()
				x.hub.ServeHTTP(w, r)
				return w
			}
			foreign, missing := request("projects/foreign"), request("projects/missing")
			require.Equal(t, 404, foreign.Code)
			require.Equal(t, foreign.Code, missing.Code)
			require.Equal(t, foreign.Body.String(), missing.Body.String())
			require.Contains(t, foreign.Body.String(), `"code":"access.not-found"`)
			require.Equal(t, 200, request("containers/canonical").Code)
			require.Equal(t, 200, request("containers/web-one").Code)
			require.Equal(t, 404, request("containers/standalone").Code)
			require.Greater(t, x.reads.Load(), int32(0))
			x.failure.Store(true)
			r := request("containers/canonical")
			require.Equal(t, 503, r.Code)
			require.Contains(t, r.Body.String(), `"code":"access.unavailable"`)
			x.failure.Store(false)
			x.empty.Store(true)
			empty := request("containers/canonical")
			require.Equal(t, 404, empty.Code)
			require.Contains(t, empty.Body.String(), `"code":"access.not-found"`)
			x.empty.Store(false)
			list := request("projects?limit=1")
			require.Equal(t, 200, list.Code)
			require.Contains(t, list.Body.String(), "project-uuid")
			require.NotContains(t, list.Body.String(), "foreign")
		})
	}
}
func TestUserListKeepsAdministratorSnapshotConcurrent(t *testing.T) {
	x := newM15Transport(t, "direct")
	f := x.fixture
	u, err := f.users.CreateUser(t.Context(), &user.User{Username: "viewer"})
	require.NoError(t, err)
	require.NoError(t, f.users.SetAppAssignments(t.Context(), u.ID, []userDTO.AppAssignment{{EnvironmentID: "example", ProjectID: "project-uuid", Level: "restricted"}}))
	tokens, err := f.svc.CompleteLogin(t.Context(), u, authDTO.SessionMeta{}, "local")
	require.NoError(t, err)
	key := hoststate.Key{HostID: "example", List: "/projects?limit=1"}
	adminReq := httptest.NewRequest("GET", "/api/environments/example/projects?limit=1", nil)
	adminReq.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: f.tokens.BrowserToken})
	adminResp := httptest.NewRecorder()
	x.hub.ServeHTTP(adminResp, adminReq)
	require.Equal(t, 200, adminResp.Code)
	initial, ok := x.hosts.ReadSnapshots().Get(key)
	require.True(t, ok)
	baseline := initial.Data
	require.Contains(t, string(baseline), "foreign")

	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			r := httptest.NewRequest("GET", "/api/environments/example/projects?limit=1", nil)
			r.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: tokens.BrowserToken})
			w := httptest.NewRecorder()
			x.hub.ServeHTTP(w, r)
			require.Equal(t, 200, w.Code)
			require.NotContains(t, w.Body.String(), "foreign")
		})
		wg.Go(func() {
			r := httptest.NewRequest("GET", "/api/environments/example/projects?limit=2", nil)
			r.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: f.tokens.BrowserToken})
			w := httptest.NewRecorder()
			x.hub.ServeHTTP(w, r)
			require.Equal(t, 200, w.Code)
			require.Contains(t, w.Body.String(), "foreign")
			snap, ok := x.hosts.ReadSnapshots().Get(key)
			require.True(t, ok)
			require.JSONEq(t, string(baseline), string(snap.Data))
		})
	}
	wg.Wait()
	snap, _ := x.hosts.ReadSnapshots().Get(key)
	require.JSONEq(t, string(baseline), string(snap.Data))
}

func TestUserStreamsRevokeRealDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x := newM15Transport(t, mode)
			f := x.fixture
			f.cfg.AuthTrustedProxies = "127.0.0.1"
			f.cfg.AuthProxyAdminUsers = new("Operator")
			u, err := f.users.CreateUser(t.Context(), &user.User{Username: "viewer"})
			require.NoError(t, err)
			assignment := []userDTO.AppAssignment{{EnvironmentID: "example", ProjectID: "project-uuid", Level: "restricted"}}
			tokens, err := f.svc.CompleteLogin(t.Context(), u, authDTO.SessionMeta{}, "local")
			require.NoError(t, err)
			key, err := f.keys.CreatePersonalApiKey(t.Context(), u.ID, apiKeyDTO.CreateUserApiKey{Name: "viewer"})
			require.NoError(t, err)
			hub := httptest.NewServer(x.hub)
			defer hub.Close()
			for _, credential := range []string{"cookie", "personal-key", "proxy"} {
				t.Run(credential, func(t *testing.T) {
					f.cfg.AuthMode = "local"
					if credential == "proxy" {
						f.cfg.AuthMode = "proxy"
					}
					require.NoError(t, f.users.SetAppAssignments(t.Context(), u.ID, assignment))
					ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
					defer cancel()
					connections := []*websocket.Conn{}
					for _, kind := range []string{"logs", "stats", "terminal"} {
						headers := http.Header{"Origin": {f.cfg.AppUrl}}
						switch credential {
						case "cookie":
							r := &http.Request{Header: headers}
							r.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: tokens.BrowserToken})
						case "personal-key":
							headers.Set(middleware.HeaderApiKey, key.Key)
						case "proxy":
							headers.Set("Remote-User", u.Username)
						}
						conn, response, err := websocket.Dial(ctx, edge.HTTPToWebSocketURL(hub.URL)+"/api/environments/example/ws/containers/canonical/"+kind, &websocket.DialOptions{HTTPHeader: headers})
						if response != nil && err != nil {
							t.Logf("upgrade status %d", response.StatusCode)
						}
						require.NoError(t, err)
						connections = append(connections, conn)
						defer conn.CloseNow()
						_, body, err := conn.Read(ctx)
						require.NoError(t, err)
						require.Equal(t, "ready", string(body))
					}
					require.NoError(t, f.users.SetAppAssignments(t.Context(), u.ID, nil))
					for _, conn := range connections {
						_ = conn.Write(ctx, websocket.MessageText, []byte("after revoke"))
						check, stop := context.WithTimeout(ctx, 2*time.Second)
						closed := false
						for range 3 {
							_, body, err := conn.Read(check)
							if err != nil {
								closed = true
								break
							}
							require.NotEqual(t, "after revoke", string(body), "a revoked stream may finish its terminal protocol but cannot echo new input")
						}
						require.True(t, closed)
						require.NoError(t, check.Err(), "revocation must close rather than expire")
						stop()
					}
					if credential == "personal-key" {
						require.NoError(t, f.users.SetAppAssignments(t.Context(), u.ID, assignment))
						_, _, err := f.keys.ValidateApiKeyWithID(t.Context(), key.Key)
						require.NoError(t, err)
					}
				})
			}
		})
	}
}

func TestRemoteUserScanOriginIsManualAndMachineDispatchStaysScheduled(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x := newM15Transport(t, mode)
			r := httptest.NewRequest("POST", "/api/environments/example/vulnerabilities/scan", strings.NewReader(`{"scope":"all","origin":"scheduled"}`))
			r.Header.Set("Content-Type", "application/json")
			r.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: x.fixture.tokens.BrowserToken})
			w := httptest.NewRecorder()
			x.hub.ServeHTTP(w, r)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), `"origin":"manual"`)
			require.NotContains(t, w.Body.String(), "scheduled")
			var out base.ApiResponse[map[string]any]
			err := x.hosts.ProxyJSONRequest(t.Context(), "example", "POST", "/api/environments/0/vulnerabilities/scan", []byte(`{"scope":"all","origin":"scheduled"}`), &out)
			require.NoError(t, err)
			require.Equal(t, "scheduled", out.Data["origin"])
		})
	}
}

func TestRejectedUserKeyCannotBecomeMachineDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x := newM15Transport(t, mode)
			f := x.fixture
			owner, err := f.users.CreateUser(t.Context(), &user.User{Username: "key-owner"})
			require.NoError(t, err)
			require.NoError(t, f.users.SetAdministrator(t.Context(), "operator", owner.ID, true))
			key, err := f.keys.CreateApiKey(t.Context(), owner.ID, authz.SudoPermissionSet(), apiKeyDTO.CreateApiKey{Name: "legacy-scoped", Permissions: []apiKeyDTO.PermissionGrant{{Permission: authz.PermProjectsRead, EnvironmentID: new("example")}}})
			require.NoError(t, err)
			// Preserve a legacy environment-scoped user key; this is not a pairing key.
			require.NoError(t, f.db.Table("api_keys").Where("id = ?", key.ID).Update("environment_id", "example").Error)
			request := func() *httptest.ResponseRecorder {
				r := httptest.NewRequest("GET", "/api/environments/example/projects/foreign", nil)
				r.Header.Set(middleware.HeaderApiKey, key.Key)
				w := httptest.NewRecorder()
				x.hub.ServeHTTP(w, r)
				return w
			}
			require.Equal(t, 200, request().Code)
			require.NoError(t, f.users.SetAdministrator(t.Context(), "operator", owner.ID, false))
			require.Equal(t, 401, request().Code, "a rejected user key must not become an environment credential")
			require.NoError(t, f.users.SetDisabled(t.Context(), owner.ID, true))
			require.Equal(t, 401, request().Code)
		})
	}
}
