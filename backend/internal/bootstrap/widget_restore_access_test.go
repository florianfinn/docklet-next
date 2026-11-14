// Source: docklet-next/issues/378
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original widget and previous-image restore route decision parity tests.

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestWidgetAndPreviousImageRestoreRouteDecisionsMatchHuma(t *testing.T) {
	for _, tc := range []struct {
		method, path, operation string
		want                    access.Decision
	}{
		{"GET", "/service-widgets", "list-service-widgets", access.Allow},
		{"GET", "/service-widgets/{kind}", "get-service-widget", access.Allow},
		{"GET", "/service-widgets/{kind}/config", "get-service-widget-config", access.AdminRequired},
		{"PUT", "/service-widgets/{kind}/config", "put-service-widget-config", access.AdminRequired},
		{"POST", "/service-widgets/{kind}/test", "test-service-widget", access.AdminRequired},
		{"POST", "/service-widgets/overseerr/requests/{requestId}/approve", "approve-service-widget-request", access.AdminRequired},
		{"POST", "/service-widgets/overseerr/requests/{requestId}/decline", "decline-service-widget-request", access.AdminRequired},
		{"POST", "/environments/{id}/containers/{containerId}/previous-image/restore", "restore-container-image", access.AdminRequired},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			item := api.SetupAPIForSpec().OpenAPI().Paths[tc.path]
			require.NotNil(t, item)
			op := map[string]*huma.Operation{"GET": item.Get, "PUT": item.Put, "POST": item.Post}[tc.method]
			require.NotNil(t, op)
			require.Equal(t, tc.operation, op.OperationID)
			path := "/api" + strings.NewReplacer("{id}", "0", "{containerId}", "example", "{kind}", "overseerr", "{requestId}", "1").Replace(tc.path)
			matched, env, params := access.Match(tc.method, path)
			require.Equal(t, op.OperationID, matched)
			ps := access.Synthesize("viewer", nil)
			decision := access.Decide(t.Context(), access.FromPermissions(ps), matched, env, params)
			require.Equal(t, tc.want, decision)
			router := echo.New()
			registered := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("rule parity", "1"))
			registered.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, ps)))
			})
			registered.UseMiddleware(middleware.AccessMiddleware(registered))
			// The actual operation identity and path enter the production Huma access middleware.
			type input struct{}
			type output struct{ Body struct{} }
			reached := false
			huma.Register(registered, huma.Operation{OperationID: op.OperationID, Method: tc.method, Path: tc.path}, func(_ context.Context, _ *input) (*output, error) { reached = true; return &output{}, nil })
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tc.method, path, http.NoBody))
			if decision == access.Allow {
				require.Equal(t, 200, rec.Code, rec.Body.String())
				require.True(t, reached)
			} else {
				require.Equal(t, decision.Status(), rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), string(decision))
				require.False(t, reached)
			}
		})
	}
}
