// Source: docklet-next/issues/309
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original real producer and Direct/Edge refusal boundary tests.

package environment_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/autoheal"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/contract"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestHostRefusalsRealProducersDirectAndEdge(t *testing.T) {
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "refusal.db")), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	settingService, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	defer settingService.Stop(context.WithoutCancel(t.Context()))
	cfg := &config.Config{AgentMode: true, AgentToken: "example", AuthMode: "local", UIConfigurationDisabled: true}
	require.NoError(t, db.Exec("INSERT INTO api_keys(id,name,key_hash,key_prefix,kind) VALUES(?,?,?,?,?)", "example", "Example key", "example", "example", "environment").Error)
	users := user.NewUserService(db, role.NewRoleService(db), session.RevokeAllUserSessionsExceptInDB)
	authService := auth.NewAuthService(users, settingService, nil, session.NewSessionService(db), cfg)
	agentRouter := echo.New()
	apiConfig := huma.DefaultConfig("Refusals", "0.0.0")
	apiConfig.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(typ reflect.Type, hint string) string {
		return typ.PkgPath() + "." + huma.DefaultSchemaNamer(typ, hint)
	})
	api := humaecho.NewWithGroup(agentRouter, agentRouter.Group("/api"), apiConfig)
	api.UseMiddleware(auth.NewHumaMiddleware(api, authService, nil, nil, nil, cfg))
	autoheal.New(settingService, &environment.EnvironmentService{}, cfg).RegisterRoutes(api)
	settings.RegisterSettings(api, settingService, nil, nil, cfg)
	huma.Get(api, "/app-version", func(context.Context, *struct{}) (*struct{ Body version.Info }, error) {
		return &struct{ Body version.Info }{Body: version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}}, nil
	})
	huma.Head(api, "/environments/{id}/system/health", func(context.Context, *struct {
		ID string `path:"id"`
	}) (*struct{}, error) {
		return nil, nil
	}, func(op *huma.Operation) { op.Security = handlerutil.DefaultOperationSecurity() })
	var refusalMode atomic.Value
	refusalMode.Store("")
	agent := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch refusalMode.Load().(string) {
		case "unknown":
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"code":"arbitrary"}`))
			return
		case "malformed":
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`not JSON`))
			return
		}
		agentRouter.ServeHTTP(w, r)
	})
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(agent)
			defer server.Close()
			registry := edge.NewTunnelRegistry()
			endpoint := server.URL
			token := "example"
			if mode == "edge" {
				connected := make(chan struct{})
				var once sync.Once
				tunnelServer := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return "example", nil }, func(_ context.Context, _ string, online bool) {
					if online {
						once.Do(func() { close(connected) })
					}
				})
				tunnelServer.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return "example", nil })
				router := echo.New()
				router.GET("/api/tunnel/connect", tunnelServer.HandleConnect)
				manager := httptest.NewServer(router)
				defer manager.Close()
				life, cancel := context.WithCancel(t.Context())
				done := make(chan struct{})
				client := edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: "example", EdgeReconnectInterval: 1}, agent)
				go func() { defer close(done); client.StartWithErrorChan(life, nil) }()
				defer func() {
					cancel()
					registry.Unregister("example")
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("Edge worker did not join")
					}
				}()
				select {
				case <-connected:
				case <-time.After(5 * time.Second):
					t.Fatal("Edge registration missing")
				}
				endpoint = "edge://example"
			}
			previousRegistry := edge.GetRegistry()
			edge.SetDefaultRegistry(registry)
			defer edge.SetDefaultRegistry(previousRegistry)
			require.NoError(t, db.Create(&environment.Environment{ID: "example", Name: "Example host", ApiUrl: endpoint, IsEdge: mode == "edge", Enabled: true, AccessToken: new("example"), ApiKeyID: new("example")}).Error)
			defer db.Delete(&environment.Environment{}, "id=?", "example")
			envService := environment.NewEnvironmentService(db, http.DefaultClient, nil, nil, nil, nil)
			hub := echo.New()
			actor := &userDTO.Actor{ID: "operator", Username: "operator"}
			hub.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) { return endpoint, &token, true, nil }, func(_ context.Context, c *echo.Context) (*authz.PermissionSet, *userDTO.Actor, bool) {
				require.Equal(t, "docklet_session=example", c.Request().Header.Get("Cookie"))
				return authz.SudoPermissionSet(), actor, true
			}, nil, registry, nil, middleware.EnvironmentProxySecurity{AgentAccess: func(ctx context.Context, _ string, _ *string, _, _ string) (context.Context, context.CancelFunc, error) {
				return edge.WithSessionCredential(ctx, "example", "example", nil), func() {}, nil
			}}))
			hubConfig := huma.DefaultConfig("Hub refusals", "0.0.0")
			hubConfig.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(typ reflect.Type, hint string) string {
				return typ.PkgPath() + "." + huma.DefaultSchemaNamer(typ, hint)
			})
			hubAPI := humaecho.NewWithGroup(hub, hub.Group("/api"), hubConfig)
			hubAPI.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				require.Equal(t, "docklet_session=example", ctx.Header("Cookie"))
				next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
			})
			autoheal.New(settingService, envService, &config.Config{}).RegisterRoutes(hubAPI)
			settings.RegisterSettings(hubAPI, settingService, nil, envService.ProxyJSONRequest, &config.Config{})
			hub.Any("/api/environments/:id/*", func(c *echo.Context) error { return c.NoContent(418) })
			for _, tc := range []struct{ suffix, body string }{{"auto-heal", `{"maxRestarts":3,"windowMinutes":30}`}, {"settings", `{}`}} {
				refusalMode.Store("")
				cfg.AgentToken = "example"
				producer := httptest.NewRecorder()
				req := httptest.NewRequest("PUT", "/api/environments/0/"+tc.suffix, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Arcane-Agent-Token", "example")
				agent.ServeHTTP(producer, req)
				require.Equal(t, 403, producer.Code, producer.Body.String())
				require.Contains(t, producer.Body.String(), "settings.ui-configuration-disabled")
				verdict := handlerutil.AgentVerdict(&remenv.StatusError{StatusCode: producer.Code, Body: producer.Body.Bytes()})
				require.NotNil(t, verdict)
				require.Equal(t, "agent.ui-configuration-disabled", verdict.Code)
				require.Equal(t, 502, verdict.GetStatus())
				for _, refusal := range []string{"", "unknown", "malformed", "bad-token"} {
					refusalMode.Store(refusal)
					if refusal == "bad-token" {
						cfg.AgentToken = "test-only"
					} else {
						cfg.AgentToken = "example"
					}
					path := "/api/environments/example/" + tc.suffix
					if refusal == "unknown" || refusal == "malformed" {
						path += "?refusal=" + refusal
					}
					response := httptest.NewRecorder()
					request := httptest.NewRequest("PUT", path, strings.NewReader(tc.body))
					request.Header.Set("Content-Type", "application/json")
					request.Header.Set("Cookie", "docklet_session=example")
					hub.ServeHTTP(response, request)
					require.Equal(t, 502, response.Code, "%s/%s: %s", tc.suffix, refusal, response.Body.String())
					code := "agent.invalid-token"
					if refusal == "" {
						code = "agent.ui-configuration-disabled"
					}
					require.Contains(t, response.Body.String(), code)
					require.Empty(t, response.Header().Values("Set-Cookie"), "host refusal must not mutate browser identity")
				}
			}
		})
	}
}
