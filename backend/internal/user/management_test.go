// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package user_test

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	domain "github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	dto "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLastEnabledAdministratorAndStaleUserWrite(t *testing.T) {
	svc, roles := setupUserAndRoleServices(t)
	admin := createTestUser(t, svc, "first", "first")
	grantGlobalAdmin(t, roles, admin.ID)
	require.ErrorIs(t, svc.SetDisabled(t.Context(), admin.ID, true), domain.ErrCannotRemoveLastAdmin)
	require.ErrorIs(t, svc.SetAdministrator(t.Context(), "operator", admin.ID, false), domain.ErrCannotRemoveLastAdmin)
	require.Error(t, svc.SetAdministrator(t.Context(), admin.ID, admin.ID, true))
	second := createTestUser(t, svc, "second", "second")
	require.NoError(t, svc.SetAdministrator(t.Context(), admin.ID, second.ID, true))
	require.NoError(t, svc.SetDisabled(t.Context(), second.ID, true))
	require.ErrorIs(t, svc.SetDisabled(t.Context(), admin.ID, true), domain.ErrCannotRemoveLastAdmin)
	old := *second
	old.DisabledAt = nil
	old.DisplayName = new("Changed")
	_, err := svc.UpdateUser(t.Context(), &old, nil)
	require.NoError(t, err)
	stored, err := svc.GetUserByID(t.Context(), second.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.DisabledAt)
	require.NoError(t, svc.SetDisabled(t.Context(), second.ID, false))
	stored, err = svc.GetUserByID(t.Context(), second.ID)
	require.NoError(t, err)
	require.Nil(t, stored.DisabledAt)
}
func TestEveryRightsMutationRevokesUserLease(t *testing.T) {
	svc, roles := setupUserAndRoleServices(t)
	admin := createTestUser(t, svc, "admin", "admin")
	grantGlobalAdmin(t, roles, admin.ID)
	u := createTestUser(t, svc, "viewer", "viewer")
	actions := []func() error{
		func() error {
			return svc.SetAppAssignments(t.Context(), u.ID, []dto.AppAssignment{{EnvironmentID: "0", ProjectID: "project", Level: "full"}})
		},
		func() error { return svc.SetDisabled(t.Context(), u.ID, true) }, func() error { return svc.SetDisabled(t.Context(), u.ID, false) },
		func() error { return svc.ResetInitialPassword(t.Context(), u.ID, "new-example-password") },
		func() error { return svc.SetAdministrator(t.Context(), admin.ID, u.ID, true) },
		func() error { return svc.SetAdministrator(t.Context(), admin.ID, u.ID, false) },
		func() error { return svc.DeleteUser(t.Context(), u.ID, nil) },
	}
	for i, action := range actions {
		rev := access.Revision(u.ID)
		closed := false
		ctx := sessionlife.WithSession(t.Context(), "", u.ID, func(context.Context) error { return nil })
		lease, err := sessionlife.Register(ctx, func() { closed = true })
		require.NoError(t, err)
		require.NoError(t, action(), i)
		require.True(t, closed, i)
		require.Greater(t, access.Revision(u.ID), rev, i)
		lease.Release()
	}
}
