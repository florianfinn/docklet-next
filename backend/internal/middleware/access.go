// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package middleware

import (
	"bytes"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/labstack/echo/v5"
	"io"
)

func AccessMiddleware(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		ps, _ := PermissionsFromContext(ctx.Context())
		p := access.FromPermissions(ps)
		if p.IsUser() {
			_, env, params := access.Match(ctx.Method(), ctx.URL().Path)
			if actual := ctx.Param("id"); actual != "" {
				env = actual
			}
			params.ProjectID = ctx.Param("projectId")
			params.ContainerID = ctx.Param("containerId")
			params.RunID = ctx.Param("runId")
			if ctx.Operation().OperationID == "deploy-project" {
				req := humaecho.Unwrap(ctx).Request()
				if req.Body != nil {
					body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
					if err != nil {
						_ = huma.WriteErr(api, ctx, 400, "Invalid deployment options")
						return
					}
					req.Body = io.NopCloser(bytes.NewReader(body))
					params.Body = body
				}
			}
			if ctx.Operation().OperationID == "list-projects" {
				body, err := access.ProjectList(ctx.Context(), p, env, humaecho.Unwrap(ctx).Request().URL.Query())
				if err != nil {
					_ = access.WriteProblem(ctx, 503, "access.unavailable")
					return
				}
				ctx.SetHeader("Content-Type", "application/json")
				ctx.SetStatus(200)
				_ = json.NewEncoder(ctx.BodyWriter()).Encode(body)
				return
			}
			if d := access.Decide(ctx.Context(), p, ctx.Operation().OperationID, env, params); d != access.Allow {
				_ = access.WriteProblem(ctx, d.Status(), string(d))
				return
			}
		}
		next(ctx)
	}
}
func echoAccessDecision(c *echo.Context, ps *authz.PermissionSet) access.Decision {
	op, env, params := access.Match(c.Request().Method, c.Request().URL.Path)
	return access.Decide(c.Request().Context(), access.FromPermissions(ps), op, env, params)
}
