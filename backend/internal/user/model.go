// Source: getarcaneapp/arcane/backend/internal/user/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed OIDC, passkeys and federated accounts; PostgreSQL avatar storage.

package user

import (
	"time"

	"github.com/florianfinn/docklet-next/types/user"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

type User struct {
	database.BaseModel

	Username               string           `json:"username" sortable:"true"`
	PasswordHash           string           `json:"-" gorm:"column:password_hash"`
	DisplayName            *string          `json:"displayName,omitempty" gorm:"column:display_name" sortable:"true" unorm:"nfc" trim:"true"`
	Email                  *string          `json:"email,omitempty" sortable:"true"`
	LastLogin              *time.Time       `json:"lastLogin,omitempty" gorm:"column:last_login" sortable:"true"`
	Locale                 *string          `json:"locale,omitempty" gorm:"column:locale"`
	TimeFormat             user.TimeFormat  `json:"timeFormat" gorm:"column:time_format;not null;default:auto"`
	FontSize               *int             `json:"fontSize,omitempty" gorm:"column:font_size"`
	Preferences            user.Preferences `json:"preferences" gorm:"column:preferences;serializer:json"`
	RequiresPasswordChange bool             `json:"requiresPasswordChange" gorm:"column:requires_password_change"`

	// Avatar metadata
	HasAvatar bool `json:"hasAvatar" gorm:"column:has_avatar;not null;default:false"`
}

func (User) TableName() string {
	return "users"
}

// UserAvatar represents the raw profile picture data for a user.
// Stored separately from the main User struct to prevent
// loading up to 2MB of binary data on every user query.
type UserAvatar struct {
	UserID   string `json:"userId" gorm:"column:user_id;primaryKey"`
	Data     []byte `json:"-" gorm:"column:data;type:bytea;not null"`
	MimeType string `json:"mimeType" gorm:"column:mime_type;not null"`
}

func (UserAvatar) TableName() string {
	return "user_avatars"
}

// Actor returns the identity and preferences used by requests and background work.
func (u *User) Actor() *user.Actor {
	if u == nil {
		return nil
	}
	return &user.Actor{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Preferences: u.Preferences}
}
