// Source: docklet-next/issues/668
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original trusted-proxy production-router regression.

package bootstrap

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
)

func securityAgentRouter(t *testing.T, cfg *config.Config) *echo.Echo {
	t.Helper()
	r, _ := newRouter(RouterParams{Context: t.Context(), Lifecycle: fxtest.NewLifecycle(t), Config: cfg,
		HandlerDeps:    api.HandlerDeps{Project: project.New(&project.ProjectService{}, nil), Container: container.New(&container.ContainerService{}, nil, nil, nil), System: system.New(&system.SystemService{}, nil, nil, nil, nil)},
		AuthMiddleware: auth.NewAuthMiddleware(nil, cfg), TunnelRegistry: edge.NewTunnelRegistry()})
	return r
}

func TestConfiguredProxyTrust(t *testing.T) {
	t.Setenv("LISTEN", "127.0.0.1")
	t.Setenv("TRUSTED_PROXIES", "")
	defaults := config.Load()
	require.NotEmpty(t, defaults.TrustedProxies)
	private := net.IPv4(10, 1, 2, 3).String()
	lan := net.IPv4(192, 168, 1, 2).String()
	link := net.IPv4(169, 254, 1, 2).String()
	for _, tc := range []struct{ name, ranges, peer, want string }{
		{"private", "192.0.2.0/24", private, private},
		{"link-local", "192.0.2.0/24", link, link},
		{"loopback-not-listed", "192.0.2.0/24", "127.0.0.1", "127.0.0.1"},
		{"listed", "192.0.2.0/24", "192.0.2.5", "203.0.113.9"},
		{"auto-default", defaults.TrustedProxies, lan, lan},
		{"auto-listed", defaults.TrustedProxies, "127.0.0.1", "203.0.113.9"},
		{"direct", "", private, private},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := securityAgentRouter(t, &config.Config{AgentMode: true, TrustedProxies: tc.ranges, Environment: config.AppEnvironmentTest})
			r.GET("/ip", func(c *echo.Context) error { return c.String(200, c.RealIP()) })
			req := httptest.NewRequest("GET", "/ip", nil)
			req.RemoteAddr = net.JoinHostPort(tc.peer, "1234")
			req.Header.Set("X-Forwarded-For", "203.0.113.9")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, tc.want, rec.Body.String())
		})
	}
}
