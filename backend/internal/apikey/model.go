// Source: getarcaneapp/arcane/backend/internal/apikey/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package apikey

import (
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/role"
)

const (
	// ApiKeyKindScoped keys carry their own permission grant rows.
	ApiKeyKindScoped = "scoped"
	// ApiKeyKindPersonal keys have no grants; they inherit the owner's role
	// permissions at authentication time.
	ApiKeyKindPersonal = "personal"
)

type ApiKey struct {
	database.BaseModel

	Name             string                  `json:"name" gorm:"column:name;not null" sortable:"true"`
	Description      *string                 `json:"description,omitempty" gorm:"column:description"`
	KeyHash          string                  `json:"-" gorm:"column:key_hash;not null"`
	KeyPrefix        string                  `json:"keyPrefix" gorm:"column:key_prefix;not null"`
	ManagedBy        *string                 `json:"-" gorm:"column:managed_by"`
	Kind             string                  `json:"kind" gorm:"column:kind;not null;default:scoped"`
	UserID           *string                 `json:"userId,omitempty" gorm:"column:user_id"`
	EnvironmentID    *string                 `json:"environmentId,omitempty" gorm:"column:environment_id"`
	ExpiresAt        *time.Time              `json:"expiresAt,omitempty" gorm:"column:expires_at" sortable:"true"`
	LastUsedAt       *time.Time              `json:"lastUsedAt,omitempty" gorm:"column:last_used_at" sortable:"true"`
	PermissionGrants []role.ApiKeyPermission `json:"-" gorm:"foreignKey:ApiKeyID;references:ID;constraint:OnDelete:CASCADE"`
}

func (ApiKey) TableName() string {
	return "api_keys"
}
