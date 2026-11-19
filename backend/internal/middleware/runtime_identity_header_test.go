// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original Direct/Edge test that forwarding strips the identity header of the current runtime sign-in policy.

package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
)

func TestForwardingStripsRuntimeIdentityHeader(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			mw := newTestEnvironmentMiddleware()
			mw.proxyIdentityHeader = "Remote-User"
			current := "X-Example-User"
			mw.proxyIdentityHeaderFunc = func() string { return current }
			token := "example"
			mw.resolver = func(context.Context, string) (string, *string, bool, error) {
				if mode == "edge" {
					return "edge://example", &token, true, nil
				}
				return "http://agent.example.com", &token, true, nil
			}
			var forwarded http.Header
			mw.httpClient = &http.Client{Transport: boundaryRoundTrip(func(r *http.Request) (*http.Response, error) {
				forwarded = r.Header.Clone()
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}")), Request: r}, nil
			})}
			conn := &boundaryTunnelConn{}
			tunnel := edge.NewAgentTunnelWithConn("example", conn)
			conn.send = func(m *edge.TunnelMessage) {
				if m.Type != edge.MessageTypeCommandRequest {
					return
				}
				forwarded = http.Header{}
				for name, value := range m.Headers {
					forwarded.Set(name, value)
				}
				pending, ok := tunnel.Pending.Load(m.ID)
				require.True(t, ok)
				pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeResponse, Status: 200, Body: []byte("{}")}
			}
			if mode == "edge" {
				mw.registry.Register("example", tunnel)
				defer mw.registry.Unregister("example")
			}
			router := echo.New()
			api := attachMiddleware(router, mw)
			api.GET("/environments/:id/containers", func(c *echo.Context) error { return c.NoContent(500) })
			send := func() {
				req := httptest.NewRequest(http.MethodGet, "/api/environments/example/containers", http.NoBody).WithContext(ctx)
				req.Header.Set("X-Example-User", "example-operator")
				req.Header.Set("X-Other-User", "example-operator")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.Equal(t, 200, w.Code, w.Body.String())
			}
			send()
			require.Empty(t, forwarded.Get("X-Example-User"), "the active header never reaches the agent")
			current = "X-Other-User"
			send()
			require.Empty(t, forwarded.Get("X-Other-User"), "a runtime header change applies to the next request")
		})
	}
}
