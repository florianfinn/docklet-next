// Source: getarcaneapp/arcane/backend/internal/settings/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-14
// License: BSD-3-Clause
// Deviations: Local module namespace; environment values follow Arcane initialization.

package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

func (s *SettingsService) upsertEnvSetting(ctx context.Context, tx *gorm.DB, key, envVal string) error {
	var existing SettingVariable
	err := tx.Where("key = ?", key).First(&existing).Error

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		newVar := SettingVariable{Key: key, Value: envVal}
		if createEnvSettingErr := tx.Create(&newVar).Error; createEnvSettingErr != nil {
			return fmt.Errorf("persist env setting %s: %w", key, createEnvSettingErr)
		}
		slog.DebugContext(ctx, "Created setting from environment", "key", key)
	case err != nil:
		return fmt.Errorf("check setting %s: %w", key, err)
	default:
		if existing.Value != envVal {
			if updateEnvSettingErr := tx.Model(&existing).Update("value", envVal).Error; updateEnvSettingErr != nil {
				return fmt.Errorf("update env setting %s: %w", key, updateEnvSettingErr)
			}
			slog.DebugContext(ctx, "Updated setting from environment", "key", key)
		}
	}

	return nil
}
