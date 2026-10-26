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
	dto "github.com/florianfinn/docklet-next/types/updater"
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

func TestPolicySaveRightsMatchMode(t *testing.T) {
	for _, mode := range []string{"off", "notify", "apply"} {
		for _, full := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/read-check", true: "/apply-grants"}[full], func(t *testing.T) {
				f := newPolicyFixture(t)
				f.s.roles = role.NewRoleService(f.db)
				require.NoError(t, f.db.Create(&userdomain.User{BaseModel: database.BaseModel{ID: "example-actor"}, Username: "operator"}).Error)
				grants := database.StringSlice{authz.PermProjectsRead, authz.PermImageUpdatesCheck, authz.PermImageUpdatesRead}
				if full {
					grants = append(grants, authz.PermJobsManage, authz.PermProjectsDeploy, authz.PermProjectsUpdate, authz.PermImagesPull, authz.PermActivitiesRead)
				}
				require.NoError(t, f.db.Create(&role.Role{ID: "example-role", Name: "Policy reader", Permissions: grants}).Error)
				require.NoError(t, f.db.Create(&role.UserRoleAssignment{UserID: "example-actor", RoleID: "example-role"}).Error)
				f.p.ports.actor = f.s.imagePolicyActor
				result, err := f.s.SaveImagePolicy(t.Context(), "0", "stack", dto.UpdatePolicyInput{Mode: mode}, "example-actor", "")
				if mode == "apply" && !full {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, mode, result.Mode)
				}
			})
		}
	}
}
