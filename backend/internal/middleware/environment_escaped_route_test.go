// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: c477a3d9aba28f8ae7ab6683b56e380a36fa1aa1c6d8e7de3bffeb4b61b7305f
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original escaped route identity regression tests for issue #260.

package middleware

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func escapedRouter() *echo.Echo {
	return echo.NewWithConfig(echo.Config{Router: echo.NewRouter(echo.RouterConfig{AllowOverwritingRoute: true, UnescapePathParamValues: true})})
}

type commandConn struct{ pointTestConn }

func (c *commandConn) Send(msg *edge.TunnelMessage) error {
	if msg.Type != edge.MessageTypeCommandRequest {
		return nil
	}
	if !edge.ValidateEdgeCommand(msg.Command, msg.Method, msg.Path, false) {
		return errors.New("agent rejects escaped command")
	}
	target := msg.Path
	if msg.Query != "" {
		target += "?" + msg.Query
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(msg.Method, target, bytes.NewReader(msg.Body))
	c.handler.ServeHTTP(rr, req)
	raw, _ := c.tunnel.Pending.Load(msg.ID)
	raw.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: msg.ID, Type: edge.MessageTypeCommandComplete, Status: rr.Code, Body: rr.Body.Bytes(), Headers: map[string]string{"Content-Type": "application/json"}}
	return nil
}
func escapedHub(t *testing.T, isEdge bool, m *EnvironmentMiddleware, agent http.Handler) *echo.Echo {
	t.Helper()
	m.resolver = func(context.Context, string) (string, *string, bool, error) {
		if isEdge {
			return "edge://example", nil, true, nil
		}
		return "https://agent.example.com", nil, true, nil
	}
	if isEdge {
		conn := &commandConn{pointTestConn: pointTestConn{handler: agent}}
		tunnel := edge.NewAgentTunnelWithConn("example", conn)
		conn.tunnel = tunnel
		m.registry.Register("example", tunnel)
	} else {
		m.httpClient = &http.Client{Transport: vulnerabilityTransport(func(r *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			agent.ServeHTTP(rr, r)
			return rr.Result(), nil
		})}
	}
	hub := escapedRouter()
	attachMiddleware(hub, m).Any("/environments/:id/*", func(c *echo.Context) error { return c.NoContent(418) })
	return hub
}
func TestEscapedForwardingKeepsResourceIdentity(t *testing.T) {
	cases := []struct{ suffix, route, param, want string }{
		{"/images/team%2Frepo%3Atag", "/images/:name", "name", "team/repo:tag"},
		{"/images/team%2fvulnerabilities", "/images/:name", "name", "team/vulnerabilities"},
		{"/images/team%252Frepo", "/images/:name", "name", "team/repo"},
		{"/images/a%3Fb%23c%25", "/images/:name", "name", "a?b#c%"},
		{"/containers/app%2Eblue", "/containers/:name", "name", "app.blue"},
		{"/volumes/app%2Ddata", "/volumes/:name", "name", "app-data"},
		{"/projects/stack%2Eblue/compose", "/projects/:name/compose", "name", "stack.blue"},
		{"/projects/stack%20blue/workspace/file", "/projects/:name/workspace/file", "name", "stack blue"},
		{"/volumes/app%2Ddata/workspace/file", "/volumes/:name/workspace/file", "name", "app-data"},
		{"/volumes/backups/run%2D1/files/browse", "/volumes/backups/:name/files/browse", "name", "run-1"},
	}
	for _, isEdge := range []bool{false, true} {
		for i, tc := range cases {
			t.Run(fmt.Sprintf("%t/%d", isEdge, i), func(t *testing.T) {
				m := newTestEnvironmentMiddleware()
				m.vulnerabilityEnabled = func(context.Context) bool { return false }
				query := "path=dir%2Ffile+name%25.txt&filter=a%3Fb%26c&bare"
				agent := escapedRouter()
				agent.GET("/api/environments/:id"+tc.route, func(c *echo.Context) error {
					require.Equal(t, tc.want, c.Param(tc.param))
					require.Equal(t, "dir/file name%.txt", c.QueryParam("path"))
					require.Equal(t, query, c.Request().URL.RawQuery)
					require.Equal(t, "/api/environments/0"+tc.suffix, c.Request().URL.EscapedPath())
					return c.String(200, "resource")
				})
				hub := escapedHub(t, isEdge, m, agent)
				rr := httptest.NewRecorder()
				hub.ServeHTTP(rr, httptest.NewRequest("GET", "/api/environments/example"+tc.suffix+"?"+query, nil))
				require.Equal(t, 200, rr.Code, rr.Body.String())
				require.Equal(t, "resource", rr.Body.String())
			})
		}
	}
}
func TestForwardedRouteUsesItsOwnPermission(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(fmt.Sprint(isEdge), func(t *testing.T) {
			m := newTestEnvironmentMiddleware()
			m.matcher = authz.NewPermissionMatcher()
			m.matcher.Add("GET", "/images/{imageId}", authz.PermImagesRead)
			m.matcher.Add("GET", "/images/{imageId}/vulnerabilities", authz.PermVulnsRead)
			ps := authz.NewPermissionSet()
			ps.AddEnv("example", authz.PermVulnsRead)
			require.False(t, ps.Allows(authz.PermImagesRead, "example"))
			m.authValidator = func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) { return ps, nil, true }
			m.vulnerabilityEnabled = func(context.Context) bool { return false }
			agent := escapedRouter()
			agent.GET("/api/environments/:id/images/:imageId", func(c *echo.Context) error { return c.String(200, "image-inspect:"+c.Param("imageId")) })
			agent.GET("/api/environments/:id/images/:imageId/vulnerabilities", func(c *echo.Context) error { return c.String(200, "vulnerability-data") })
			hub := escapedHub(t, isEdge, m, agent)
			for _, path := range []string{"/images/normal", "/images/team%2Fvulnerabilities"} {
				rr := httptest.NewRecorder()
				hub.ServeHTTP(rr, httptest.NewRequest("GET", "/api/environments/example"+path, nil))
				require.Equal(t, 403, rr.Code, "image inspection requires images:read")
			}
		})
	}
}
func TestCSRFRefusalShapesPerRoute(t *testing.T) {
	for _, vuln := range []bool{false, true} {
		for _, mode := range []string{"origin", "token", "credential", "valid", "transport"} {
			t.Run(fmt.Sprintf("%t/%s", vuln, mode), func(t *testing.T) {
				path := "/api/environments/example/containers"
				if vuln {
					path = "/api/environments/example/vulnerabilities/ignore/bulk"
				}
				csrf := NewCSRFMiddleware(&config.Config{AuthMode: "local", AppUrl: "https://hub.example.com"})
				if mode == "credential" {
					csrf.WithLocalCredentialValidator(func(*http.Request) (bool, error) { return false, errors.New("synthetic credential error") })
				}
				if mode == "transport" {
					csrf.WithTransportValidator(func(*http.Request) bool { return true })
				}
				e := escapedRouter()
				e.Use(csrf.Add())
				e.POST(path, func(c *echo.Context) error { return c.NoContent(202) })
				req := httptest.NewRequest("POST", path, nil)
				if mode == "token" || mode == "valid" {
					req.Header.Set("Origin", "https://hub.example.com")
				}
				if mode == "valid" {
					token := strings.Repeat("A", 43)
					req.AddCookie(&http.Cookie{Name: "docklet_csrf", Value: token})
					req.Header.Set(CSRFHeader, token)
				}
				rr := httptest.NewRecorder()
				e.ServeHTTP(rr, req)
				want := 403
				if mode == "credential" {
					want = 401
				}
				if mode == "valid" || mode == "transport" {
					want = 202
				}
				require.Equal(t, want, rr.Code)
				if want == 202 {
					return
				}
				if vuln {
					require.Contains(t, rr.Header().Get("Content-Type"), "application/problem+json")
					var problem huma.ErrorModel
					require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &problem, json.RejectUnknownMembers(true)))
					require.Equal(t, want, problem.Status)
				} else {
					var raw map[string]any
					require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &raw))
					require.Contains(t, raw, "message")
					require.Contains(t, raw, "code")
					require.Contains(t, raw, "statusCode")
					require.NotContains(t, raw, "status")
				}
			})
		}
	}
}
func TestEncodedEnvironmentIDKeepsRoute(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(fmt.Sprint(isEdge), func(t *testing.T) {
			m := newTestEnvironmentMiddleware()
			var forwarded string
			agent := escapedRouter()
			agent.GET("/api/environments/:id/vulnerabilities/runs", func(c *echo.Context) error { return c.String(200, "runs") })
			witness := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = r.URL.EscapedPath(); agent.ServeHTTP(w, r) })
			hub := escapedHub(t, isEdge, m, witness)
			rr := httptest.NewRecorder()
			hub.ServeHTTP(rr, httptest.NewRequest("GET", "/api/environments/%65xample/vulnerabilities/runs", nil))
			require.Equal(t, 200, rr.Code, "percent-encoded env ID identifies the same registered host")
			require.Equal(t, "/api/environments/0/vulnerabilities/runs", forwarded)
		})
	}
}
