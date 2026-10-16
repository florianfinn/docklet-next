// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original mode matrix for Huma and Echo authentication.

package auth

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func modeTestService(t *testing.T) (*AuthService, *TokenPair) {
	t.Helper()
	db := setupAuthServiceTestDB(t)
	roles := role.NewRoleService(db)
	require.NoError(t, roles.EnsureBuiltInRoles(t.Context()))
	users := user.NewUserService(db, roles, session.RevokeAllUserSessionsExceptInDB)
	admin := &user.User{Username: "admin"}
	hash, err := users.HashPassword("example-password")
	require.NoError(t, err)
	admin.PasswordHash = hash
	_, err = users.CreateUser(t.Context(), admin)
	require.NoError(t, err)
	require.NoError(t, roles.SetUserAssignments(t.Context(), admin.ID, []role.UserRoleAssignment{{RoleID: authz.BuiltInRoleAdmin}}))
	settings, err := newSettingsServiceForAuthTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	svc := NewAuthService(users, settings, nil, session.NewSessionService(db), &config.Config{
		AuthMode: "local", AuthTrustedProxies: "192.0.2.10,2001:db8::/32", JWTRefreshExpiry: time.Hour,
		AppUrl: "https://hub.example.com", Environment: "production",
	}).WithSigningKey(newTestSigningKeyInternal())
	require.NoError(t, svc.InitializeSignIn(t.Context()))
	svc.browserSigningKey = bytes.Repeat([]byte{0x42}, browserSessionSigningKeySize)
	tokens, err := svc.CompleteLogin(t.Context(), admin, authtypes.SessionMeta{}, session.UserSessionSourceLocal)
	require.NoError(t, err)
	return svc, tokens
}

func modeTestRouter(t *testing.T, svc *AuthService, surface string, csrf bool) *echo.Echo {
	t.Helper()
	router := echo.New()
	// Ensure authentication uses the direct socket even when the framework trusts XFF.
	router.IPExtractor = func(r *http.Request) string { return r.Header.Get("X-Forwarded-For") }
	if csrf {
		router.Use(middleware.NewCSRFMiddleware(svc.config).Add())
	}
	if surface == "echo" {
		router.Use(NewAuthMiddleware(svc, svc.config).WithPermissionResolver(svc.userServiceRoleResolver()).Add())
		handler := func(c *echo.Context) error {
			actor, ok := c.Get(string(middleware.ContextKeyCurrentUser)).(*user.User)
			require.True(t, ok)
			return c.String(http.StatusOK, actor.ID+":"+actor.Username)
		}
		router.GET("/secure", handler)
		router.POST("/secure", handler)
	} else {
		api := humaecho.New(router, huma.DefaultConfig("test", "1.0.0"))
		api.UseMiddleware(NewHumaMiddleware(api, svc, nil, svc.userServiceRoleResolver(), nil, svc.config))
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			huma.Register(api, huma.Operation{OperationID: method + "-secure", Method: method, Path: "/secure", Security: []map[string][]string{{"BearerAuth": {}}}}, func(ctx context.Context, _ *struct{}) (*secureOutput, error) {
				actor, ok := user.CurrentUserFromContext(ctx)
				require.True(t, ok)
				out := &secureOutput{}
				out.Body.UserID = actor.ID + ":" + actor.Username
				return out, nil
			})
		}
	}
	return router
}

func (s *AuthService) userServiceRoleResolver() PermissionResolver {
	// The fixture resolves a single global administrator, matching this milestone.
	return staticPermissionResolverInternal{ps: authz.SudoPermissionSet()}
}

func TestSignInRequestMatrix(t *testing.T) {
	svc, tokens := modeTestService(t)
	cases := []struct {
		name, peer, identity, xff, credential string
		localStatus, proxyStatus              int
	}{
		{name: "no credential", peer: "198.51.100.5:5000", localStatus: 401, proxyStatus: 401},
		{name: "valid local bearer", peer: "198.51.100.5:5000", credential: "bearer", localStatus: 200, proxyStatus: 401},
		{name: "valid local cookie", peer: "198.51.100.5:5000", credential: "cookie", localStatus: 200, proxyStatus: 401},
		{name: "untrusted spoof", peer: "198.51.100.5:5000", identity: "operator", localStatus: 401, proxyStatus: 401},
		{name: "XFF spoof", peer: "198.51.100.5:5000", identity: "operator", xff: "192.0.2.10", localStatus: 401, proxyStatus: 401},
		{name: "trusted proxy", peer: "192.0.2.10:5000", identity: "operator", xff: "198.51.100.5", localStatus: 401, proxyStatus: 200},
		{name: "trusted IPv6", peer: "[2001:db8::10]:5000", identity: "operator", localStatus: 401, proxyStatus: 200},
		{name: "trusted missing identity", peer: "192.0.2.10:5000", localStatus: 401, proxyStatus: 401},
		{name: "unparseable peer", peer: "invalid", identity: "operator", localStatus: 401, proxyStatus: 401},
	}
	for _, mode := range []string{"none", "local", "proxy"} {
		svc.config.AuthMode = mode
		for _, surface := range []string{"huma", "echo"} {
			router := modeTestRouter(t, svc, surface, false)
			for _, tc := range cases {
				t.Run(mode+"/"+surface+"/"+tc.name, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, "/secure", http.NoBody)
					req.RemoteAddr = tc.peer
					req.Header.Set("Remote-User", tc.identity)
					req.Header.Set("X-Forwarded-For", tc.xff)
					if tc.credential == "bearer" {
						req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
					}
					if tc.credential == "cookie" {
						req.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: tokens.BrowserToken})
					}
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					want := 200
					if mode == "local" {
						want = tc.localStatus
					}
					if mode == "proxy" {
						want = tc.proxyStatus
					}
					require.Equal(t, want, rec.Code, rec.Body.String())
					if mode == "proxy" && want == 200 {
						require.Contains(t, rec.Body.String(), ":operator")
					}
				})
			}
		}
	}
	stored, err := svc.userService.GlobalAdministrator(t.Context())
	require.NoError(t, err)
	require.Equal(t, "admin", stored.Username)
}
