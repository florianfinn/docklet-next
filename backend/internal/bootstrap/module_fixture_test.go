// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original real-router module fixture with Direct and Edge senders and receivers. M27 proxy contract foundation (#473/#474).

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/modules"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/features"
	moduleDTO "github.com/florianfinn/docklet-next/types/modules"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

const m27HostProbe = "/api/environments/example/modules/proxy/data"

type m27ModuleFixture struct {
	agentProblem               atomic.Pointer[moduleDTO.Problem]
	f                          signInRoutingFixture
	hub                        *echo.Echo
	agent                      http.Handler
	agentIntercept             func(http.ResponseWriter, *http.Request, http.Handler)
	hubSettings, agentSettings *settings.SettingsService
	manager, agentManager      *modules.Service
	hosts                      *environment.EnvironmentService
	hubAPI                     huma.API
	calls                      atomic.Int32
	hubCalls                   atomic.Int32
	online                     atomic.Bool
	version                    atomic.Int32
	wrongFingerprint           atomic.Bool
	tunnel                     *edge.AgentTunnel
	agentServer                *httptest.Server
}

func m27Settings(t *testing.T, db *database.DB) *settings.SettingsService {
	t.Helper()
	s, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Stop(context.WithoutCancel(t.Context()))) })
	require.NoError(t, s.EnsureDefaultSettings(t.Context()))
	return s
}
func m27DB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "module.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

// The test operation uses the existing bounded module cleanup command path. Its
// test router deliberately makes it gated work, rather than management cleanup.
func newM27ModuleFixture(t *testing.T, mode string, probe bool, extend ...func(*m27ModuleFixture, huma.API)) *m27ModuleFixture {
	t.Helper()
	x := &m27ModuleFixture{f: newSignInRoutingFixture(t)}
	x.online.Store(true)
	x.version.Store(contract.AgentHubVersion)
	x.hubSettings = m27Settings(t, x.f.db)
	x.agentSettings = m27Settings(t, m27DB(t))
	x.manager = modules.New(x.hubSettings)
	x.agentManager = modules.New(x.agentSettings)
	agent := echo.New()
	agentConfig := huma.DefaultConfig("module agent", "test")
	agentConfig.Security = handlerutil.DefaultOperationSecurity()
	agentConfig.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(t reflect.Type, hint string) string {
		n := huma.DefaultSchemaNamer(t, hint)
		if strings.Contains(t.PkgPath(), "/types/proxy") {
			return "Proxy" + n
		}
		return n
	})
	agentAPI := humaecho.NewWithGroup(agent, agent.Group("/api"), agentConfig)
	cfg := &config.Config{AgentMode: true, AgentToken: "example"}
	agentAPI.UseMiddleware(auth.NewHumaMiddleware(agentAPI, &auth.AuthService{}, nil, nil, nil, cfg))
	agentAPI.UseMiddleware(middleware.AccessMiddleware(agentAPI))
	settings.RegisterSettings(agentAPI, x.agentSettings, nil, cfg)
	modules.RegisterRoutes(agentAPI, x.agentManager, nil, true)
	if probe {
		registerM27Probe(t, agentAPI, x.agentSettings, true, "/environments/{id}/modules/proxy/data", "m27-host-probe", &x.calls, func() error {
			if p := x.agentProblem.Load(); p != nil {
				return p
			}
			return nil
		})
	}
	huma.Get(agentAPI, "/app-version", func(context.Context, *struct{}) (*struct{ Body version.Info }, error) {
		fingerprint := contract.AgentHubFingerprint
		if x.version.Load() == 18 {
			fingerprint = contract.AgentHubV18Fingerprint
		}
		if x.wrongFingerprint.Load() {
			fingerprint = "different"
		}
		return &struct{ Body version.Info }{Body: version.Info{AgentContractVersion: int(x.version.Load()), AgentContractFingerprint: fingerprint}}, nil
	})
	huma.Head(agentAPI, "/environments/{id}/system/health", func(context.Context, *struct {
		ID string `path:"id"`
	}) (*struct{}, error) {
		return nil, nil
	})
	huma.Get(agentAPI, "/environments/{id}/containers", func(context.Context, *struct {
		ID string `path:"id"`
	}) (*struct{ Body map[string]any }, error) {
		return &struct{ Body map[string]any }{Body: map[string]any{"success": true}}, nil
	})
	for _, setup := range extend {
		setup(x, agentAPI)
	}
	x.agent = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !x.online.Load() {
			w.WriteHeader(503)
			return
		}
		if x.agentIntercept != nil {
			x.agentIntercept(w, r, agent)
		} else {
			agent.ServeHTTP(w, r)
		}
	})
	server := httptest.NewServer(x.agent)
	t.Cleanup(server.Close)
	x.agentServer = server
	var keyID string
	require.NoError(t, x.f.db.Table("api_keys").Select("id").Where("environment_id = ?", "example").Limit(1).Scan(&keyID).Error)
	endpoint := server.URL
	if mode == "edge" {
		x.tunnel = actualSettingsTunnelForEnvironment(t, x.agent, "example", "example", keyID)
		endpoint = "edge://example"
	}
	require.NoError(t, x.f.db.Model(&environment.Environment{}).Where("id = ?", "example").Updates(map[string]any{"api_url": endpoint, "is_edge": mode == "edge", "enabled": true, "access_token": "example", "api_key_id": keyID}).Error)
	x.hosts = environment.NewEnvironmentService(x.f.db, server.Client(), nil, nil, x.hubSettings, x.f.keys)
	deps := api.HandlerDeps{Auth: auth.New(x.f.svc, nil, nil), ApiKey: apikey.New(x.f.keys), Role: role.New(x.f.roles), Settings: settings.New(x.hubSettings, x.hosts.ProxyJSONRequest, x.f.cfg), Environment: environment.New(x.hosts, nil, x.f.keys, nil, x.f.cfg, nil), Modules: x.manager}
	hub := echo.New()
	x.hub = hub
	hubConfig := huma.DefaultConfig("module hub", "test")
	hubConfig.Security = handlerutil.DefaultOperationSecurity()
	hubConfig.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(t reflect.Type, hint string) string {
		n := huma.DefaultSchemaNamer(t, hint)
		if strings.Contains(t.PkgPath(), "/types/proxy") {
			return "Proxy" + n
		}
		return n
	})
	hubAPI := humaecho.NewWithGroup(hub, hub.Group("/api"), hubConfig)
	x.hubAPI = hubAPI
	hubAPI.UseMiddleware(auth.NewHumaMiddleware(hubAPI, x.f.svc, x.f.keys, x.f.roles, x.hosts, x.f.cfg))
	hubAPI.UseMiddleware(middleware.AccessMiddleware(hubAPI))
	hubAPI.UseMiddleware(activity.OperationCapabilityContext(activity.OperationCapabilityReaders{ModuleReason: func(ctx context.Context, env string, id features.ID) string {
		if !x.manager.Enabled(ctx, id) {
			return "feature-disabled"
		}
		return x.hosts.ModuleReason(ctx, env, id)
	}}))
	activity.RegisterCapabilities(hubAPI)
	modules.RegisterRoutes(hubAPI, x.manager, x.hosts, false)
	settings.RegisterSettings(hubAPI, x.hubSettings, x.hosts.ProxyJSONRequest, x.f.cfg)
	if probe {
		registerM27Probe(t, hubAPI, x.hubSettings, false, "/modules/proxy/probe", "m27-hub-probe", &x.hubCalls)
		registerM27Probe(t, hubAPI, x.hubSettings, false, "/environments/{id}/modules/proxy/data", "m27-host-probe", &x.hubCalls)
	}
	matcher := authz.NewPermissionMatcher()
	matcher.CollectFromHumaAPI(hubAPI)
	hub.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(ctx context.Context, id string) (string, *string, bool, error) {
		r, err := x.hosts.GetEnvironmentByID(ctx, id)
		if err != nil {
			return "", nil, false, err
		}
		return r.ApiUrl, r.AccessToken, r.Enabled, nil
	}, createAuthValidatorInternal(deps), matcher, edge.GetRegistry(), nil, middleware.EnvironmentProxySecurity{ModuleEnabled: x.hubSettings.IsFeatureEnabled, AgentAccess: x.hosts.AgentAccess}))

	hubGroup := hub.Group("/api")
	hubGroup.RouteNotFound("", middleware.APINotFound)
	hubGroup.RouteNotFound("/*", middleware.APINotFound)
	return x
}
func registerM27Probe(t *testing.T, api huma.API, s *settings.SettingsService, agent bool, path, id string, calls *atomic.Int32, failures ...func() error) {
	t.Helper()
	old, had := access.Rules[id]
	access.Rules[id] = access.Rule{Kind: access.Admin}
	t.Cleanup(func() {
		if had {
			access.Rules[id] = old
		} else {
			delete(access.Rules, id)
		}
	})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: id, Method: "DELETE", Path: path, Middlewares: huma.Middlewares{middleware.ModuleGate(api, s, agent, features.Proxy)}}, authz.PermSettingsWrite, func(context.Context, *struct {
		ID string `path:"id" required:"false"`
	}) (*struct{ Body string }, error) {
		calls.Add(1)
		if len(failures) > 0 {
			if err := failures[0](); err != nil {
				return nil, err
			}
		}
		return &struct{ Body string }{Body: "admitted"}, nil
	})
}
func (x *m27ModuleFixture) request(method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: x.f.tokens.BrowserToken})
	w := httptest.NewRecorder()
	x.hub.ServeHTTP(w, r)
	return w
}

// Compare the same URL on a control router where it is genuinely unknown. The
// production envelope contains the requested path; no body normalisation is used.
func m27UnknownResponse(method, path string) *httptest.ResponseRecorder {
	router := echo.New()
	group := router.Group("/api")
	group.RouteNotFound("", middleware.APINotFound)
	group.RouteNotFound("/*", middleware.APINotFound)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(method, path, nil))
	return response
}
