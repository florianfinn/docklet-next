// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original module route gate after existing access decisions.

package middleware

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/modules"
	"github.com/labstack/echo/v5"
	"net/http"
)

type ModuleSettings interface {
	IsFeatureEnabled(context.Context, features.ID) bool
}

// ModuleGate is operation middleware: global authentication and access run first.
// Management and data cleanup routes deliberately remain available while off.
func ModuleGate(api huma.API, settings ModuleSettings, agent bool, id features.ID) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if settings != nil && settings.IsFeatureEnabled(ctx.Context(), id) {
			next(ctx)
			return
		}
		c := humaecho.Unwrap(ctx)
		if !agent {
			_ = APINotFound(c)
			return
		}
		ctx.SetHeader("Content-Type", "application/problem+json")
		ctx.SetStatus(403)
		_ = api.Marshal(ctx.BodyWriter(), "application/json", &dto.Problem{Status: 403, Code: "feature-disabled", Feature: id})
	}
}

// APINotFound is the established unknown API route envelope. Module refusals
// use the same responder, including the requested path, after access checks.
func APINotFound(c *echo.Context) error {
	return c.JSON(http.StatusNotFound, struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}{Error: "API endpoint not found: " + c.Request().URL.Path})
}
