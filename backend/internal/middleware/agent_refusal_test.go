// Source: docklet-next/issues/309
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original proxy refusal envelope checks supplementing real producer domain-route tests.

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestAgentRefusalProxyClassifiers(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			mw := newTestEnvironmentMiddleware()
			token := "example"
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Set-Cookie", "session=example")
				w.WriteHeader(403)
				_, _ = w.Write([]byte(`{"code":"settings.ui-configuration-disabled","status":403}`))
			}))
			defer upstream.Close()
			endpoint := upstream.URL
			if mode == "edge" {
				endpoint = "edge://example"
				conn := &boundaryTunnelConn{}
				tunnel := edge.NewAgentTunnelWithConn("example", conn)
				conn.send = func(msg *edge.TunnelMessage) {
					if msg.Type != edge.MessageTypeCommandRequest {
						return
					}
					pending, ok := tunnel.Pending.Load(msg.ID)
					require.True(t, ok)
					pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: msg.ID, Type: edge.MessageTypeResponse, Status: 403, Body: []byte(`{"code":"settings.ui-configuration-disabled","status":403}`)}
				}
				mw.registry.Register("example", tunnel)
				defer mw.registry.Unregister("example")
			}
			mw.resolver = func(context.Context, string) (string, *string, bool, error) { return endpoint, &token, true, nil }
			router := echo.New()
			group := attachMiddleware(router, mw)
			group.GET("/environments/:id/containers", func(c *echo.Context) error { return c.NoContent(418) })
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/environments/example/containers", http.NoBody))
			require.Equal(t, 502, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "agent.ui-configuration-disabled")
			require.Empty(t, rec.Header().Get("Set-Cookie"))
		})
	}
}
