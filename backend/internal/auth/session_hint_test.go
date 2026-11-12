// Source: docklet-next/issues/425
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original router tests for session presence cookie lifetime and refusal pairs.

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func assertSessionHintPair(t *testing.T, response *http.Response, secure, clear bool) *http.Cookie {
	t.Helper()
	tokenName, hintName := cookie.InsecureTokenCookieName, "docklet_session"
	if secure {
		tokenName, hintName = cookie.TokenCookieName, "__Host-docklet_session"
	}
	cookies := map[string]*http.Cookie{}
	for _, c := range response.Cookies() {
		cookies[c.Name] = c
	}
	token, hint := cookies[tokenName], cookies[hintName]
	require.NotNil(t, token, "token cookie")
	require.NotNil(t, hint, "session presence hint")
	require.True(t, token.HttpOnly)
	require.False(t, hint.HttpOnly)
	require.Equal(t, token.Path, hint.Path)
	require.Equal(t, token.Domain, hint.Domain)
	require.Equal(t, token.MaxAge, hint.MaxAge)
	require.Equal(t, token.Expires, hint.Expires)
	require.Equal(t, token.Secure, hint.Secure)
	require.Equal(t, token.SameSite, hint.SameSite)
	require.Equal(t, secure, hint.Secure)
	if clear {
		require.Empty(t, hint.Value)
		require.Equal(t, -1, hint.MaxAge)
	} else {
		require.Equal(t, "1", hint.Value)
		require.Positive(t, hint.MaxAge)
	}
	if secure && clear {
		assertSessionHintPair(t, response, false, true)
	}
	return token
}

func TestSessionHintLoginRenewalLogoutThroughRouter(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "trusted-https"}[secure], func(t *testing.T) {
			svc, _ := modeTestService(t)
			router := echo.New()
			api := humaecho.New(router, huma.DefaultConfig("test", "1.0.0"))
			api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				next(huma.WithContext(ctx, context.WithValue(ctx.Context(), cookie.SecureCookieContextKey{}, secure)))
			})
			api.UseMiddleware(NewHumaMiddleware(api, svc, nil, svc.userServiceRoleResolver(), nil, svc.config))
			RegisterAuth(api, svc.userService, svc, svc.settingsService)
			send := func(path, body string, token *http.Cookie) *httptest.ResponseRecorder {
				req := httptest.NewRequest("POST", path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.RemoteAddr = "192.0.2.10:5000"
				if token != nil {
					req.AddCookie(token)
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				require.Equal(t, 200, rec.Code, rec.Body.String())
				return rec
			}
			phrase := "example-password"
			payload, err := json.Marshal(map[string]string{"username": "admin", "password": phrase})
			require.NoError(t, err)
			login := send("/auth/login", string(payload), nil)
			token := assertSessionHintPair(t, login.Result(), secure, false)
			var body struct {
				Data struct {
					RefreshToken string `json:"refreshToken"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(login.Body.Bytes(), &body))
			require.NotEmpty(t, body.Data.RefreshToken)
			renewPayload, err := json.Marshal(map[string]string{"refreshToken": body.Data.RefreshToken})
			require.NoError(t, err)
			refresh := send("/auth/refresh", string(renewPayload), token)
			renewed := assertSessionHintPair(t, refresh.Result(), secure, false)
			require.NotEqual(t, token.Value, renewed.Value)
			// Repeated login for the same principal uses the same paired issuance path.
			again := send("/auth/login", string(payload), renewed)
			token = assertSessionHintPair(t, again.Result(), secure, false)
			logout := send("/auth/logout", `{}`, token)
			assertSessionHintPair(t, logout.Result(), secure, true)
		})
	}
}

func TestSessionHintRefusalClearsThroughRouters(t *testing.T) {
	for _, surface := range []string{"huma", "echo"} {
		for _, secure := range []bool{false, true} {
			for _, failure := range []string{"revoked", "expired"} {
				t.Run(surface+"/"+map[bool]string{false: "http", true: "https"}[secure]+"/"+failure, func(t *testing.T) {
					svc, tokens, db := modeTestFixture(t)
					actor, sessionID, err := svc.VerifyBrowserToken(t.Context(), tokens.BrowserToken)
					require.NoError(t, err)
					switch failure {
					case "revoked":
						require.NoError(t, svc.RevokeSession(t.Context(), sessionID))
					case "expired":
						require.NoError(t, db.Model(&session.UserSession{}).Where("id=?", sessionID).Update("expires_at", time.Now().Add(-time.Hour)).Error)
					}
					svc.InvalidateUserTokenCache(actor.ID)
					router := modeTestRouter(t, svc, surface, false)
					req := httptest.NewRequest("GET", "/api/secure", http.NoBody)
					req = req.WithContext(context.WithValue(req.Context(), cookie.SecureCookieContextKey{}, secure))
					name := cookie.InsecureTokenCookieName
					if secure {
						name = cookie.TokenCookieName
					}
					req.AddCookie(&http.Cookie{Name: name, Value: tokens.BrowserToken})
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					require.Equal(t, 401, rec.Code, rec.Body.String())
					assertSessionHintPair(t, rec.Result(), secure, true)
				})
			}
		}
	}
}

func TestSessionHintNeverAuthorizesThroughRouter(t *testing.T) {
	svc, _ := modeTestService(t)
	router := modeTestRouter(t, svc, "huma", false)
	req := httptest.NewRequest("GET", "/api/secure", http.NoBody)
	req.AddCookie(&http.Cookie{Name: "docklet_session", Value: "1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, 401, rec.Code)
}
