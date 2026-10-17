// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type boundaryRoundTrip func(*http.Request) (*http.Response, error)

func (f boundaryRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type boundaryTunnelConn struct {
	send   func(*edge.TunnelMessage)
	closed bool
}

func (c *boundaryTunnelConn) Send(m *edge.TunnelMessage) error      { c.send(m); return nil }
func (c *boundaryTunnelConn) Receive() (*edge.TunnelMessage, error) { return nil, io.EOF }
func (c *boundaryTunnelConn) IsExpectedReceiveError(error) bool     { return true }
func (c *boundaryTunnelConn) Close() error                          { c.closed = true; return nil }
func (c *boundaryTunnelConn) IsClosed() bool                        { return c.closed }
func (c *boundaryTunnelConn) Transport() string                     { return edge.EdgeTransportWebSocket }

func TestAgentResponseBoundaryBothTransports(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, contentType := range []string{"text/html", "image/svg+xml", "application/json"} {
			t.Run(mode+"/"+contentType, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				mw := newTestEnvironmentMiddleware()
				mw.proxyIdentityHeader = "X-Identity"
				token := "example"
				mw.resolver = func(context.Context, string) (string, *string, bool, error) {
					if mode == "edge" {
						return "edge://example", &token, true, nil
					}
					return "http://agent.example.com", &token, true, nil
				}
				body := "<!doctype html><script>example()</script>"
				if contentType == "image/svg+xml" {
					body = `<svg xmlns="http://www.w3.org/2000/svg"><script>example()</script></svg>`
				}
				agentHeaders := http.Header{}
				for _, name := range []string{"Content-Type", "Content-Length", "Content-Encoding", "Content-Range", "Accept-Ranges", "Cache-Control", "ETag", "Last-Modified"} {
					agentHeaders.Set(name, "example")
				}
				agentHeaders.Set("Content-Type", contentType)
				agentHeaders.Set("Content-Length", strconv.Itoa(len(body)))
				if contentType == "application/json" {
					agentHeaders.Set("Content-Disposition", "attachment; filename=example.json")
				}
				agentHeaders.Set("Set-Cookie", "session=example")
				agentHeaders.Set("Content-Security-Policy", "default-src * 'unsafe-inline'")
				agentHeaders.Set("X-Agent-Controlled", "example")
				checkRequest := func(h http.Header) {
					require.Empty(t, h.Get("Authorization"))
					require.Empty(t, h.Get("Cookie"))
					require.Empty(t, h.Get("X-Identity"))
					require.Empty(t, h.Get("Remote-User"))
					require.Equal(t, token, h.Get("X-Api-Key"))
					require.Equal(t, token, h.Get("X-Arcane-Agent-Token"))
				}
				mw.httpClient = &http.Client{Transport: boundaryRoundTrip(func(r *http.Request) (*http.Response, error) {
					checkRequest(r.Header)
					return &http.Response{StatusCode: 200, Header: agentHeaders, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				})}
				conn := &boundaryTunnelConn{}
				tunnel := edge.NewAgentTunnelWithConn("example", conn)
				conn.send = func(m *edge.TunnelMessage) {
					if m.Type != edge.MessageTypeCommandRequest {
						return
					}
					h := http.Header{}
					for name, value := range m.Headers {
						h.Set(name, value)
					}
					checkRequest(h)
					pending, ok := tunnel.Pending.Load(m.ID)
					require.True(t, ok)
					headers := map[string]string{}
					for name, values := range agentHeaders {
						headers[name] = values[0]
					}
					pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeResponse, Status: 200, Headers: headers, Body: []byte(body)}
				}
				if mode == "edge" {
					mw.registry.Register("example", tunnel)
					defer mw.registry.Unregister("example")
				}
				router := echo.New()
				api := attachMiddleware(router, mw)
				api.GET("/environments/:id/containers", func(c *echo.Context) error { return c.NoContent(500) })
				req := httptest.NewRequest(http.MethodGet, "/api/environments/example/containers", http.NoBody).WithContext(ctx)
				for _, name := range []string{"Authorization", "Cookie", "X-Api-Key", "X-Identity", "Remote-User"} {
					req.Header.Set(name, "example-hub-session")
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.Equal(t, 200, w.Code)
				require.Equal(t, body, w.Body.String())
				for name, values := range agentHeaders {
					if name == "Set-Cookie" || name == "Content-Security-Policy" || name == "X-Agent-Controlled" {
						continue
					}
					require.Equal(t, values, w.Header().Values(name), name)
				}
				require.Empty(t, w.Header().Get("Set-Cookie"))
				require.Empty(t, w.Header().Get("X-Agent-Controlled"))
				if contentType != "application/json" {
					require.Empty(t, w.Header().Get("Content-Disposition"))
				}
				require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
				require.Equal(t, remenv.AgentContentSecurityPolicy, w.Header().Get("Content-Security-Policy"))
			})
		}
	}
}

func TestAgentDownloadHeaderAllowlist(t *testing.T) {
	from := http.Header{"Content-Disposition": {"attachment; filename=example.txt"}, "Set-Cookie": {"example=bad"}}
	to := http.Header{}
	edge.CopyResponseHeaders(from, to, edge.BuildHopByHopHeaders(from))
	require.Equal(t, from.Get("Content-Disposition"), to.Get("Content-Disposition"))
	require.Empty(t, to.Get("Set-Cookie"))
	require.Equal(t, "nosniff", to.Get("X-Content-Type-Options"))
	require.Equal(t, remenv.AgentContentSecurityPolicy, to.Get("Content-Security-Policy"))
}

func TestAgentWebSocketCredentialHeaders(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			mw := newTestEnvironmentMiddleware()
			mw.proxyIdentityHeader = "X-Identity"
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			for _, name := range []string{"Authorization", "Cookie", "X-Api-Key", "X-Identity", "Remote-User"} {
				req.Header.Set(name, "example-hub-session")
			}
			c := echo.New().NewContext(req, httptest.NewRecorder())
			token := "example"
			mw.setProxyContextHeadersInternal(c, &token)
			headers := edge.AgentRequestHeaders(req.Header)
			if mode == "direct" {
				headers = edge.BuildWebSocketHeaders(c, &token)
			}
			for _, name := range []string{"Authorization", "Cookie", "X-Identity", "Remote-User"} {
				require.Empty(t, headers.Get(name), name)
			}
			require.Equal(t, token, headers.Get("X-Api-Key"))
			require.Equal(t, token, headers.Get("X-Arcane-Agent-Token"))
		})
	}
}
