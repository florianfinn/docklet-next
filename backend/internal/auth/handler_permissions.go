// Source: getarcaneapp/arcane/backend/internal/auth/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; removed OIDC/passkeys; startup-mode dispatch before local credentials; shared reserved synthetic agent identity; M15 simple-user rights and identity contracts.

package auth

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	userdomain "github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"log/slog"
)

func resolveUserPermissionsInternal(ctx context.Context, permResolver PermissionResolver, authenticatedUser *userdomain.User) *authz.PermissionSet {
	if authenticatedUser == nil {
		return authz.NewPermissionSet()
	}
	if permResolver == nil {
		return access.Synthesize(authenticatedUser.ID, nil)
	}
	revision := access.Revision(authenticatedUser.ID)
	ps, err := permResolver.ResolvePermissions(ctx, authenticatedUser.ID)
	if err != nil || ps == nil {
		slog.WarnContext(ctx, "failed to resolve user permissions", "error", err, "userId", authenticatedUser.ID)
		return access.Synthesize(authenticatedUser.ID, nil)
	}
	copy := *ps
	copy.RightsRevision = revision
	return &copy
}

func resolveApiKeyPermissionsInternal(ctx context.Context, permResolver PermissionResolver, apiKeyID string) *authz.PermissionSet {
	if permResolver == nil || apiKeyID == "" {
		return nil
	}
	ps, err := permResolver.ResolveApiKeyPermissions(ctx, apiKeyID)
	if err != nil {
		slog.WarnContext(ctx, "failed to resolve api key permissions", "error", err, "apiKeyId", apiKeyID)
		return nil
	}
	return ps
}
