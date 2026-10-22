// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original authenticated Direct/Edge environment-routing session regressions for issue #133.

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/api/ws"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

// Real browser upgrades use the production early authentication and environment
// middleware. Only the agent's application stream is synthetic.
func TestSessionProductionDirectEdgeRoutingRevokesOnlyMatchingViewers(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newSignInRoutingFixture(t)
			admin, err := f.users.GetUserByID(t.Context(), f.adminID)
			require.NoError(t, err)
			otherTokens, err := f.svc.CompleteLogin(t.Context(), admin, authtypes.SessionMeta{}, session.UserSessionSourceLocal)
			require.NoError(t, err)
			_, sessionID, err := f.svc.VerifyToken(t.Context(), f.tokens.AccessToken)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			registry := edge.NewTunnelRegistry()
			agentToken := "example"
			agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Empty(t, r.Header.Get("Cookie"), "browser cookies stay at the hub")
				require.Empty(t, r.Header.Get("Authorization"))
				require.Equal(t, "/api/environments/0/ws/system/stats", r.URL.Path)
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				if conn.Write(ctx, websocket.MessageText, []byte("ready")) != nil {
					return
				}
				for {
					kind, data, err := conn.Read(ctx)
					if err != nil {
						return
					}
					if conn.Write(ctx, kind, data) != nil {
						return
					}
				}
			}))
			defer agent.Close()
			var tunnel *edge.AgentTunnel
			if mode == "edge" {
				tunnel = sessionRoutingTunnel(t, ctx, registry)
			}
			resolver := func(context.Context, string) (string, *string, bool, error) {
				if mode == "edge" {
					return "edge://example", &agentToken, true, nil
				}
				return agent.URL, &agentToken, true, nil
			}
			authMiddleware := auth.NewAuthMiddleware(f.svc, f.cfg).WithApiKeyValidator(f.keys).WithPermissionResolver(f.roles)
			deps := api.HandlerDeps{Auth: auth.New(f.svc, nil, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles)}
			matcher := authz.NewPermissionMatcher()
			ws.AddProxiedPermissions(matcher)
			router := echo.New()
			router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver, createAuthValidatorInternal(deps), matcher, registry,
				middleware.WebSocketOriginValidator(f.cfg, authMiddleware.ValidateNonCookieCredential)))
			router.GET("/api/environments/:id/ws/system/stats", func(c *echo.Context) error { return c.NoContent(500) })
			hub := httptest.NewServer(router)
			defer hub.Close()
			connect := func(token string) *websocket.Conn {
				req := &http.Request{Header: http.Header{"Origin": {f.cfg.AppUrl}}}
				req.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: token})
				conn, _, err := websocket.Dial(ctx, edge.HTTPToWebSocketURL(hub.URL)+"/api/environments/example/ws/system/stats", &websocket.DialOptions{HTTPHeader: req.Header})
				require.NoError(t, err)
				t.Cleanup(func() { _ = conn.CloseNow() })
				_, data, err := conn.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "ready", string(data))
				return conn
			}
			first, second, other := connect(f.tokens.BrowserToken), connect(f.tokens.BrowserToken), connect(otherTokens.BrowserToken)
			require.NoError(t, f.svc.RevokeSession(ctx, sessionID))
			for _, conn := range []*websocket.Conn{first, second} {
				// An incorrectly unbound viewer echoes this payload, giving a direct
				// assertion failure rather than a deadline-only red proof.
				_ = conn.Write(ctx, websocket.MessageText, []byte("after revoke"))
				readCtx, stop := context.WithTimeout(ctx, 2*time.Second)
				_, _, err := conn.Read(readCtx)
				require.Error(t, err)
				require.NoError(t, readCtx.Err(), "revoked viewer must close before the deadline")
				stop()
			}
			require.NoError(t, other.Write(ctx, websocket.MessageText, []byte("other session still live")))
			_, data, err := other.Read(ctx)
			require.NoError(t, err)
			require.Equal(t, "other session still live", string(data))
			if tunnel != nil {
				require.False(t, tunnel.Conn.IsClosed(), "browser revocation must preserve the shared Edge tunnel")
				require.Eventually(t, func() bool {
					count := 0
					tunnel.Pending.Range(func(_, _ any) bool { count++; return true })
					return count == 1
				}, time.Second, time.Millisecond)
			}
			_ = other.CloseNow()
		})
	}
}

// A real WebSocket tunnel carries stream-open/data/close frames; the synthetic
// agent echoes application data without injecting a browser ViewerContext.
func sessionRoutingTunnel(t *testing.T, ctx context.Context, registry *edge.TunnelRegistry) *edge.AgentTunnel {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		conn := edge.NewTunnelConn(socket)
		defer conn.Close()
		for {
			message, err := conn.Receive()
			if err != nil {
				return
			}
			switch message.Type {
			case edge.MessageTypeStreamOpen:
				if message.Path != "/api/environments/0/ws/system/stats" {
					t.Error("wrong forwarded route")
					return
				}
				for name := range message.Headers {
					if strings.EqualFold(name, "Cookie") || strings.EqualFold(name, "Authorization") {
						t.Error("browser credentials forwarded")
						return
					}
				}
				message.Type, message.Body, message.WSMessageType = edge.MessageTypeStreamData, []byte("ready"), int(websocket.MessageText)
				if conn.Send(message) != nil {
					return
				}
			case edge.MessageTypeStreamData:
				if conn.Send(message) != nil {
					return
				}
			}
		}
	}))
	socket, _, err := websocket.Dial(ctx, edge.HTTPToWebSocketURL(server.URL), nil)
	require.NoError(t, err)
	tunnel := edge.NewAgentTunnelWithConn("example", edge.NewTunnelConn(socket))
	registry.Register("example", tunnel)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		for {
			message, err := tunnel.Conn.Receive()
			if err != nil {
				return
			}
			if pending, ok := tunnel.Pending.Load(message.ID); ok {
				select {
				case pending.(*edge.PendingRequest).ResponseCh <- message:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	t.Cleanup(func() {
		_ = tunnel.CloseWithReason("")
		server.Close()
		select {
		case <-joined:
		case <-time.After(5 * time.Second):
			t.Error("Edge receiver did not join")
		}
	})
	return tunnel
}
