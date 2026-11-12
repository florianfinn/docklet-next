// Source: getarcaneapp/arcane/backend/internal/role/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed OIDC and passkeys.; Service-account filter removed with the federated account cut; M15 simple-user rights and identity contracts.

package role

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"time"
)

func (s *RoleService) ResolveExecutionPermissions(ctx context.Context, userID, keyID string) (*authz.PermissionSet, error) {
	if userID == "" {
		return nil, errors.New("requesting user unavailable")
	}
	db := s.db.WithContext(ctx)
	var user struct {
		ID         string
		DisabledAt *time.Time
	}
	if err := db.Table("users").Select("id, disabled_at").Where("id = ?", userID).Take(&user).Error; err != nil {
		return nil, fmt.Errorf("failed to load requesting user: %w", err)
	}
	if keyID == "" {
		if user.DisabledAt != nil {
			return nil, errors.New("requesting user is disabled")
		}
		return s.ResolveUserPermissionsInDB(ctx, db, userID)
	}
	var key struct {
		ID            string
		Kind          string
		UserID        *string
		EnvironmentID *string
		ExpiresAt     *time.Time
		ManagedBy     *string
	}
	if err := db.Table("api_keys").Select("id, kind, user_id, environment_id, expires_at, managed_by").Where("id = ?", keyID).Take(&key).Error; err != nil {
		return nil, fmt.Errorf("failed to load requesting API key: %w", err)
	}
	if key.UserID == nil || *key.UserID != userID || (key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now())) {
		return nil, errors.New("requesting API key is no longer valid")
	}
	// The configured static administrator key retains its existing authority.
	static := key.ManagedBy != nil && *key.ManagedBy == "admin_account_default_api_key"
	if !static && user.DisabledAt != nil {
		return nil, errors.New("requesting user is disabled")
	}
	var permissions *authz.PermissionSet
	var err error
	switch key.Kind {
	case "personal":
		permissions, err = s.ResolveUserPermissionsInDB(ctx, db, userID)
	case "scoped":
		if !static {
			owner, e := s.ResolveUserPermissionsInDB(ctx, db, userID)
			if e != nil || !owner.IsGlobalAdmin() {
				return nil, errors.New("scoped key owner is not an administrator")
			}
		}
		permissions, err = s.resolveApiKeyPermissionsInDBInternal(db, keyID)
	default:
		return nil, errors.New("requesting API key kind is invalid")
	}
	if err != nil {
		return nil, err
	}
	if key.EnvironmentID == nil {
		return permissions, nil
	}
	if *key.EnvironmentID == "" {
		return nil, errors.New("requesting API key environment scope is invalid")
	}
	restricted := authz.NewPermissionSet()
	for permission := range permissions.Global {
		restricted.AddEnv(*key.EnvironmentID, permission)
	}
	for permission := range permissions.PerEnv[*key.EnvironmentID] {
		restricted.AddEnv(*key.EnvironmentID, permission)
	}
	return restricted, nil
}
