// Source: getarcaneapp/arcane/backend/internal/session/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed federated credentials.

package session

import (
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/user"
)

const (
	UserSessionSourceLocal   = "local"
	UserSessionSourceOidc    = "oidc"
	UserSessionSourcePasskey = "passkey"

	// PasskeyMFAMethod and RecoveryCodeMFAMethod name the second factor a login
	// completed with, recorded on the session for auditing.
	PasskeyMFAMethod      = "passkey"
	RecoveryCodeMFAMethod = "recovery_code"
)

type UserSession struct {
	database.BaseModel

	UserID           string     `json:"userId" gorm:"column:user_id;not null;index"`
	User             *user.User `json:"user,omitempty" gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	RefreshTokenHash string     `json:"-" gorm:"column:refresh_token_hash;not null;uniqueIndex"`
	UserAgent        *string    `json:"userAgent,omitempty" gorm:"column:user_agent"`
	IPAddress        *string    `json:"ipAddress,omitempty" gorm:"column:ip_address"`
	Source           string     `json:"source,omitempty" gorm:"column:source"`
	LastUsedAt       time.Time  `json:"lastUsedAt" gorm:"column:last_used_at;not null"`
	ExpiresAt        time.Time  `json:"expiresAt" gorm:"column:expires_at;not null;index"`
	RevokedAt        *time.Time `json:"revokedAt,omitempty" gorm:"column:revoked_at"`
	MFAMethod        *string    `json:"mfaMethod,omitempty" gorm:"column:mfa_method"`
	MFAVerifiedAt    *time.Time `json:"mfaVerifiedAt,omitempty" gorm:"column:mfa_verified_at"`
}

func (UserSession) TableName() string {
	return "user_sessions"
}
