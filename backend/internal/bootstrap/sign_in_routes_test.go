// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original environment-forwarding and registered stream authentication matrix.

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/api/ws"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	apikeytypes "github.com/florianfinn/docklet-next/types/apikey"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

type signInRoutingFixture struct {
	cfg                       *config.Config
	svc                       *auth.AuthService
	roles                     *role.RoleService
	keys                      *apikey.ApiKeyService
	tokens                    *auth.TokenPair
	bootstrapKey, personalKey string
	adminID                   string
}

func newSignInRoutingFixture(t *testing.T) signInRoutingFixture {
	t.Helper()
	cfg := &config.Config{AuthMode: "local", AuthTrustedProxies: "192.0.2.10", AppUrl: "https://hub.example.com", Environment: "production", JWTRefreshExpiry: time.Hour, DatabaseURL: sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db"))}
	db, err := initializeDBAndMigrateInternal(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	roles := role.NewRoleService(db)
	require.NoError(t, roles.EnsureBuiltInRoles(t.Context()))
	users := user.NewUserService(db, roles, session.RevokeAllUserSessionsExceptInDB)
	require.NoError(t, users.CreateDefaultAdmin(t.Context()))
	admin, err := users.GlobalAdministrator(t.Context())
	require.NoError(t, err)
	settingsSvc, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, settingsSvc.Stop(context.WithoutCancel(t.Context()))) })
	crypto.InitEncryption(&crypto.Config{EncryptionKey: "test-only-encryption-key-32-chars", Environment: "test"})
	svc := auth.NewAuthService(users, settingsSvc, nil, session.NewSessionService(db), cfg)
	require.NoError(t, svc.InitializeSignIn(t.Context()))
	tokens, err := svc.CompleteLogin(t.Context(), admin, authtypes.SessionMeta{}, session.UserSessionSourceLocal)
	require.NoError(t, err)
	keys := apikey.NewApiKeyService(db, users, roles)
	env := &environment.Environment{ID: "example", Name: "Example", ApiUrl: "https://agent.example.com", Enabled: true}
	require.NoError(t, db.Create(env).Error)
	bootstrapKey, err := keys.CreateEnvironmentApiKey(t.Context(), env.ID)
	require.NoError(t, err)
	personalKey, err := keys.CreatePersonalApiKey(t.Context(), admin.ID, apikeytypes.CreateUserApiKey{Name: "example"})
	require.NoError(t, err)
	return signInRoutingFixture{cfg, svc, roles, keys, tokens, bootstrapKey.Key, personalKey.Key, admin.ID}
}

func TestSignInForwardingAndStreamMatrix(t *testing.T) {
	fixture := newSignInRoutingFixture(t)
	cfg, svc, roles, keys, tokens := fixture.cfg, fixture.svc, fixture.roles, fixture.keys, fixture.tokens
	transportValidator := csrfTransportValidatorInternal(api.HandlerDeps{ApiKey: apikey.New(keys)})
	validator := createAuthValidatorInternal(api.HandlerDeps{Auth: auth.New(svc, nil, nil), ApiKey: apikey.New(keys), Role: role.New(roles)})
	for _, mode := range []string{"none", "local", "proxy"} {
		cfg.AuthMode = mode
		transportRouter := echo.New()
		transportRouter.Use(middleware.NewCSRFMiddleware(cfg).WithTransportValidator(transportValidator).
			WithLocalCredentialValidator(auth.NewAuthMiddleware(svc, cfg).WithApiKeyValidator(keys).ValidateLocalHeaderCredential).Add())
		transportRouter.POST("/api/transport", func(c *echo.Context) error { return c.NoContent(200) })
		for _, tc := range []struct {
			value  string
			status int
		}{{fixture.bootstrapKey, 200}, {fixture.personalKey, 403}, {"invalid", 403}} {
			req := httptest.NewRequest(http.MethodPost, "/api/transport", http.NoBody)
			req.Header.Set(middleware.HeaderApiKey, tc.value)
			rec := httptest.NewRecorder()
			transportRouter.ServeHTTP(rec, req)
			want := tc.status
			if mode == "local" && tc.value != fixture.bootstrapKey {
				want = 401
				if tc.value == fixture.personalKey {
					want = 200
				}
			}
			require.Equal(t, want, rec.Code)
		}
		router := echo.New()
		router.IPExtractor = func(req *http.Request) string { return req.Header.Get("X-Forwarded-For") }
		ws.NewWebSocketHandler(router.Group("/api"), nil, nil, nil, nil, auth.NewAuthMiddleware(svc, cfg).WithPermissionResolver(roles), cfg)
		for _, tc := range []struct {
			name, peer, identity, credential string
			localStatus, proxyStatus         int
		}{
			{"no credential", "198.51.100.5:5000", "", "", 401, 401},
			{"local bearer", "198.51.100.5:5000", "", "bearer", 200, 401},
			{"local browser cookie", "198.51.100.5:5000", "", "cookie", 200, 401},
			{"untrusted XFF spoof", "198.51.100.5:5000", "operator", "", 401, 401},
			{"trusted proxy", "192.0.2.10:5000", "operator", "", 401, 200},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				request := func(path string) *http.Request {
					req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
					req.RemoteAddr = tc.peer
					req.Header.Set("Remote-User", tc.identity)
					req.Header.Set("X-Forwarded-For", "192.0.2.10")
					req.Header.Set("Origin", "https://hub.example.com")
					if tc.credential == "bearer" {
						req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
					}
					if tc.credential == "cookie" {
						req.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: tokens.BrowserToken})
					}
					return req
				}
				want := 200
				if mode == "local" {
					want = tc.localStatus
				}
				if mode == "proxy" {
					want = tc.proxyStatus
				}
				c := router.NewContext(request("/api/environments/example/containers"), httptest.NewRecorder())
				ps, actor, ok := validator(t.Context(), c)
				require.Equal(t, want == 200, ok)
				if ok {
					require.True(t, ps.IsGlobalAdmin())
					require.Equal(t, fixture.adminID, actor.ID)
				}
				for _, path := range []string{"/api/diagnostics/stream", "/api/environments/0/ws/system/stats"} {
					req := request(path)
					req.Header.Set("Origin", "https://other.example.com")
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					status := want
					if want == 200 {
						status = 403
					}
					require.Equal(t, status, rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestSignInInvalidStartupFailsBeforeDatabase(t *testing.T) {
	for _, tc := range []struct{ name, mode, peers string }{
		{"invalid mode", "invalid", ""},
		{"missing proxy policy", "proxy", ""},
		{"bad proxy policy", "proxy", "192.0.2.10,invalid"},
		{"all IPv4 peers", "proxy", "0.0.0.0/0"},
		{"all IPv6 peers", "proxy", "::/0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "unopened.db")
			t.Setenv("DATABASE_URL", sqlitestore.URL(path))
			t.Setenv("AUTH_MODE", tc.mode)
			t.Setenv("AUTH_TRUSTED_PROXIES", tc.peers)
			t.Setenv("AUTH_MODE_FILE", "")
			t.Setenv("AUTH_MODE__FILE", "")
			t.Setenv("AUTH_TRUSTED_PROXIES_FILE", "")
			t.Setenv("AUTH_TRUSTED_PROXIES__FILE", "")
			require.Error(t, Bootstrap(t.Context()))
			_, err := os.Stat(path)
			require.True(t, os.IsNotExist(err))
		})
	}
}
