//go:build postgres_integration

// Source: getarcaneapp/arcane/backend/internal/role/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed template catalogue; functional cut: removed central variables and .env.global; functional cut: removed OIDC and passkeys; functional cut: removed SQLite runtime and migrations; functional cut: PostgreSQL integration test fixture.; Service-account fixture removed with the federated account cut.

package role_test

import (
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/testpg"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
)

func TestValidatePermissionsAgainstCallerRejectsEscalation(t *testing.T) {
	_, roleSvc := setupUserAndRoleServices(t)

	caller := authz.NewPermissionSet()
	caller.AddGlobal(authz.PermRolesRead, authz.PermRolesList)

	err := roleSvc.ValidatePermissionsAgainstCaller(caller, []string{
		authz.PermRolesRead,
		authz.PermUsersDelete,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, common.ErrRolePermissionEscalation)

	require.NoError(t, roleSvc.ValidatePermissionsAgainstCaller(caller, []string{authz.PermRolesRead}))
	require.NoError(t, roleSvc.ValidatePermissionsAgainstCaller(authz.SudoPermissionSet(), []string{authz.PermUsersDelete}))
}

func TestValidatePermissionsAgainstCallerRejectsEnvOnlyGrantForGlobalRole(t *testing.T) {
	_, roleSvc := setupUserAndRoleServices(t)

	caller := authz.NewPermissionSet()
	caller.AddEnv("env-1", authz.PermContainersStart)

	err := roleSvc.ValidatePermissionsAgainstCaller(caller, []string{authz.PermContainersStart})
	require.Error(t, err)
	require.ErrorIs(t, err, common.ErrRolePermissionEscalation)
}

func TestValidatePermissionsAgainstCallerRejectsUnknownPermissionBeforeEscalation(t *testing.T) {
	_, roleSvc := setupUserAndRoleServices(t)

	// A sudo caller would otherwise short-circuit past the escalation loop;
	// unknown perms must still surface as UnknownPermissionError (→ 400),
	// not as an opaque escalation 403 or a silent pass.
	err := roleSvc.ValidatePermissionsAgainstCaller(authz.SudoPermissionSet(), []string{"containrs:start"})
	require.Error(t, err)
	require.ErrorIs(t, err, common.ErrUnknownPermission)
	require.NotErrorIs(t, err, common.ErrRolePermissionEscalation)
}

func assignedRoleIDs(t *testing.T, roleSvc *role.RoleService, userID string) []string {
	t.Helper()
	assignments, err := roleSvc.ListUserAssignments(t.Context(), userID)
	require.NoError(t, err)
	ids := make([]string, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.RoleID)
	}
	return ids
}

func TestSetUserAssignmentsRejectsUnknownRole(t *testing.T) {
	ctx := t.Context()
	userSvc, roleSvc := setupUserAndRoleServices(t)
	victim := createTestUser(t, userSvc, "victim", "victim")

	err := roleSvc.SetUserAssignments(ctx, victim.ID, []role.UserRoleAssignment{
		{RoleID: "role_does_not_exist"},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, common.ErrInvalidRoleAssignment)
}

func TestEffectiveGlobalAdminCountIncludesCustomAllPermissionsRole(t *testing.T) {
	ctx := t.Context()
	userSvc, roleSvc := setupUserAndRoleServices(t)
	customAdmin := createTestUser(t, userSvc, "custom-admin", "custom-admin")
	customRole, err := roleSvc.CreateRole(ctx, "Custom Admin", nil, authz.AllPermissions())
	require.NoError(t, err)

	require.NoError(t, roleSvc.SetUserAssignments(ctx, customAdmin.ID, []role.UserRoleAssignment{
		{RoleID: customRole.ID, EnvironmentID: nil},
	}))

	count, err := roleSvc.CountGlobalAdminsExcludingUser(ctx, "")
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NoError(t, roleSvc.AssertGlobalAdminExists(ctx))

	err = roleSvc.SetUserAssignments(ctx, customAdmin.ID, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, common.ErrNoGlobalAdminRemains)
}

func TestEffectiveGlobalAdminCountIgnoresEnvScopedAssignments(t *testing.T) {
	ctx := t.Context()
	userSvc, roleSvc := setupUserAndRoleServices(t)
	customRole, err := roleSvc.CreateRole(ctx, "Custom Admin", nil, authz.AllPermissions())
	require.NoError(t, err)
	envID := "env-1"
	createTestEnvironment(t, userSvc, envID, "http://localhost:3552", nil)

	globalAdmin := createTestUser(t, userSvc, "global-admin", "global-admin")
	envScopedAdmin := createTestUser(t, userSvc, "env-scoped-admin", "env-scoped-admin")

	require.NoError(t, roleSvc.SetUserAssignments(ctx, globalAdmin.ID, []role.UserRoleAssignment{
		{RoleID: customRole.ID, EnvironmentID: nil},
	}))
	require.NoError(t, roleSvc.SetUserAssignments(ctx, envScopedAdmin.ID, []role.UserRoleAssignment{
		{RoleID: customRole.ID, EnvironmentID: &envID},
	}))
	count, err := roleSvc.CountGlobalAdminsExcludingUser(ctx, "")
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func setupAuthServiceTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := gorm.Open(testpg.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&settings.SettingVariable{},
		&user.User{},
		&session.UserSession{},
		&testEnvironmentRow{},
		&role.Role{},
		&role.UserRoleAssignment{},
		&testApiKeyRow{},
		&role.ApiKeyPermission{},
	))
	return &database.DB{DB: db}
}

func setupUserAndRoleServices(t *testing.T) (*database.DB, *role.RoleService) {
	t.Helper()
	db := setupAuthServiceTestDB(t)
	roleService := role.NewRoleService(db)
	require.NoError(t, roleService.EnsureBuiltInRoles(t.Context()))
	return db, roleService
}

func createTestUser(t *testing.T, db *database.DB, id, username string) *user.User {
	t.Helper()
	created := &user.User{ID: id, Username: username}
	require.NoError(t, db.WithContext(t.Context()).Create(created).Error)
	return created
}

func grantGlobalAdmin(t *testing.T, roleService *role.RoleService, userID string) {
	t.Helper()
	require.NoError(t, roleService.SetUserAssignments(t.Context(), userID, []role.UserRoleAssignment{
		{RoleID: authz.BuiltInRoleAdmin},
	}))
}

func createTestEnvironment(t *testing.T, db *database.DB, id, apiURL string, accessToken *string) {
	t.Helper()
	now := time.Now()
	require.NoError(t, db.WithContext(t.Context()).Create(&testEnvironmentRow{
		ID: id, CreatedAt: now, UpdatedAt: &now,
		Name:        "env-" + id,
		ApiUrl:      apiURL,
		Status:      "online",
		Enabled:     true,
		AccessToken: accessToken,
	}).Error)
}

// Minimal stand-ins for the environments and api_keys rows these tests need.
type testEnvironmentRow struct {
	database.BaseModel
	Name        string
	ApiUrl      string `gorm:"column:api_url"`
	Status      string
	Enabled     bool
	AccessToken *string `gorm:"column:access_token"`
}

func (testEnvironmentRow) TableName() string { return "environments" }

type testApiKeyRow struct {
	database.BaseModel
	Name          string
	KeyHash       string `gorm:"column:key_hash"`
	KeyPrefix     string `gorm:"column:key_prefix"`
	Kind          string
	UserID        *string `gorm:"column:user_id"`
	EnvironmentID *string `gorm:"column:environment_id"`
	ManagedBy     *string `gorm:"column:managed_by"`
	ExpiresAt     *time.Time
}

func (testApiKeyRow) TableName() string { return "api_keys" }

func TestResolveExecutionPermissions(t *testing.T) {
	t.Run("personal keys use current user permissions and explicit scope", func(t *testing.T) {
		db, service := setupUserAndRoleServices(t)
		owner := createTestUser(t, db, "owner", "owner")
		grantGlobalAdmin(t, service, owner.ID)
		environmentID := "env-1"
		key := testApiKeyRow{ID: "personal", Kind: "personal", UserID: &owner.ID, EnvironmentID: &environmentID}
		require.NoError(t, db.Create(&key).Error)
		permissions, err := service.ResolveExecutionPermissions(t.Context(), owner.ID, key.ID)
		require.NoError(t, err)
		require.True(t, permissions.Allows(authz.PermJobsManage, environmentID))
		require.False(t, permissions.Allows(authz.PermJobsManage, "env-2"))
		require.False(t, permissions.IsGlobalAdmin())

		require.NoError(t, db.Where("user_id = ?", owner.ID).Delete(&role.UserRoleAssignment{}).Error)
		for _, keyID := range []string{"", key.ID} {
			permissions, err = service.ResolveExecutionPermissions(t.Context(), owner.ID, keyID)
			require.NoError(t, err)
			require.False(t, permissions.Allows(authz.PermJobsManage, environmentID))
		}
	})

	t.Run("scoped keys bypass cached grants", func(t *testing.T) {
		db, service := setupUserAndRoleServices(t)
		owner := createTestUser(t, db, "owner", "owner")
		environmentID := "env-1"
		key := testApiKeyRow{ID: "scoped", Kind: "scoped", UserID: &owner.ID, EnvironmentID: &environmentID}
		require.NoError(t, db.Create(&key).Error)
		require.NoError(t, service.SetApiKeyPermissions(t.Context(), key.ID, []role.ApiKeyPermission{{Permission: authz.PermJobsManage}}))
		cached, err := service.ResolveApiKeyPermissions(t.Context(), key.ID)
		require.NoError(t, err)
		require.True(t, cached.Allows(authz.PermJobsManage, "env-2"))
		permissions, err := service.ResolveExecutionPermissions(t.Context(), owner.ID, key.ID)
		require.NoError(t, err)
		require.True(t, permissions.Allows(authz.PermJobsManage, environmentID))
		require.False(t, permissions.Allows(authz.PermJobsManage, "env-2"))

		require.NoError(t, db.Where("api_key_id = ?", key.ID).Delete(&role.ApiKeyPermission{}).Error)
		permissions, err = service.ResolveExecutionPermissions(t.Context(), owner.ID, key.ID)
		require.NoError(t, err)
		require.False(t, permissions.Allows(authz.PermJobsManage, environmentID))
	})

	t.Run("rejects invalid persisted identities", func(t *testing.T) {
		db, service := setupUserAndRoleServices(t)
		owner := createTestUser(t, db, "owner", "owner")
		otherOwner := "other"
		emptyEnvironment := ""
		expired := time.Now().Add(-time.Minute)
		for _, key := range []testApiKeyRow{
			{ID: "expired", Kind: "personal", UserID: &owner.ID, ExpiresAt: &expired},
			{ID: "other-owner", Kind: "personal", UserID: &otherOwner},
			{ID: "ownerless", Kind: "scoped"},
			{ID: "invalid-kind", Kind: "unknown", UserID: &owner.ID},
			{ID: "invalid-scope", Kind: "personal", UserID: &owner.ID, EnvironmentID: &emptyEnvironment},
		} {
			require.NoError(t, db.Create(&key).Error)
			_, err := service.ResolveExecutionPermissions(t.Context(), owner.ID, key.ID)
			require.Error(t, err, key.ID)
		}
		_, err := service.ResolveExecutionPermissions(t.Context(), owner.ID, "deleted-key")
		require.Error(t, err)
		require.NoError(t, db.Delete(owner).Error)
		_, err = service.ResolveExecutionPermissions(t.Context(), owner.ID, "")
		require.Error(t, err)
	})
}
