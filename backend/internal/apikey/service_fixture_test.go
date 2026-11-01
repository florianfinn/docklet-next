// Source: getarcaneapp/arcane/backend/internal/apikey/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed template catalogue; functional cut: removed OIDC and passkeys; SQLite temp-file persistence fixture; M15 simple-user rights and identity contracts.

package apikey

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"strings"
	"testing"
)

func setupAPIKeyServiceTestDB(t *testing.T) *database.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(testsqlite.Dialector(t, dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&settings.SettingVariable{},
		&user.User{},
		&session.UserSession{},
		&testEnvironmentRow{},
		&access.Assignment{},
		&role.Role{},
		&role.UserRoleAssignment{},
		&ApiKey{},
		&role.ApiKeyPermission{},
	))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	return &database.DB{DB: db}
}
func setupAPIKeyService(t *testing.T) (*ApiKeyService, *database.DB, *user.UserService) {
	t.Helper()

	db := setupAPIKeyServiceTestDB(t)
	roles := role.NewRoleService(db)
	require.NoError(t, roles.EnsureBuiltInRoles(t.Context()))
	userService := user.NewUserService(db, roles, session.RevokeAllUserSessionsExceptInDB)
	return NewApiKeyService(db, userService, roles), db, userService
}

func createTestAPIKeyUser(t *testing.T, ctx context.Context, userService *user.UserService, id string, usernames ...string) *user.User {
	t.Helper()
	username := "user-" + id
	if len(usernames) > 0 {
		username = usernames[0]
	}

	localUser := &user.User{
		ID:       id,
		Username: username,
	}

	created, err := userService.CreateUser(ctx, localUser)
	require.NoError(t, err)
	require.NoError(t, userService.SetAdministrator(ctx, "fixture-operator", created.ID, true))
	return created
}
