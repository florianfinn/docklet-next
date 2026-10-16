// Source: docklet-next/issues/68
// Revision: af5fa9df01e730f7691d650985abd33d74f51ae5
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original exact-Origin matrix across local terminal, direct and Edge upgrades.

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/api/ws"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestSignInWebSocketOriginMatrix(t *testing.T) {
	f := newSignInRoutingFixture(t)
	for _, mode := range []string{"none", "local", "proxy"} {
		f.cfg.AuthMode = mode
		authMiddleware := auth.NewAuthMiddleware(f.svc, f.cfg).WithApiKeyValidator(f.keys).WithPermissionResolver(f.roles)
		deps := api.HandlerDeps{Auth: auth.New(f.svc, nil, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles)}
		for _, transport := range []string{"local", "direct", "edge"} {
			router := echo.New()
			if transport != "local" {
				registry := edge.NewTunnelRegistry()
				registry.Register("example", edge.NewAgentTunnelWithConn("example", &originTestTunnelConnection{}))
				resolver := func(context.Context, string) (string, *string, bool, error) {
					if transport == "edge" {
						return "edge://example", new("example-agent-credential"), true, nil
					}
					return "https://agent.example.com", new("example-agent-credential"), true, nil
				}
				if transport == "direct" {
					registry = edge.NewTunnelRegistry()
				}
				validate := func(req *http.Request) bool {
					// Recognize the server-injected token too: the early gate must reject before injection.
					return authMiddleware.ValidateNonCookieCredential(req) || req.Header.Get(middleware.HeaderAgentToken) == "example-agent-credential" || csrfTransportValidatorInternal(deps)(req)
				}
				router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver,
					createAuthValidatorInternal(deps), nil, registry, middleware.WebSocketOriginValidator(f.cfg, validate)))
			}
			ws.NewWebSocketHandler(router.Group("/api"), nil, nil, nil, nil, authMiddleware, f.cfg)
			for _, route := range []string{"system/stats", "containers/example/terminal"} {
				for _, tc := range []struct {
					name, host, origin string
					status             int
				}{
					{"hub", "hub.example.com", "https://hub.example.com", 101},
					{"foreign", "hub.example.com", "https://other.example.com", 403},
					{"rebind", "rebind.example.com", "http://rebind.example.com", 403},
					{"localhost port", "localhost:3552", "http://localhost:8080", 403},
					{"wrong scheme", "hub.example.com", "http://hub.example.com", 403},
					{"null", "hub.example.com", "null", 403},
					{"empty cookie", "hub.example.com", "", 403},
				} {
					t.Run(mode+"/"+transport+"/"+route+"/"+tc.name, func(t *testing.T) {
						envID := "0"
						if transport != "local" {
							envID = "example"
						}
						req := httptest.NewRequest(http.MethodGet, "/api/environments/"+envID+"/ws/"+route, http.NoBody)
						req.Host = tc.host
						req.RemoteAddr = "192.0.2.10:5000"
						req.Header.Set("Origin", tc.origin)
						req.Header.Set("Remote-User", "operator")
						if mode == "local" {
							req.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: f.tokens.BrowserToken})
						}
						webSocketHandshake(req)
						rec := httptest.NewRecorder()
						router.ServeHTTP(rec, req)
						require.Equal(t, tc.status, rec.Code, rec.Body.String())
						if tc.status == 403 && transport != "local" {
							require.Empty(t, req.Header.Get(middleware.HeaderAgentToken))
						}
					})
				}
			}
		}
	}
}

func TestSignInWebSocketEmptyOriginCredentials(t *testing.T) {
	f := newSignInRoutingFixture(t)
	for _, mode := range []string{"none", "local", "proxy"} {
		f.cfg.AuthMode = mode
		authMiddleware := auth.NewAuthMiddleware(f.svc, f.cfg).WithApiKeyValidator(f.keys).WithPermissionResolver(f.roles)
		router := echo.New()
		ws.NewWebSocketHandler(router.Group("/api"), nil, nil, nil, nil, authMiddleware, f.cfg)
		for _, credential := range []string{"bearer", "key", "invalid bearer", "invalid key", "cookie", "proxy"} {
			t.Run(mode+"/"+credential, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "/api/environments/0/ws/containers/example/terminal", http.NoBody)
				req.RemoteAddr = "192.0.2.10:5000"
				req.Header.Set("Remote-User", "operator")
				switch credential {
				case "bearer":
					req.Header.Set("Authorization", "Bearer "+f.tokens.AccessToken)
				case "key":
					req.Header.Set(middleware.HeaderApiKey, f.personalKey)
				case "invalid bearer":
					req.Header.Set("Authorization", "Bearer example")
				case "invalid key":
					req.Header.Set(middleware.HeaderApiKey, "example")
				case "cookie":
					req.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: f.tokens.BrowserToken})
				}
				webSocketHandshake(req)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				want := 403
				if mode == "local" {
					want = 401
					if credential == "bearer" || credential == "key" {
						want = 101
					}
					if credential == "cookie" {
						want = 403
					}
				}
				require.Equal(t, want, rec.Code, rec.Body.String())
			})
		}
	}
}

func webSocketHandshake(req *http.Request) {
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "ZXhhbXBsZS1rZXktMTIzNA==")
}

func TestSignInAgentWebSocketOrigin(t *testing.T) {
	cfg := &config.Config{AgentMode: true, AgentToken: "example", AppUrl: "https://hub.example.com", Environment: "production"}
	router := echo.New()
	ws.NewWebSocketHandler(router.Group("/api"), nil, nil, nil, nil, auth.NewAuthMiddleware(nil, cfg), cfg)
	for _, tc := range []struct {
		name, token, origin string
		status              int
	}{
		{"valid transport", "example", "", 101},
		{"invalid transport", "invalid", "", 403},
		{"wrong origin", "example", "https://other.example.com", 403},
		{"allowed origin", "example", "https://hub.example.com", 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/environments/0/ws/containers/example/terminal", http.NoBody)
			req.Header.Set(middleware.HeaderAgentToken, tc.token)
			req.Header.Set("Origin", tc.origin)
			webSocketHandshake(req)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
		})
	}
}

type originTestTunnelConnection struct{ edge.TunnelConnection }

// The recorder cannot hijack, so upgrades stop before any tunnel I/O.
func (*originTestTunnelConnection) IsClosed() bool { return false }
