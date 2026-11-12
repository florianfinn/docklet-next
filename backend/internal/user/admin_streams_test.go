// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package user_test

import (
	"context"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/stretchr/testify/require"
)

func openLease(t *testing.T, sessionID, userID string) *bool {
	t.Helper()
	closed := false
	ctx := sessionlife.WithSession(t.Context(), sessionID, userID, func(context.Context) error { return nil })
	lease, err := sessionlife.Register(ctx, func() { closed = true })
	require.NoError(t, err)
	t.Cleanup(lease.Release)
	return &closed
}

// Administrator streams are unaffected by user revocation (resource-rights §7).
func TestAdministratorAccountWritesKeepAdministratorStreams(t *testing.T) {
	svc, roles := setupUserAndRoleServices(t)
	admin := createTestUser(t, svc, "admin", "admin")
	grantGlobalAdmin(t, roles, admin.ID)
	closed := openLease(t, "admin-session", admin.ID)

	admin.PasswordHash = "test-only"
	_, err := svc.UpdateUser(t.Context(), admin, nil)
	require.NoError(t, err)
	require.False(t, *closed, "updateUser closed the administrator's streams")

	roles.InvalidateUser(admin.ID)
	require.False(t, *closed, "a role cache purge closed the administrator's streams")
}

// Ordinary account writes (last login, profile) do not close a user's own streams.
func TestUserOrdinaryWriteKeepsUserStreams(t *testing.T) {
	svc, _ := setupUserAndRoleServices(t)
	u := createTestUser(t, svc, "viewer", "viewer")
	closed := openLease(t, "", u.ID)
	locale := "de-DE"
	u.Locale = &locale
	_, err := svc.UpdateUser(t.Context(), u, nil)
	require.NoError(t, err)
	require.False(t, *closed)
}
