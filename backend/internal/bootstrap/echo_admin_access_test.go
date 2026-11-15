// Source: docklet-next/issues/378
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original router-derived administrator denial and handler-entry guards.

package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/api/ws"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
)

func TestEchoAdministratorRoutesDenyUserBeforeHandler(t *testing.T) {
	f := newSignInRoutingFixture(t)
	viewer, err := f.users.CreateUser(t.Context(), &user.User{Username: "viewer"})
	require.NoError(t, err)
	tokens, err := f.svc.CompleteLogin(t.Context(), viewer, authtypes.SessionMeta{}, "local")
	require.NoError(t, err)
	authMiddleware := auth.NewAuthMiddleware(f.svc, f.cfg).WithApiKeyValidator(f.keys).WithPermissionResolver(f.roles)
	deps := api.HandlerDeps{Auth: auth.New(f.svc, f.users, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles), Environment: environment.New(&environment.EnvironmentService{}, nil, f.keys, nil, f.cfg, nil), Project: project.New(&project.ProjectService{}, nil), Container: container.New(&container.ContainerService{}, nil, nil, nil), System: system.New(&system.SystemService{}, nil, nil, nil, nil)}
	production, _ := newRouter(RouterParams{Context: t.Context(), Lifecycle: fxtest.NewLifecycle(t), Config: f.cfg, HandlerDeps: deps, AuthMiddleware: authMiddleware, TunnelRegistry: edge.NewTunnelRegistry()})
	humaRoutes := map[string]bool{}
	for path, item := range api.SetupAPIForSpec().OpenAPI().Paths {
		parts := strings.Split(path, "/")
		for i, part := range parts {
			if strings.HasPrefix(part, "{") {
				parts[i] = ":" + strings.Trim(part, "{}")
			}
		}
		for method, op := range map[string]*huma.Operation{"GET": item.Get, "POST": item.Post, "PUT": item.Put, "PATCH": item.Patch, "DELETE": item.Delete, "HEAD": item.Head, "OPTIONS": item.Options, "TRACE": item.Trace} {
			if op != nil {
				humaRoutes[method+" /api"+strings.Join(parts, "/")] = true
			}
		}
	}
	// Capture actual endpoint registrations, including every group/route middleware.
	// Replacing only the final handler prevents an accidentally admitted profile or
	// WebSocket from doing work, and detects entry independently of response status.
	sentinels := newEchoInternal()
	registrations := map[string]echo.Route{}
	sentinels.OnAddRoute = func(route echo.Route) error {
		registrations[route.Method+" "+route.Path] = route
		return nil
	}
	// Supporting coarse grants isolate the fail-closed access decision from the
	// later permission check. This remains a simple user, never an administrator.
	sentinelAuth := authMiddleware.WithPermissionResolver(echoAdminSupportingGrants{f.roles})
	ws.NewWebSocketHandler(sentinels.Group("/api"), nil, nil, nil, nil, sentinelAuth, f.cfg)
	sentinels.OnAddRoute = nil
	count := 0
	for _, route := range production.Router().Routes() {
		key := route.Method + " " + route.Path
		if !strings.HasPrefix(route.Path, "/api/") || humaRoutes[key] || route.Method == "echo_route_not_found" || access.Rules[key].Kind != access.Admin {
			continue
		}
		count++
		t.Run(key, func(t *testing.T) {
			registered, ok := registrations[key]
			require.True(t, ok, "administrator endpoint must retain its actual middleware in the sentinel router")
			reached := false
			registered.Handler = func(c *echo.Context) error { reached = true; return c.NoContent(http.StatusNoContent) }
			_, err := sentinels.AddRoute(registered)
			require.NoError(t, err)
			path := route.Path
			for _, param := range route.Parameters {
				value := "example"
				if param == "id" {
					value = "0"
				}
				if param == "name" {
					value = "goroutine"
				}
				path = strings.ReplaceAll(path, ":"+param, value)
			}
			for _, router := range []*echo.Echo{sentinels, production} {
				req := httptest.NewRequest(route.Method, path, http.NoBody)
				req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
				req.Header.Set("Origin", f.cfg.AppUrl)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				require.False(t, reached, "administrator endpoint handler reached by simple user: %s", key)
				require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
				if strings.HasPrefix(route.Path, "/api/debug/pprof") {
					var body common.APIError
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
					require.Equal(t, "access.admin-required", string(body.Code))
					require.Equal(t, "access.admin-required", body.Message)
				} else {
					var body struct {
						Code string `json:"code"`
					}
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
					require.Equal(t, "access.admin-required", body.Code)
				}
			}
		})
	}
	require.GreaterOrEqual(t, count, 12, "include diagnostics, system stats and GET/POST pprof symbol")
	t.Logf("router-derived administrator endpoints=%d", count)
}

// A signed-in user's ordinary roles still drive the complete production-router
// check; the sentinel check also proves a coarse grant cannot bypass the rule.
type echoAdminSupportingGrants struct{ auth.PermissionResolver }

func (r echoAdminSupportingGrants) ResolvePermissions(ctx context.Context, id string) (*authz.PermissionSet, error) {
	ps, err := r.PermissionResolver.ResolvePermissions(ctx, id)
	if err == nil {
		ps.AddGlobal(authz.PermDiagnosticsRead, authz.PermSystemRead)
	}
	return ps, err
}
