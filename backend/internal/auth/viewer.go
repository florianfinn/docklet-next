// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation; M15 simple-user rights and identity contracts.

package auth

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
)

func (s *AuthService) ViewerContext(ctx context.Context, sessionID, userID string) context.Context {
	if s == nil || s.userService == nil {
		return ctx
	}
	revision := access.Revision(userID)
	ps, err := s.userService.ResolveUserPermissions(ctx, userID)
	if err == nil && ps != nil {
		copy := *ps
		copy.RightsRevision = revision
		ps = &copy
	}
	return s.ViewerContextForPermissions(ctx, sessionID, userID, ps)
}

// ViewerContextForPermissions binds the revision captured before permission
// resolution. A mutation between authentication and upgrade cannot renew it.
func (s *AuthService) ViewerContextForPermissions(ctx context.Context, sessionID, userID string, ps *authz.PermissionSet) context.Context {
	if s == nil {
		return ctx
	}
	if s.SignInMode() == "local" && s.sessionService != nil && sessionID != "" {
		ctx = s.sessionService.ViewerContext(ctx, sessionID, userID)
	}
	if s.userService == nil || userID == "" || ps == nil || ps.IsGlobalAdmin() {
		return ctx
	}
	original := ctx
	revision := ps.RightsRevision
	return sessionlife.WithSession(ctx, sessionID, userID, func(check context.Context) error {
		if sessionlife.Check(original) != nil {
			return access.ErrRevoked
		}
		u, err := s.userService.GetUserByID(check, userID)
		if err != nil || u.DisabledAt != nil || access.Revision(userID) != revision {
			return access.ErrRevoked
		}
		return nil
	})
}

// ViewerMiddleware runs after Huma authentication; public and machine requests
// without a local session keep their existing credential lifetime.
func (s *AuthService) ViewerMiddleware(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		id, _ := middleware.GetCurrentSessionIDFromContext(ctx.Context())
		userID, _ := middleware.GetUserIDFromContext(ctx.Context())
		ps, _ := middleware.PermissionsFromContext(ctx.Context())
		viewerCtx := s.ViewerContextForPermissions(ctx.Context(), id, userID, ps)
		if strings.HasSuffix(ctx.URL().Path, "/auth/me") && sessionlife.Check(viewerCtx) != nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Session expired")
			return
		}
		next(huma.WithContext(ctx, viewerCtx))
	}
}
