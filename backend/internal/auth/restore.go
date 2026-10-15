// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original cache invalidation after restore for issue #32.

package auth

import (
	"context"
	"errors"
)

func (s *AuthService) InvalidateAllTokenCache() {
	if s == nil || s.tokenCache == nil {
		return
	}
	s.invalidateTokenCacheInternal(func(verifiedTokenEntry) bool { return true })
}

func (s *AuthService) RevokeAllSessions(ctx context.Context) error {
	if s == nil || s.sessionService == nil {
		return errors.New("session service unavailable")
	}
	if err := s.sessionService.RevokeAllSessions(ctx); err != nil {
		return err
	}
	s.InvalidateAllTokenCache()
	return nil
}
