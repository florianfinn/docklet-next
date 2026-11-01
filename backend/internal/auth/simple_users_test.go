// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package auth

import (
	"github.com/florianfinn/docklet-next/backend/internal/role"
	domain "github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/florianfinn/docklet-next/types/auth"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxySimpleUserExactMapping(t *testing.T) {
	s, _, _ := modeTestFixture(t)
	u, err := s.userService.CreateUser(t.Context(), &domain.User{Username: "viewer@example.com"})
	require.NoError(t, err)
	s.config.AuthMode = "proxy"
	s.config.AuthProxyAdminUsers = new("Operator")
	request := func(name string) (*domain.User, error) {
		req := httptest.NewRequest("GET", "https://hub.example.com/api/auth/me", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		req.Header.Set("Remote-User", name)
		return s.ModeRequestUser(t.Context(), req)
	}
	got, err := request("viewer@example.com")
	require.NoError(t, err)
	require.Equal(t, u.ID, got.ID)
	for _, name := range []string{"Viewer@example.com", "operator", "unknown"} {
		_, err = request(name)
		require.EqualError(t, err, "access.not-registered")
	}
	got, err = request("Operator")
	require.NoError(t, err)
	require.NotEqual(t, u.ID, got.ID)
	require.NoError(t, s.userService.SetDisabled(t.Context(), u.ID, true))
	_, err = request("viewer@example.com")
	require.EqualError(t, err, "access.disabled")
	for _, list := range []*string{nil, new(""), new("  ")} {
		s.config.AuthProxyAdminUsers = list
		got, err = request("anything")
		require.NoError(t, err, "an unset or blank list keeps every identity administrative")
		require.NotEqual(t, u.ID, got.ID)
	}
}
func TestDisabledUserRejectsCachedBrowserAndPassword(t *testing.T) {
	s, _, _ := modeTestFixture(t)
	hash, err := s.userService.HashPassword("example-password")
	require.NoError(t, err)
	u, err := s.userService.CreateUser(t.Context(), &domain.User{Username: "viewer", PasswordHash: hash})
	require.NoError(t, err)
	tokens, err := s.CompleteLogin(t.Context(), u, auth.SessionMeta{}, "local")
	require.NoError(t, err)
	_, _, err = s.VerifyBrowserToken(t.Context(), tokens.BrowserToken)
	require.NoError(t, err)
	require.NoError(t, s.userService.SetDisabled(t.Context(), u.ID, true))
	_, _, err = s.VerifyBrowserToken(t.Context(), tokens.BrowserToken)
	require.Error(t, err)
	_, err = s.AuthenticateLocalPrimary(t.Context(), u.Username, "example-password")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}
func TestSessionlessUserLeaseAndLegacyRoleIgnored(t *testing.T) {
	s, _, db := modeTestFixture(t)
	u, err := s.userService.CreateUser(t.Context(), &domain.User{Username: "viewer"})
	require.NoError(t, err)
	roles := role.NewRoleService(db)
	require.NoError(t, roles.SetUserAssignments(t.Context(), u.ID, []role.UserRoleAssignment{{RoleID: authz.BuiltInRoleEditor}}))
	ps, err := s.userService.ResolveUserPermissions(t.Context(), u.ID)
	require.NoError(t, err)
	require.False(t, ps.Allows(authz.PermContainersCreate, "0"))
	for _, mode := range []string{"local", "proxy"} {
		s.config.AuthMode = mode
		closed := false
		ctx := s.ViewerContext(t.Context(), "", u.ID)
		lease, err := sessionlife.Register(ctx, func() { closed = true })
		require.NoError(t, err)
		require.NoError(t, s.userService.SetDisabled(t.Context(), u.ID, true))
		require.True(t, closed)
		require.Error(t, sessionlife.Check(ctx))
		lease.Release()
		require.NoError(t, s.userService.SetDisabled(t.Context(), u.ID, false))
	}
}

func TestLogoutURLOnlyExposedInProxyMode(t *testing.T) {
	s, _, _ := modeTestFixture(t)
	s.config.AuthProxyLogoutURL = "https://proxy.example.com/logout"
	h := &AuthHandler{authService: s}
	for _, mode := range []string{"local", "none", "proxy"} {
		s.config.AuthMode = mode
		out, err := h.GetSignInMode(t.Context(), nil)
		require.NoError(t, err)
		if mode == "proxy" {
			require.Equal(t, s.config.AuthProxyLogoutURL, *out.Body.Data.LogoutURL)
		} else {
			require.Nil(t, out.Body.Data.LogoutURL)
		}
	}
	s.config.AuthMode = "proxy"
	out, err := h.Logout(t.Context(), nil)
	require.NoError(t, err)
	require.Contains(t, strings.Join(out.SetCookie, ";"), "docklet_csrf=")
	require.Contains(t, strings.Join(out.SetCookie, ";"), "Max-Age=0")
}

func TestLateUserUpgradeKeepsAuthenticationRevision(t *testing.T) {
	s, _, _ := modeTestFixture(t)
	u, err := s.userService.CreateUser(t.Context(), &domain.User{Username: "late-viewer"})
	require.NoError(t, err)
	ps := s.modePermissions(t.Context(), u)
	require.NoError(t, s.userService.SetDisabled(t.Context(), u.ID, true))
	require.NoError(t, s.userService.SetDisabled(t.Context(), u.ID, false))
	ctx := s.ViewerContextForPermissions(t.Context(), "", u.ID, ps)
	lease, err := sessionlife.Register(ctx, func() {})
	require.Error(t, err)
	if lease != nil {
		lease.Release()
	}
}
