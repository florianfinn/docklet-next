// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original startup recovery and secret-handling tests.

package auth

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignInRecoverySingleUse(t *testing.T) {
	for _, mode := range []string{"none", "local", "proxy"} {
		t.Run(mode, func(t *testing.T) {
			original, tokens := modeTestService(t)
			_, _, err := original.VerifyToken(t.Context(), tokens.AccessToken)
			require.NoError(t, err)
			cfg := *original.config
			cfg.AuthMode = mode
			replacement := "Example-Recovery-Password1!"
			cfg.AuthResetAdminPassword = replacement
			svc := NewAuthService(original.userService, original.settingsService, nil, original.sessionService, &cfg).WithSigningKey(original.signingKey)
			svc.browserSigningKey = original.browserSigningKey
			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
			defer slog.SetDefault(previous)
			require.NoError(t, svc.InitializeSignIn(t.Context()))
			require.Empty(t, cfg.AuthResetAdminPassword)
			admin, err := svc.userService.GlobalAdministrator(t.Context())
			require.NoError(t, err)
			require.NoError(t, svc.userService.ValidatePassword(admin.PasswordHash, replacement))
			require.Error(t, svc.userService.ValidatePassword(admin.PasswordHash, "example-password"))
			require.False(t, admin.RequiresPasswordChange)
			passwordHash := admin.PasswordHash
			require.NoError(t, svc.InitializeSignIn(t.Context()))
			admin, err = svc.userService.GlobalAdministrator(t.Context())
			require.NoError(t, err)
			require.Equal(t, passwordHash, admin.PasswordHash, "recovery must not hash again in this start")
			require.NotContains(t, output.String(), replacement)
			require.Equal(t, 1, strings.Count(output.String(), "Administrator password reset"))
			cfg.AuthMode = "local"
			_, _, err = svc.VerifyToken(t.Context(), tokens.AccessToken)
			require.Error(t, err)
			_, _, err = svc.VerifyBrowserToken(t.Context(), tokens.BrowserToken)
			require.Error(t, err)
			// The cached entry in the previous process is gone on a real restart.
			original.InvalidateAllTokenCache()
			_, _, err = original.VerifyToken(t.Context(), tokens.AccessToken)
			require.Error(t, err)
			_, err = svc.AuthenticateLocalPrimary(t.Context(), "admin", replacement)
			require.NoError(t, err)
		})
	}
}

func TestSignInRecoveryInvalidPasswordFailsStart(t *testing.T) {
	original, _ := modeTestService(t)
	cfg := *original.config
	weak := "weak"
	cfg.AuthResetAdminPassword = weak
	svc := NewAuthService(original.userService, original.settingsService, nil, original.sessionService, &cfg)
	err := svc.InitializeSignIn(t.Context())
	require.Error(t, err)
	require.NotContains(t, err.Error(), weak)
	require.Empty(t, cfg.AuthResetAdminPassword)
	require.Error(t, svc.InitializeSignIn(t.Context()), "a failed initialization stays failed")
}
