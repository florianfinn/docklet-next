// Source: getarcaneapp/arcane/backend/internal/role/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed OIDC and passkeys.; Service-account filter removed with the federated account cut; split from service.go unchanged for the file-size limit.

package role

import (
	"context"
	"fmt"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
	"gorm.io/gorm"
)

// ResolveApiKeyPermissions returns the PermissionSet for an API key. Caches
// per-key. Falls back to an empty set (deny-all) if the key has no perms.
func (s *RoleService) ResolveApiKeyPermissions(ctx context.Context, apiKeyID string) (*authz.PermissionSet, error) {
	if ps, ok, _ := s.apiKeyCache.Get(apiKeyID); ok {
		return ps, nil
	}
	gen := s.apiKeyCacheGen.Load()
	permissions, err := s.resolveApiKeyPermissionsInDBInternal(s.db.WithContext(ctx), apiKeyID)
	if err != nil {
		return nil, err
	}
	s.cacheFillMu.Lock()
	if s.apiKeyCacheGen.Load() == gen {
		s.apiKeyCache.Set(apiKeyID, permissions)
	}
	s.cacheFillMu.Unlock()
	return permissions, nil
}

func (s *RoleService) resolveApiKeyPermissionsInDBInternal(db *gorm.DB, apiKeyID string) (*authz.PermissionSet, error) {
	var grants []ApiKeyPermission
	if err := db.Where("api_key_id = ?", apiKeyID).Find(&grants).Error; err != nil {
		return nil, fmt.Errorf("failed to resolve api key permissions: %w", err)
	}
	permissions := authz.NewPermissionSet()
	for _, grant := range grants {
		if grant.EnvironmentID == nil {
			permissions.AddGlobal(grant.Permission)
		} else {
			permissions.AddEnv(*grant.EnvironmentID, grant.Permission)
		}
	}
	return permissions, nil
}

// SetApiKeyPermissions replaces every permission row on the given API key
// atomically. Validation that the granted permissions don't exceed the
// creator's capabilities happens in the handler layer.
func (s *RoleService) SetApiKeyPermissions(ctx context.Context, apiKeyID string, grants []ApiKeyPermission) error {
	err := dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		return s.SetApiKeyPermissionsInDB(ctx, tx, apiKeyID, grants)
	})
	if err != nil {
		return err
	}
	s.InvalidateApiKey(apiKeyID)
	return nil
}

// SetApiKeyPermissionsInDB replaces permission rows using the supplied transaction or database handle.
func (s *RoleService) SetApiKeyPermissionsInDB(ctx context.Context, tx *gorm.DB, apiKeyID string, grants []ApiKeyPermission) error {
	for i := range grants {
		grants[i].ApiKeyID = apiKeyID
	}
	if err := tx.WithContext(ctx).Where("api_key_id = ?", apiKeyID).Delete(&ApiKeyPermission{}).Error; err != nil {
		return fmt.Errorf("failed to clear api key permissions: %w", err)
	}
	if len(grants) > 0 {
		if err := tx.WithContext(ctx).Create(&grants).Error; err != nil {
			return fmt.Errorf("failed to insert api key permissions: %w", err)
		}
	}
	return nil
}

// InvalidateApiKey drops the cached PermissionSet for one API key.
func (s *RoleService) InvalidateApiKey(apiKeyID string) {
	s.cacheFillMu.Lock()
	defer s.cacheFillMu.Unlock()
	s.apiKeyCacheGen.Add(1)
	s.apiKeyCache.Delete(apiKeyID)
}
