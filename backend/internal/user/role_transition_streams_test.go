// Source: docklet-next/docs/design/resource-rights.md
// Revision: 23bd198e2cbef8b19488c34b3b0b22343b41c0d1
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original role-driven administrator transition revocation (S1 verify finding S1V-1).

package user_test

import (
	"context"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

// Role mutations that change actual administrator status close streams and bump the
// rights revision; a mutation that keeps the account an administrator does not.
func TestRoleAdministratorTransitionsRevokeStreams(t *testing.T) {
	for _, action := range []string{"assignment-demote", "assignment-promote", "update-role-demote", "delete-role-demote", "update-role-keeps-admin"} {
		t.Run(action, func(t *testing.T) {
			svc, roles := setupUserAndRoleServices(t)
			operator := createTestUser(t, svc, "operator", "operator")
			grantGlobalAdmin(t, roles, operator.ID)
			target := createTestUser(t, svc, "target", "target")
			var custom *role.Role
			var err error
			switch action {
			case "assignment-demote":
				grantGlobalAdmin(t, roles, target.ID)
			case "update-role-demote", "delete-role-demote", "update-role-keeps-admin":
				custom, err = roles.CreateRole(t.Context(), "custom", nil, authz.AllPermissions())
				require.NoError(t, err)
				require.NoError(t, roles.SetUserAssignments(t.Context(), target.ID, []role.UserRoleAssignment{{RoleID: custom.ID}}))
			}
			before, err := roles.ResolvePermissions(t.Context(), target.ID)
			require.NoError(t, err)
			require.Equal(t, action != "assignment-promote", before.IsGlobalAdmin())
			revision := access.Revision(target.ID)
			closed := openLease(t, "target-session", target.ID)
			actorCtx := context.WithValue(t.Context(), userDTO.CurrentUserContextKey{}, operator)
			switch action {
			case "assignment-demote":
				err = roles.SetUserAssignments(actorCtx, target.ID, nil)
			case "assignment-promote":
				err = roles.SetUserAssignments(actorCtx, target.ID, []role.UserRoleAssignment{{RoleID: authz.BuiltInRoleAdmin}})
			case "update-role-demote":
				_, err = roles.UpdateRole(actorCtx, custom.ID, "custom", nil, []string{})
			case "update-role-keeps-admin":
				_, err = roles.UpdateRole(actorCtx, custom.ID, "renamed", nil, authz.AllPermissions())
			case "delete-role-demote":
				err = roles.DeleteRole(actorCtx, custom.ID)
			}
			require.NoError(t, err)
			after, err := roles.ResolvePermissions(t.Context(), target.ID)
			require.NoError(t, err)
			changed := action != "update-role-keeps-admin"
			require.Equal(t, action == "assignment-promote" || !changed, after.IsGlobalAdmin())
			require.Equal(t, changed, *closed, "only an administrator status change closes existing streams")
			if changed {
				require.Greater(t, access.Revision(target.ID), revision, "a status change must invalidate in-flight upgrades")
			} else {
				require.Equal(t, revision, access.Revision(target.ID))
			}
		})
	}
}
