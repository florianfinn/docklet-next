// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original frozen copy of the image update record as shipped in migration 001.

package schema

import (
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

// baselineImageUpdateRecord is the image update record exactly as migration 001
// created it. Columns added by later forward migrations (008) stay on
// imageupdate.ImageUpdateRecord only.
type baselineImageUpdateRecord struct {
	database.BaseModel

	ProjectID   string `json:"projectId,omitempty" gorm:"column:project_id;not null;default:'';index"`
	ServiceName string `json:"serviceName,omitempty" gorm:"column:service_name;not null;default:''"`
	PolicyKey   string `json:"-" gorm:"column:policy_key;not null;default:''"`

	ContainerID string `json:"containerId,omitempty" gorm:"column:container_id;not null;default:'';index"`
	ImageID     string `json:"imageId,omitempty" gorm:"column:image_id;not null;default:'';index"`

	CheckTime        time.Time `json:"checkTime" gorm:"column:check_time"`
	LatestVersion    *string   `json:"latestVersion,omitempty" gorm:"column:latest_version"`
	CurrentDigest    *string   `json:"currentDigest,omitempty" gorm:"column:current_digest"`
	LatestDigest     *string   `json:"latestDigest,omitempty" gorm:"column:latest_digest"`
	LastError        *string   `json:"lastError,omitempty" gorm:"column:last_error"`
	AuthMethod       *string   `json:"authMethod,omitempty" gorm:"column:auth_method"`
	AuthUsername     *string   `json:"-" gorm:"column:auth_username"`
	AuthRegistry     *string   `json:"authRegistry,omitempty" gorm:"column:auth_registry"`
	ID               string    `json:"id" gorm:"primaryKey;type:text"`
	Repository       string    `json:"repository"`
	Tag              string    `json:"tag"`
	UpdateType       string    `json:"updateType" gorm:"column:update_type"`
	CurrentVersion   string    `json:"currentVersion" gorm:"column:current_version"`
	ResponseTimeMs   int       `json:"responseTimeMs" gorm:"column:response_time_ms"`
	HasUpdate        bool      `json:"hasUpdate" gorm:"column:has_update"`
	UsedCredential   bool      `json:"usedCredential,omitempty" gorm:"column:used_credential"`
	NotificationSent bool      `json:"notificationSent" gorm:"column:notification_sent;default:false"`
	Platform         *string   `json:"platform,omitempty"`
}

func (baselineImageUpdateRecord) TableName() string { return "image_updates" }
