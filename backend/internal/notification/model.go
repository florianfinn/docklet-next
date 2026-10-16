// Source: getarcaneapp/arcane/backend/internal/notification/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package notification

import (
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
)

type NotificationSettings struct {
	ID        uint                               `json:"id" gorm:"primaryKey"`
	Provider  notifications.NotificationProvider `json:"provider" gorm:"not null;index;type:varchar(50)"`
	Enabled   bool                               `json:"enabled" gorm:"default:false"`
	Config    database.JSON                      `json:"config" gorm:"type:text"`
	CreatedAt time.Time                          `json:"createdAt"`
	UpdatedAt time.Time                          `json:"updatedAt"`
}

func (NotificationSettings) TableName() string {
	return "notification_settings"
}
