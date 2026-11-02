// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBindRoutesAdminOnlyBeforeData(t *testing.T) {
	router, raw := humatest.New(t)
	api := huma.NewGroup(raw, "/api")
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, access.Synthesize("viewer", nil))))
	})
	api.UseMiddleware(middleware.AccessMiddleware(api))
	registerBindRoutes(api, &VolumeService{})
	for _, route := range []struct{ method, path, body string }{{"GET", "/volumes/binds", ""}, {"POST", "/volumes/binds/size", `{"key":"bind:2f7372762f617070"}`}} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(route.method, "/api/environments/0"+route.path, strings.NewReader(route.body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		require.Equal(t, 403, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), "access.admin-required")
	}
}
