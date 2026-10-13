//go:build postgres_integration

// Source: docklet-next/docs/design/core-adoption.md
// Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Verifies service deletes against the retained PostgreSQL baseline.

package schema

import (
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/user"
)

func cascadeDatabase(t *testing.T) *database.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, dsn)
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminSQL.Close()) })
	migrated, err := database.Initialize(t.Context(), freshDatabase(t, admin, u), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, migrated.Close()) })
	return migrated
}

func rowCount(t *testing.T, db *database.DB, table, column, id string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Raw("SELECT count(*) FROM "+table+" WHERE "+column+" = ?", id).Scan(&count).Error)
	return count
}

func TestDeleteUserCascades(t *testing.T) {
	db := cascadeDatabase(t)
	actor := user.User{Username: "delete-user", PasswordHash: "test-only"}
	require.NoError(t, db.Create(&actor).Error)
	roleRecord := role.Role{Name: "delete-role", Permissions: database.StringSlice{"read"}}
	require.NoError(t, db.Create(&roleRecord).Error)
	require.NoError(t, db.Create(&role.UserRoleAssignment{UserID: actor.ID, RoleID: roleRecord.ID}).Error)
	require.NoError(t, db.Create(&session.UserSession{UserID: actor.ID, RefreshTokenHash: "test-only", LastUsedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}).Error)
	require.NoError(t, db.Create(&user.UserAvatar{UserID: actor.ID, Data: []byte("avatar"), MimeType: "image/png"}).Error)
	key := apikey.ApiKey{Name: "delete-key", KeyHash: "test-only", KeyPrefix: "del", UserID: &actor.ID}
	require.NoError(t, db.Create(&key).Error)
	require.NoError(t, db.Create(&role.ApiKeyPermission{ApiKeyID: key.ID, Permission: "read"}).Error)

	require.NoError(t, user.NewUserService(db, nil, nil).DeleteUser(t.Context(), actor.ID, nil))
	for _, row := range []struct{ table, column, id string }{
		{"user_sessions", "user_id", actor.ID},
		{"user_avatars", "user_id", actor.ID},
		{"user_role_assignments", "user_id", actor.ID},
		{"api_keys", "user_id", actor.ID},
		{"api_key_permissions", "api_key_id", key.ID},
	} {
		require.Zero(t, rowCount(t, db, row.table, row.column, row.id), row.table)
	}
}

func TestDeleteEnvironmentCascades(t *testing.T) {
	db := cascadeDatabase(t)
	actor := user.User{Username: "env-owner", PasswordHash: "test-only"}
	require.NoError(t, db.Create(&actor).Error)
	roleRecord := role.Role{Name: "env-role", Permissions: database.StringSlice{"read"}}
	require.NoError(t, db.Create(&roleRecord).Error)
	env := environment.Environment{Name: "delete-environment", ApiUrl: "https://agent.example.com", Status: "offline"}
	require.NoError(t, db.Create(&env).Error)
	require.NoError(t, db.Create(&role.UserRoleAssignment{UserID: actor.ID, RoleID: roleRecord.ID, EnvironmentID: &env.ID}).Error)
	key := apikey.ApiKey{Name: "env-key", KeyHash: "test-only", KeyPrefix: "env", EnvironmentID: &env.ID}
	require.NoError(t, db.Create(&key).Error)
	otherKey := apikey.ApiKey{Name: "other-key", KeyHash: "test-only", KeyPrefix: "oth"}
	require.NoError(t, db.Create(&otherKey).Error)
	require.NoError(t, db.Create(&role.ApiKeyPermission{ApiKeyID: otherKey.ID, EnvironmentID: &env.ID, Permission: "read"}).Error)

	require.NoError(t, environment.NewEnvironmentService(db, nil, nil, nil, nil, nil).DeleteEnvironment(t.Context(), env.ID, nil, nil))
	for _, row := range []struct{ table, column string }{
		{"user_role_assignments", "environment_id"},
		{"api_keys", "environment_id"},
		{"api_key_permissions", "environment_id"},
	} {
		require.Zero(t, rowCount(t, db, row.table, row.column, env.ID), row.table)
	}
	require.EqualValues(t, 1, rowCount(t, db, "api_keys", "id", otherKey.ID))
}

func TestRetainedUniqueKeys(t *testing.T) {
	db := cascadeDatabase(t)
	first := user.User{Username: "unique-user", PasswordHash: "test-only"}
	require.NoError(t, db.Create(&first).Error)
	require.Error(t, db.Create(&user.User{Username: first.Username, PasswordHash: "test-only"}).Error)
	require.NoError(t, db.Create(&session.UserSession{UserID: first.ID, RefreshTokenHash: "test-only", LastUsedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}).Error)
	require.Error(t, db.Create(&session.UserSession{UserID: first.ID, RefreshTokenHash: "test-only", LastUsedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}).Error)
}
