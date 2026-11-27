// Source: docklet-next/issues/670
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original hub and agent header coverage over production routers.

package bootstrap

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProductionSecurityHeaders(t *testing.T) {
	hub, _ := newEventIngestionLimitRouter(t)
	agent := securityAgentRouter(t, &config.Config{AgentMode: true, Environment: config.AppEnvironmentTest})
	for _, r := range []*echo.Echo{hub, agent} {
		r.GET("/api/header-error", func(c *echo.Context) error { return echo.NewHTTPError(418, "test refusal") })
		server := httptest.NewServer(r)
		for _, path := range []string{"/", "/assets/placeholder.txt", "/api/health", "/api/header-error"} {
			resp, err := server.Client().Get(server.URL + path)
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, resp.Body)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"), path)
			require.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"), path)
			require.Equal(t, "frame-ancestors 'none'", resp.Header.Get("Content-Security-Policy"), path)
			require.Equal(t, "same-origin", resp.Header.Get("Referrer-Policy"), path)
			if path == "/api/header-error" {
				require.Equal(t, 418, resp.StatusCode)
			}
			if r == hub && !strings.HasPrefix(path, "/api") {
				require.Equal(t, 200, resp.StatusCode)
			}
		}
		server.Close()
	}
}
