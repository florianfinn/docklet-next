// Source: docklet-next/issues/439
// Revision: e683d402f358e87b8eef5b81a71b530fd70cc0fe
// Retrieved: 2026-11-10
// License: Apache-2.0
// Deviations: Original cross-version session and authentication boundary tests.

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestHubUpdateKeepsSignedSessionAcrossVersionChange(t *testing.T) {
	previous := config.Version
	t.Cleanup(func() { config.Version = previous })
	config.Version = "0.90.0"
	original, _, db := modeTestFixture(t)
	key, err := original.settingsService.EnsureEncryptionKey(t.Context())
	require.NoError(t, err)
	crypto.InitEncryption(&crypto.Config{EncryptionKey: key, Environment: "development"})
	// Force issuance through the production persisted signing-key path.
	original.signingKey, original.browserSigningKey = nil, nil
	admin, err := original.userService.GlobalAdministrator(t.Context())
	require.NoError(t, err)
	tokens, err := original.CompleteLogin(t.Context(), admin, authtypes.SessionMeta{}, session.UserSessionSourceLocal)
	require.NoError(t, err)

	config.Version = "0.90.1"
	freshSettings, err := newSettingsServiceForAuthTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	restarted := NewAuthService(original.userService, freshSettings, nil, session.NewSessionService(db), original.config)
	require.NoError(t, restarted.InitializeSignIn(t.Context()))
	for _, credential := range []string{"cookie", "bearer"} {
		t.Run(credential, func(t *testing.T) {
			router := modeTestRouter(t, restarted, "huma", false)
			request := func(value string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodGet, "/api/secure", nil)
				if credential == "cookie" {
					req.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: value})
				} else {
					req.Header.Set("Authorization", "Bearer "+value)
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				return rec
			}
			value := tokens.AccessToken
			if credential == "cookie" {
				value = tokens.BrowserToken
			}
			response := request(value)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Empty(t, response.Header().Values("Set-Cookie"), "update keeps the existing cookie")
			require.Equal(t, http.StatusUnauthorized, request(value+"corrupted").Code)
		})
	}
	require.Equal(t, tokens.ExpiresAt.Unix(), func() int64 {
		claims, err := parseAccessTokenInternal(t.Context(), tokens.AccessToken, original.signingKey.PublicKey())
		require.NoError(t, err)
		return claims.ExpiresAt.Unix()
	}())
	foreign := newTestAuthService()
	foreign.userService, foreign.sessionService = original.userService, original.sessionService
	_, _, err = foreign.VerifyBrowserToken(t.Context(), tokens.BrowserToken)
	require.Error(t, err, "foreign browser signing key is refused")
	_, _, err = foreign.VerifyToken(t.Context(), tokens.AccessToken)
	require.Error(t, err, "foreign bearer signing key is refused")
	_, id, err := restarted.VerifyBrowserToken(t.Context(), tokens.BrowserToken)
	require.NoError(t, err)
	require.NoError(t, restarted.RevokeSession(t.Context(), id))
	_, _, err = restarted.VerifyBrowserToken(t.Context(), tokens.BrowserToken)
	require.Error(t, err, "update must not revive a revoked browser session")
	_, _, err = restarted.VerifyToken(t.Context(), tokens.AccessToken)
	require.Error(t, err, "update must not revive a revoked bearer session")
	browserClaims, err := parseBrowserTokenInternal(t.Context(), tokens.BrowserToken, original.browserSigningKey)
	require.NoError(t, err)
	require.Equal(t, tokens.ExpiresAt.Unix(), browserClaims.ExpiresAt.Unix())
}
