// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
)

func (s *AuthService) ViewerContext(ctx context.Context, sessionID, userID string) context.Context {
	if s == nil || s.SignInMode() != "local" || s.sessionService == nil || sessionID == "" {
		return ctx
	}
	return s.sessionService.ViewerContext(ctx, sessionID, userID)
}

// ViewerMiddleware runs after Huma authentication; public and machine requests
// without a local session keep their existing credential lifetime.
func (s *AuthService) ViewerMiddleware(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		id, _ := middleware.GetCurrentSessionIDFromContext(ctx.Context())
		userID, _ := middleware.GetUserIDFromContext(ctx.Context())
		viewerCtx := s.ViewerContext(ctx.Context(), id, userID)
		if strings.HasSuffix(ctx.URL().Path, "/auth/me") && sessionlife.Check(viewerCtx) != nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Session expired")
			return
		}
		next(huma.WithContext(ctx, viewerCtx))
	}
}
