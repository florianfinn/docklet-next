// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Actual role/key rows are re-read for policy execution instead of cached or system grants.

package updater

import (
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	userdomain "github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPolicyActorResolvesCurrentKeyAndCompleteGrants(t *testing.T) {
	f := newPolicyFixture(t)
	f.s.roles = role.NewRoleService(f.db)
	require.NoError(t, f.db.Create(&userdomain.User{BaseModel: database.BaseModel{ID: "example-actor"}, Username: "operator"}).Error)
	grants := database.StringSlice{authz.PermJobsManage, authz.PermProjectsRead, authz.PermImageUpdatesCheck, authz.PermImageUpdatesRead, authz.PermProjectsDeploy, authz.PermProjectsUpdate, authz.PermImagesPull, authz.PermActivitiesRead}
	require.NoError(t, f.db.Create(&role.Role{ID: "example-role", Name: "Policy operator", Permissions: grants}).Error)
	assignment := role.UserRoleAssignment{UserID: "example-actor", RoleID: "example-role"}
	require.NoError(t, f.db.Create(&assignment).Error)
	owner := "example-actor"
	key := apikey.ApiKey{BaseModel: database.BaseModel{ID: "example-key"}, Name: "Policy key", KeyHash: "synthetic-hash", KeyPrefix: "synthetic", Kind: "personal", UserID: &owner}
	require.NoError(t, f.db.Create(&key).Error)
	ctx, err := f.s.imagePolicyActor(t.Context(), "0", owner, key.ID, true)
	require.NoError(t, err)
	actor, ok := userctx.CurrentUserFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, owner, actor.ID)
	require.NotEqual(t, user.SystemUser.ID, actor.ID)
	permissions, ok := middleware.PermissionsFromContext(ctx)
	require.True(t, ok)
	require.True(t, permissions.Allows(authz.PermProjectsUpdate, "0"))
	require.NoError(t, f.db.Model(&apikey.ApiKey{}).Where("id = ?", key.ID).Update("expires_at", time.Now().Add(-time.Hour)).Error)
	_, err = f.s.imagePolicyActor(t.Context(), "0", owner, key.ID, true)
	require.Error(t, err)
	require.NoError(t, f.db.Model(&apikey.ApiKey{}).Where("id = ?", key.ID).Update("expires_at", nil).Error)
	require.NoError(t, f.db.Delete(&assignment).Error)
	_, err = f.s.imagePolicyActor(t.Context(), "0", owner, key.ID, true)
	require.Error(t, err)
	require.Zero(t, f.admissions)
}
