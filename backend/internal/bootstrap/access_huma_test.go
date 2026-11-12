// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package bootstrap

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestHumaOwnRunUsesActualEnvironmentParameter(t *testing.T) {
	router := echo.New()
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("access", "test"))
	controller := &access.Controller{RunOwner: func(_ context.Context, env, id string) (string, error) {
		if env == "0" || env == "example" {
			return "viewer", nil
		}
		return "", errors.New("missing environment")
	}}
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		c := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, access.Synthesize("viewer", nil))
		next(huma.WithContext(ctx, access.WithController(c, controller)))
	})
	api.UseMiddleware(middleware.AccessMiddleware(api))
	type input struct {
		EnvironmentID string `path:"id"`
		RunID         string `path:"runId"`
	}
	type output struct {
		Body struct {
			Allowed bool `json:"allowed"`
		}
	}
	huma.Register(api, huma.Operation{OperationID: "get-hub-runtime-run", Method: "GET", Path: "/environments/{id}/runtime-runs/{runId}"}, func(context.Context, *input) (*output, error) {
		out := &output{}
		out.Body.Allowed = true
		return out, nil
	})
	for _, env := range []string{"0", "example"} {
		t.Run(env, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/environments/"+env+"/runtime-runs/6ba7b810-9dad-11d1-80b4-00c04fd430c8", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), `"allowed":true`)
		})
	}
}
