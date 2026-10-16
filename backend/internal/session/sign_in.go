// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original atomic mode transition and session invalidation.

package session

import (
	"context"
	"errors"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/types/auth"
	"gorm.io/gorm"
)

func (s *SessionService) ActivateSignInMode(ctx context.Context, mode auth.SignInMode) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var previous settings.SettingVariable
		err := tx.Where("key = ?", "authLastMode").First(&previous).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && previous.Value == string(mode) {
			return nil
		}
		now := time.Now().UTC()
		if err := tx.Model(&UserSession{}).Where("revoked_at IS NULL").Updates(map[string]any{"revoked_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Save(&settings.SettingVariable{Key: "authLastMode", Value: string(mode)}).Error
	})
}
