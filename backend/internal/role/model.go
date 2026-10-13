// Source: getarcaneapp/arcane/backend/internal/role/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed OIDC and passkeys.

package role

import "github.com/florianfinn/docklet-next/backend/internal/database"

// Role is a named permission set. Built-in roles (Admin, Editor, Deployer,
// Viewer) are seeded by migration 054 and cannot be edited or deleted.
type Role struct {
	database.BaseModel

	Name        string               `json:"name" gorm:"column:name;not null;uniqueIndex" sortable:"true"`
	Description *string              `json:"description,omitempty" gorm:"column:description"`
	Permissions database.StringSlice `json:"permissions" gorm:"column:permissions;type:text;not null"`
	BuiltIn     bool                 `json:"builtIn" gorm:"column:built_in;not null;default:false" sortable:"true"`
}

func (Role) TableName() string { return "roles" }

// UserRoleAssignment binds a user to a role, optionally scoped to one
// environment. EnvironmentID == nil means "global" — the role's permissions
// apply across all environments AND to org-level resources.
//
// Source records the manual assignment origin.
type UserRoleAssignment struct {
	database.BaseModel

	UserID        string  `json:"userId" gorm:"column:user_id;not null;index"`
	RoleID        string  `json:"roleId" gorm:"column:role_id;not null;index"`
	EnvironmentID *string `json:"environmentId,omitempty" gorm:"column:environment_id;index"`
	Source        string  `json:"source" gorm:"column:source;not null;default:'manual'"`
}

func (UserRoleAssignment) TableName() string { return "user_role_assignments" }

const (
	// Assignment source values stored in UserRoleAssignment.Source.

	RoleAssignmentSourceManual = "manual"
)

// ApiKeyPermission is one permission grant on an API key, optionally scoped to
// a single environment. Permissions are stored per-row (rather than as a JSON
// column on api_keys) so we can index by (api_key_id, permission) for fast
// lookups in the auth bridge.
type ApiKeyPermission struct {
	database.BaseModel

	ApiKeyID      string  `json:"apiKeyId" gorm:"column:api_key_id;not null;index"`
	Permission    string  `json:"permission" gorm:"column:permission;not null"`
	EnvironmentID *string `json:"environmentId,omitempty" gorm:"column:environment_id"`
}

func (ApiKeyPermission) TableName() string { return "api_key_permissions" }
