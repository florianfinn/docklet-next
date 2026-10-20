// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

package session

import (
	"context"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
)

func (s *SessionService) ViewerContext(ctx context.Context, sessionID, userID string) context.Context {
	return sessionlife.WithSession(ctx, sessionID, userID, func(ctx context.Context) error {
		record, err := s.GetSessionByID(ctx, sessionID)
		if err != nil {
			return err
		}
		if record.UserID != userID {
			return common.ErrInvalidToken
		}
		if err := ValidateActive(record); err != nil {
			return err
		}
		var count int64
		if err := s.db.WithContext(ctx).Model(&user.User{}).Where("id = ?", userID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return common.ErrInvalidToken
		}
		return nil
	})
}
