// Source: docklet-next/issues/670
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original Direct proxy CSP preservation transport regression.

package middleware

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func securityDirectRouter(t *testing.T, agentURL string) *echo.Echo {
	t.Helper()
	m := newTestEnvironmentMiddleware()
	m.resolver = func(context.Context, string) (string, *string, bool, error) {
		return agentURL, new("example"), true, nil
	}
	r := echo.New()
	r.Use(SecurityHeaders())
	api := attachMiddleware(r, m)
	api.Any("/environments/:id/*", func(c *echo.Context) error { return c.NoContent(500) })
	return r
}

func TestDirectProxySecurityHeaders(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
		w.Header().Set("Content-Type", "application/octet-stream")
		fmt.Fprint(w, "agent body")
	}))
	defer agent.Close()
	hub := httptest.NewServer(securityDirectRouter(t, agent.URL))
	defer hub.Close()
	resp, err := hub.Client().Get(hub.URL + "/api/environments/example/containers")
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "agent body", string(data))
	require.Equal(t, remenv.AgentContentSecurityPolicy, resp.Header.Get("Content-Security-Policy"))
	require.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
	require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	require.Equal(t, "same-origin", resp.Header.Get("Referrer-Policy"))
}
