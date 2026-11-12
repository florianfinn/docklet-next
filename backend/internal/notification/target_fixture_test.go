// Source: docklet-next/issues/351
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original target setup bridge for retained provider delivery fixtures.

package notification

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"gorm.io/gorm"
	"sync"
)

var fixtureSettingsMu sync.Mutex

func (s *NotificationService) CreateOrUpdateSettings(ctx context.Context, p notifications.NotificationProvider, enabled bool, c database.JSON) (*NotificationSettings, error) {
	fixtureSettingsMu.Lock()
	defer fixtureSettingsMu.Unlock()
	var row NotificationSettings
	err := s.db.WithContext(ctx).Where("provider=?", p).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return s.SaveTarget(ctx, row.ID, dto.Update{Name: string(p), Provider: dto.Provider(p), Enabled: enabled, Config: base.JsonObject(c)})
}
