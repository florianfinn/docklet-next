// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package bootstrap

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistrySyncAuthenticatedReceiverRequiresEffectGrants(t *testing.T) {
	for _, missing := range []string{authz.PermRegistriesCreate, authz.PermRegistriesDelete, ""} {
		t.Run(missing, func(t *testing.T) {
			db := newSignInRoutingFixture(t).db
			service := registry.NewContainerRegistryService(db, nil, nil, nil)
			router := echo.New()
			api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("sync", "test"))
			cfg := &config.Config{AgentMode: true, AgentToken: "example"}
			api.UseMiddleware(auth.NewHumaMiddleware(api, &auth.AuthService{}, nil, nil, nil, cfg))
			api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				require.True(t, middleware.HasAgentCredential(ctx.Context()))
				ps := authz.NewPermissionSet()
				for _, grant := range []string{authz.PermRegistriesUpdate, authz.PermRegistriesCreate, authz.PermRegistriesDelete} {
					if grant != missing {
						ps.AddGlobal(grant)
					}
				}
				next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, ps)))
			})
			registry.New(service, nil).RegisterAgentRoutes(api)
			r := httptest.NewRequest(http.MethodPost, "/api/container-registries/sync", strings.NewReader(`{"registries":[]}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set(middleware.HeaderAgentToken, "example")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if missing == "" {
				require.Equal(t, 200, w.Code, w.Body.String())
			} else {
				require.Equal(t, 403, w.Code, w.Body.String())
			}
		})
	}
}
