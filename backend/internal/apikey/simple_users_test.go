// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package apikey

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	dto "github.com/florianfinn/docklet-next/types/apikey"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPersonalKeyCurrentRightsAndScopedAdministratorOwner(t *testing.T) {
	svc, _, users := setupAPIKeyService(t)
	admin, err := users.CreateUser(t.Context(), &user.User{Username: "operator"})
	require.NoError(t, err)
	require.NoError(t, users.SetAdministrator(t.Context(), "fixture", admin.ID, true))
	u, err := users.CreateUser(t.Context(), &user.User{Username: "viewer"})
	require.NoError(t, err)
	personal, err := svc.CreatePersonalApiKey(t.Context(), u.ID, dto.CreateUserApiKey{Name: "personal"})
	require.NoError(t, err)
	_, _, err = svc.ValidateApiKeyWithID(t.Context(), personal.Key)
	require.NoError(t, err)
	require.NoError(t, users.SetAppAssignments(t.Context(), u.ID, []userDTO.AppAssignment{{EnvironmentID: "0", ProjectID: "uuid", Level: "full"}}))
	ps, err := svc.roleService.ResolvePermissions(t.Context(), u.ID)
	require.NoError(t, err)
	require.True(t, ps.Allows(authz.PermImagesPull, "0"))
	require.False(t, ps.Allows(authz.PermContainersCreate, "0"))
	require.NoError(t, users.SetAdministrator(t.Context(), admin.ID, u.ID, true))
	scoped, err := svc.CreateApiKey(t.Context(), u.ID, authz.SudoPermissionSet(), dto.CreateApiKey{Name: "scoped", Permissions: []dto.PermissionGrant{{Permission: authz.PermContainersRead}}})
	require.NoError(t, err)
	_, _, err = svc.ValidateApiKeyWithID(t.Context(), scoped.Key)
	require.NoError(t, err)
	require.NoError(t, users.SetAdministrator(t.Context(), admin.ID, u.ID, false))
	_, _, err = svc.ValidateApiKeyWithID(t.Context(), scoped.Key)
	require.ErrorIs(t, err, ErrApiKeyInvalid)
	_, _, err = svc.ValidateApiKeyWithID(t.Context(), personal.Key)
	require.NoError(t, err)
	require.NoError(t, users.SetDisabled(t.Context(), u.ID, true))
	_, _, err = svc.ValidateApiKeyWithID(t.Context(), personal.Key)
	require.ErrorIs(t, err, ErrApiKeyInvalid)
	require.NoError(t, users.SetDisabled(t.Context(), u.ID, false))
	closed := false
	ctx := sessionlife.WithSession(t.Context(), "", u.ID, func(context.Context) error { return nil })
	lease, err := sessionlife.Register(ctx, func() { closed = true })
	require.NoError(t, err)
	defer lease.Release()
	require.NoError(t, svc.DeleteApiKey(t.Context(), personal.ID))
	require.True(t, closed)
	_, _, err = svc.ValidateApiKeyWithID(t.Context(), personal.Key)
	require.Error(t, err)
	require.NoError(t, users.DeleteUser(t.Context(), u.ID, nil))
	_, _, err = svc.ValidateApiKeyWithID(t.Context(), scoped.Key)
	require.Error(t, err)
}

func TestStaticAdministratorKeyKeepsRuntimeAuthority(t *testing.T) {
	for _, change := range []string{"demote", "disable"} {
		t.Run(change, func(t *testing.T) {
			svc, db, users := setupAPIKeyService(t)
			owner := createTestAPIKeyUser(t, t.Context(), users, "owner", defaultAdminUsername)
			operator := createTestAPIKeyUser(t, t.Context(), users, "operator")
			raw := "arc_staticruntimeprovidedkey1234567890"
			require.NoError(t, svc.ReconcileDefaultAdminAPIKey(t.Context(), raw))
			require.NoError(t, svc.BackfillApiKeyPermissions(t.Context()))
			var key ApiKey
			require.NoError(t, db.Where("managed_by = ?", managedByAdminBootstrap).First(&key).Error)
			if change == "demote" {
				require.NoError(t, users.SetAdministrator(t.Context(), operator.ID, owner.ID, false))
			} else {
				require.NoError(t, users.SetDisabled(t.Context(), owner.ID, true))
			}
			_, _, err := svc.ValidateApiKeyWithID(t.Context(), raw)
			require.NoError(t, err)
			ps, err := svc.roleService.ResolveExecutionPermissions(t.Context(), owner.ID, key.ID)
			require.NoError(t, err)
			require.True(t, ps.Allows(authz.PermContainersStart, "0"))
		})
	}
}

// Deleting a key closes its owner's streams only for users; administrators keep theirs.
func TestKeyDeleteRevokesOnlyUserStreams(t *testing.T) {
	svc, _, users := setupAPIKeyService(t)
	admin, err := users.CreateUser(t.Context(), &user.User{Username: "operator"})
	require.NoError(t, err)
	require.NoError(t, users.SetAdministrator(t.Context(), "fixture", admin.ID, true))
	viewer, err := users.CreateUser(t.Context(), &user.User{Username: "viewer"})
	require.NoError(t, err)
	for _, owner := range []struct {
		id         string
		wantClosed bool
	}{{admin.ID, false}, {viewer.ID, true}} {
		key, err := svc.CreatePersonalApiKey(t.Context(), owner.id, dto.CreateUserApiKey{Name: "personal"})
		require.NoError(t, err)
		closed := false
		ctx := sessionlife.WithSession(t.Context(), "session-"+owner.id, owner.id, func(context.Context) error { return nil })
		lease, err := sessionlife.Register(ctx, func() { closed = true })
		require.NoError(t, err)
		require.NoError(t, svc.DeleteApiKey(t.Context(), key.ID))
		require.Equal(t, owner.wantClosed, closed, owner.id)
		lease.Release()
	}
}
