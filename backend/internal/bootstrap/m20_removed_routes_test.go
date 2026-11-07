// Source: docklet-next/issues/358
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original authenticated production-router proof of removed webhook ingress.

package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
)

func TestM20RemovedWebhookRoutesAuthenticated(t *testing.T) {
	f := newSignInRoutingFixture(t)
	deps := api.HandlerDeps{Auth: auth.New(f.svc, f.users, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles), Environment: environment.New(&environment.EnvironmentService{}, nil, f.keys, nil, f.cfg, nil), Project: project.New(&project.ProjectService{}, nil), Container: container.New(&container.ContainerService{}, nil, nil, nil), System: system.New(&system.SystemService{}, nil, nil, nil, nil)}
	router, _ := newRouter(RouterParams{Context: t.Context(), Lifecycle: fxtest.NewLifecycle(t), Config: f.cfg, HandlerDeps: deps, AuthMiddleware: auth.NewAuthMiddleware(f.svc, f.cfg).WithApiKeyValidator(f.keys).WithPermissionResolver(f.roles), TunnelRegistry: edge.NewTunnelRegistry()})
	for _, id := range []string{"list-webhooks", "create-webhook", "update-webhook", "delete-webhook", "POST /api/webhooks/trigger/:token"} {
		require.NotContains(t, access.Rules, id)
		for _, r := range access.Routes {
			require.NotEqual(t, id, r.Operation)
		}
	}
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/environments/0/webhooks"},
		{"POST", "/api/environments/0/webhooks"},
		{"PATCH", "/api/environments/0/webhooks/example"},
		{"DELETE", "/api/environments/0/webhooks/example"},
		{"POST", "/api/webhooks/trigger/x"},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
			req.Header.Set("Authorization", "Bearer "+f.tokens.AccessToken)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
		})
	}
}
