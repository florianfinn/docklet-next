// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original cache invalidation after restore for issue #32.

package role

import (
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
)

func (s *RoleService) InvalidateAllPermissions() {
	s.cacheFillMu.Lock()
	defer s.cacheFillMu.Unlock()
	s.userCacheGen.Add(1)
	s.apiKeyCacheGen.Add(1)
	var users, keys []string
	s.userCache.Range(func(id string, _ *authz.PermissionSet) bool { users = append(users, id); return true })
	s.apiKeyCache.Range(func(id string, _ *authz.PermissionSet) bool { keys = append(keys, id); return true })
	s.userCache.DeleteMany(users)
	s.apiKeyCache.DeleteMany(keys)
}
