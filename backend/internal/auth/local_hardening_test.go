// Source: docklet-next/issues/669
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original case-folding and dummy password comparison regression.

package auth

import (
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestLocalIdentityHardening(t *testing.T) {
	db := setupAuthServiceTestDB(t)
	users := user.NewUserService(db, nil, session.RevokeAllUserSessionsExceptInDB)
	settings, err := newSettingsServiceForAuthTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	s := newTestAuthService()
	s.userService = users
	s.settingsService = settings
	hash, err := users.HashPassword("example-password")
	require.NoError(t, err)
	require.Equal(t, strings.Join(strings.Split(hash, "$")[:4], "$"), strings.Join(strings.Split(dummyLocalPasswordHash(), "$")[:4], "$"))
	now := time.Now()
	for _, u := range []user.User{
		{ID: "case", Username: "case", Email: new("Admin@example.com"), PasswordHash: hash},
		{ID: "disabled", Username: "disabled", PasswordHash: hash, DisabledAt: &now},
		{ID: "dup1", Username: "dup1", Email: new("Duplicate@example.com"), PasswordHash: hash},
		{ID: "dup2", Username: "dup2", Email: new("duplicate@example.com"), PasswordHash: hash},
	} {
		require.NoError(t, db.Create(&u).Error)
	}
	got, err := s.AuthenticateLocalPrimary(t.Context(), "admin@EXAMPLE.com", "example-password")
	require.NoError(t, err)
	require.Equal(t, "case", got.ID)
	_, err = s.AuthenticateLocalPrimary(t.Context(), "DUPLICATE@example.com", "example-password")
	require.ErrorIs(t, err, ErrInvalidCredentials)
	original := validateLocalPassword
	t.Cleanup(func() { validateLocalPassword = original })
	count := 0
	validateLocalPassword = func(s *user.UserService, h, p string) error {
		count++
		require.Equal(t, dummyLocalPasswordHash(), h)
		return original(s, h, p)
	}
	for _, name := range []string{"missing", "missing@example.com", "disabled"} {
		before := count
		_, err = s.AuthenticateLocalPrimary(t.Context(), name, "example-password")
		require.ErrorIs(t, err, ErrInvalidCredentials)
		require.Equal(t, before+1, count, name)
	}
}
