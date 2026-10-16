// Source: getarcaneapp/arcane/backend/cli/admin/reset_password_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; SQLite temp-file persistence fixture.

package admin

import (
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/types/auth"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/validation"
)

func newResetPasswordTestDBInternal(t *testing.T) *database.DB {
	t.Helper()

	gormDB, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(
		&user.User{},
		&session.UserSession{},
		&role.Role{},
		&role.UserRoleAssignment{},
	))

	db := &database.DB{DB: gormDB}
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func createGlobalAdminInternal(t *testing.T, db *database.DB) (*user.User, *user.UserService) {
	t.Helper()
	ctx := t.Context()
	roleService := role.NewRoleService(db)
	require.NoError(t, roleService.EnsureBuiltInRoles(ctx))
	userService := user.NewUserService(db, roleService, session.RevokeAllUserSessionsExceptInDB)
	passwordHash, err := userService.HashPassword("old-password")
	require.NoError(t, err)
	adminUser, err := userService.CreateUser(ctx, &user.User{
		ID:                     "admin-1",
		Username:               "arcane",
		PasswordHash:           passwordHash,
		RequiresPasswordChange: true,
	})
	require.NoError(t, err)
	require.NoError(t, roleService.SetUserAssignments(ctx, adminUser.ID, []role.UserRoleAssignment{
		{RoleID: authz.BuiltInRoleAdmin},
	}))

	return adminUser, userService
}

func TestResetPasswordInternalResetsPasswordAndRevokesSessions(t *testing.T) {
	db := newResetPasswordTestDBInternal(t)
	ctx := t.Context()
	adminUser, userService := createGlobalAdminInternal(t, db)
	sessionService := session.NewSessionService(db)
	firstSession, _, err := sessionService.CreateSession(ctx, adminUser.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
	require.NoError(t, err)
	secondSession, _, err := sessionService.CreateSession(ctx, adminUser.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
	require.NoError(t, err)

	require.NoError(t, resetPasswordInternal(ctx, db, "arcane", "new-password", validation.PasswordPolicyBasic))

	updatedUser, err := userService.GetUserByID(ctx, adminUser.ID)
	require.NoError(t, err)
	require.NoError(t, userService.ValidatePassword(updatedUser.PasswordHash, "new-password"))
	require.Error(t, userService.ValidatePassword(updatedUser.PasswordHash, "old-password"))
	require.False(t, updatedUser.RequiresPasswordChange)

	revokedFirst, err := sessionService.GetSessionByID(ctx, firstSession.ID)
	require.NoError(t, err)
	require.NotNil(t, revokedFirst.RevokedAt)
	revokedSecond, err := sessionService.GetSessionByID(ctx, secondSession.ID)
	require.NoError(t, err)
	require.NotNil(t, revokedSecond.RevokedAt)
}

func TestResetPasswordInternalRollsBackWhenSessionRevocationFails(t *testing.T) {
	db := newResetPasswordTestDBInternal(t)
	ctx := t.Context()
	adminUser, userService := createGlobalAdminInternal(t, db)
	sessionService := session.NewSessionService(db)
	localSession, _, err := sessionService.CreateSession(ctx, adminUser.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_user_session_revocation BEFORE UPDATE OF revoked_at ON user_sessions FOR EACH ROW WHEN NEW.revoked_at IS NOT NULL BEGIN SELECT RAISE(ABORT,'forced session revocation failure'); END;`).Error)

	err = resetPasswordInternal(ctx, db, adminUser.Username, "new-password", validation.PasswordPolicyBasic)
	require.Error(t, err)
	require.ErrorContains(t, err, "forced session revocation failure")

	unchangedUser, err := userService.GetUserByID(ctx, adminUser.ID)
	require.NoError(t, err)
	require.NoError(t, userService.ValidatePassword(unchangedUser.PasswordHash, "old-password"))
	require.True(t, unchangedUser.RequiresPasswordChange)
	unchangedSession, err := sessionService.GetSessionByID(ctx, localSession.ID)
	require.NoError(t, err)
	require.Nil(t, unchangedSession.RevokedAt)
}

func TestResetPasswordInternalRejectsNonAdmin(t *testing.T) {
	db := newResetPasswordTestDBInternal(t)
	ctx := t.Context()
	_, userService := createGlobalAdminInternal(t, db)
	passwordHash, err := userService.HashPassword("old-password")
	require.NoError(t, err)
	nonAdmin, err := userService.CreateUser(ctx, &user.User{
		ID:                     "user-1",
		Username:               "operator",
		PasswordHash:           passwordHash,
		RequiresPasswordChange: true,
	})
	require.NoError(t, err)

	err = resetPasswordInternal(ctx, db, nonAdmin.Username, "new-password", validation.PasswordPolicyBasic)
	require.Error(t, err)
	require.ErrorContains(t, err, "effective global administrator")

	unchangedUser, err := userService.GetUserByID(ctx, nonAdmin.ID)
	require.NoError(t, err)
	require.NoError(t, userService.ValidatePassword(unchangedUser.PasswordHash, "old-password"))
	require.True(t, unchangedUser.RequiresPasswordChange)
}
