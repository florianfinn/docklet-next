// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Concurrent last-admin deletion regression against the full SQLite baseline.

package schema

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/stretchr/testify/require"
)

func TestConcurrentDeletesPreserveLastGlobalAdmin(t *testing.T) {
	db := cascadeDatabase(t)
	roles := role.NewRoleService(db)
	require.NoError(t, roles.EnsureBuiltInRoles(t.Context()))
	users := user.NewUserService(db, roles, nil)
	for round := range 12 {
		ids := []string{"first", "second"}
		for _, id := range ids {
			_, err := users.CreateUser(t.Context(), &user.User{ID: id, Username: id, PasswordHash: "test-only"})
			require.NoError(t, err)
			require.NoError(t, roles.SetUserAssignments(t.Context(), id, []role.UserRoleAssignment{{RoleID: authz.BuiltInRoleAdmin}}))
		}
		failures := concurrentCalls(t, 2, func(i int) error { return users.DeleteUser(t.Context(), ids[i], nil) })
		success, guarded := 0, 0
		for _, err := range failures {
			if err == nil {
				success++
			} else {
				require.ErrorIs(t, err, user.ErrCannotRemoveLastAdmin, "round %d", round)
				guarded++
			}
		}
		require.Equal(t, 1, success, "round %d", round)
		require.Equal(t, 1, guarded, "round %d", round)
		admins, err := roles.CountGlobalAdminsExcludingUser(t.Context(), "")
		require.NoError(t, err)
		require.Equal(t, 1, admins)
		// Reset the fixture directly; service deletion must refuse its last admin.
		require.NoError(t, db.Exec("DELETE FROM users").Error)
	}
}
