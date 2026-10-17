// Source: docklet-next/issues/68
// Revision: f46e412dc7509b97aadc9e5edc5551d5c25a14cf
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original hub-to-agent cookie boundary integration test.

package bootstrap

import (
	"context"
	"encoding/json/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/api/ws"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestSignInDirectAgentCookieBoundary(t *testing.T) {
	for _, mode := range []string{"none", "local", "proxy"} {
		t.Run(mode, func(t *testing.T) {
			f := newSignInRoutingFixture(t)
			f.cfg.AuthMode = mode
			agentToken := "example"
			agentCfg := &config.Config{AgentMode: true, AgentToken: agentToken, AppUrl: f.cfg.AppUrl, Environment: "production"}
			agentAuth := auth.NewAuthMiddleware(nil, agentCfg)
			type agentRequest struct {
				path   string
				header http.Header
				status int
			}
			websocketRequests := make(chan agentRequest, 1)
			httpRequests := make(chan agentRequest, 1)
			agentRouter := echo.New()
			agentRouter.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c *echo.Context) error {
					header := c.Request().Header.Clone()
					requests := httpRequests
					if header.Get("Upgrade") == "websocket" {
						requests = websocketRequests
					}
					response := echo.NewResponse(c.Response(), c.Logger())
					c.SetResponse(response)
					err := next(c)
					requests <- agentRequest{c.Request().URL.Path, header, response.Status}
					return err
				}
			})
			ws.NewWebSocketHandler(agentRouter.Group("/api"), nil, nil, nil, nil, agentAuth, agentCfg)
			agentRouter.GET("/api/environments/:id/containers", func(c *echo.Context) error {
				return c.String(http.StatusOK, "example-agent-response")
			}, agentAuth.Add(), middleware.RequireEchoPermission(authz.PermContainersList))
			agent := httptest.NewServer(agentRouter)
			defer agent.Close()

			authMiddleware := auth.NewAuthMiddleware(f.svc, f.cfg).WithApiKeyValidator(f.keys).WithPermissionResolver(f.roles)
			deps := api.HandlerDeps{Auth: auth.New(f.svc, nil, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles)}
			matcher := authz.NewPermissionMatcher()
			ws.AddProxiedPermissions(matcher)
			matcher.Add(http.MethodGet, "/containers", authz.PermContainersList)
			resolver := func(context.Context, string) (string, *string, bool, error) {
				return agent.URL, &agentToken, true, nil
			}
			router := echo.New()
			router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver,
				createAuthValidatorInternal(deps), matcher, nil,
				middleware.WebSocketOriginValidator(f.cfg, authMiddleware.ValidateNonCookieCredential)))
			ws.NewWebSocketHandler(router.Group("/api"), nil, nil, nil, nil, authMiddleware, f.cfg)
			router.GET("/api/environments/:id/containers", func(c *echo.Context) error {
				return c.NoContent(http.StatusInternalServerError)
			})
			hub := httptest.NewUnstartedServer(router)
			peer, _, err := net.SplitHostPort(hub.Listener.Addr().String())
			require.NoError(t, err)
			f.cfg.AuthTrustedProxies = peer
			hub.Start()
			defer hub.Close()

			headers := http.Header{"Origin": {f.cfg.AppUrl}, "Remote-User": {"operator"}}
			browserRequest := &http.Request{Header: headers}
			browserRequest.AddCookie(&http.Cookie{Name: "docklet_csrf", Value: "example-csrf"})
			browserRequest.AddCookie(&http.Cookie{Name: "proxy_session", Value: "example-proxy-session"})
			if mode == "local" {
				browserRequest.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: f.tokens.BrowserToken})
			}
			checkAgentRequest := func(t *testing.T, ctx context.Context, requests <-chan agentRequest, path string, status int) {
				t.Helper()
				select {
				case got := <-requests:
					require.Equal(t, path, got.path)
					require.Equal(t, status, got.status)
					require.NotContains(t, got.header, "Cookie")
					require.NotContains(t, got.header, "Origin")
					require.Equal(t, agentToken, got.header.Get(middleware.HeaderAgentToken))
					require.Empty(t, got.header.Get("Authorization"))
					require.Empty(t, got.header.Get("Remote-User"))
				case <-ctx.Done():
					t.Fatal("agent request did not complete before the deadline")
				}
			}
			t.Run("websocket", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				conn, response, err := websocket.Dial(ctx, edge.HTTPToWebSocketURL(hub.URL)+"/api/environments/example/ws/system/stats",
					&websocket.DialOptions{HTTPHeader: headers.Clone()})
				require.NoError(t, err)
				defer conn.CloseNow()
				require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
				// A hub-side 101 precedes the agent dial; a real stats frame proves both upgrades succeeded.
				kind, frame, err := conn.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, websocket.MessageText, kind)
				var snapshot map[string]any
				require.NoError(t, json.Unmarshal(frame, &snapshot))
				require.Contains(t, snapshot, "cpuCount")
				require.NoError(t, conn.Close(websocket.StatusNormalClosure, ""))
				checkAgentRequest(t, ctx, websocketRequests, "/api/environments/0/ws/system/stats", http.StatusSwitchingProtocols)
			})
			t.Run("http", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, hub.URL+"/api/environments/example/containers", http.NoBody)
				require.NoError(t, err)
				req.Header = headers.Clone()
				response, err := hub.Client().Do(req)
				require.NoError(t, err)
				defer response.Body.Close()
				require.Equal(t, http.StatusOK, response.StatusCode)
				checkAgentRequest(t, ctx, httpRequests, "/api/environments/0/containers", http.StatusOK)
			})
		})
	}
}
