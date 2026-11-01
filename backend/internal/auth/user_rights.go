// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package auth

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"net/http"
	"strings"
)

func modeAuthError(err error) (int, string) {
	if err != nil && strings.HasPrefix(err.Error(), "access.") {
		return 403, err.Error()
	}
	return http.StatusUnauthorized, "Sign-in required"
}
func (s *AuthService) modePermissions(ctx context.Context, u *user.User) *authz.PermissionSet {
	revision := access.Revision(u.ID)
	ps, err := s.userService.ResolveUserPermissions(ctx, u.ID)
	if err != nil || ps == nil {
		return access.Synthesize(u.ID, nil)
	}
	if ps.IsGlobalAdmin() {
		return authz.SudoPermissionSet()
	}
	copy := *ps
	copy.RightsRevision = revision
	return &copy
}
