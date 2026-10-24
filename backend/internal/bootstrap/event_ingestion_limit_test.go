// Source: docklet-next/backend/internal/bootstrap/router_bootstrap.go
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original method-specific event ingestion rate-limit regression tests.

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"go.uber.org/fx/fxtest"
)

func newEventIngestionLimitRouter(t *testing.T) (*echo.Echo, *http.Cookie) {
	t.Helper()
	cfg := &config.Config{
		AuthMode: "local", AppUrl: "https://hub.example.com", Environment: "production",
		JWTRefreshExpiry: time.Hour, DatabaseURL: sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")),
	}
	db, err := initializeDBAndMigrateInternal(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	roles := role.NewRoleService(db)
	require.NoError(t, roles.EnsureBuiltInRoles(t.Context()))
	users := user.NewUserService(db, roles, session.RevokeAllUserSessionsExceptInDB)
	require.NoError(t, users.CreateDefaultAdmin(t.Context()))
	admin, err := users.GlobalAdministrator(t.Context())
	require.NoError(t, err)
	settingsService, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, settingsService.Stop(context.WithoutCancel(t.Context()))) })
	crypto.InitEncryption(&crypto.Config{EncryptionKey: "test-only-encryption-key-32-chars", Environment: "test"})
	authService := auth.NewAuthService(users, settingsService, nil, session.NewSessionService(db), cfg)
	require.NoError(t, authService.InitializeSignIn(t.Context()))
	tokens, err := authService.CompleteLogin(t.Context(), admin, authtypes.SessionMeta{}, session.UserSessionSourceLocal)
	require.NoError(t, err)
	require.NoError(t, db.Create(&environment.Environment{
		ID: "example", Name: "Example", ApiUrl: "https://agent.example.com", Enabled: true,
		AccessToken: new("example-agent-token"),
	}).Error)
	events := event.NewEventService(db, cfg, nil)
	keys := apikey.NewApiKeyService(db, users, roles)
	environments := environment.NewEnvironmentService(db, nil, nil, events, settingsService, keys)
	router, _ := newRouter(RouterParams{
		Context: t.Context(), Config: cfg, Lifecycle: fxtest.NewLifecycle(t),
		HandlerDeps: api.HandlerDeps{
			Auth: auth.New(authService, users, settingsService), Role: role.New(roles), ApiKey: apikey.New(keys),
			Event:       event.New(events),
			Environment: environment.New(environments, settingsService, keys, events, cfg, nil),
			Project:     project.New(&project.ProjectService{}, nil),
			Container:   container.New(&container.ContainerService{}, nil, nil, nil),
			System:      system.New(&system.SystemService{}, nil, nil, nil, nil),
		},
		AuthMiddleware: auth.NewAuthMiddleware(authService, cfg).WithApiKeyValidator(keys).WithPermissionResolver(roles),
		TunnelRegistry: edge.NewTunnelRegistry(),
	})
	return router, &http.Cookie{Name: cookie.InsecureTokenCookieName, Value: tokens.BrowserToken}
}

func TestEventIngestionLimitSeparatesAuthenticatedReads(t *testing.T) {
	for _, readsFirst := range []bool{true, false} {
		name := "post-burst-before-reads"
		if readsFirst {
			name = "reads-before-post-burst"
		}
		t.Run(name, func(t *testing.T) {
			router, sessionCookie := newEventIngestionLimitRouter(t)
			bootstrap := httptest.NewRecorder()
			router.ServeHTTP(bootstrap, httptest.NewRequest(http.MethodGet, "/api/auth/mode", http.NoBody))
			require.Equal(t, http.StatusOK, bootstrap.Code)
			csrfToken := bootstrap.Header().Get(middleware.CSRFHeader)
			require.NotEmpty(t, csrfToken)
			request := func(method, peer string, authenticated bool) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, "/api/events?limit=100", http.NoBody)
				req.RemoteAddr = peer
				if authenticated {
					req.AddCookie(sessionCookie)
				}
				if method == http.MethodPost {
					req.Header.Set(middleware.HeaderAgentToken, "invalid")
					req.Header.Set("Origin", "https://hub.example.com")
					req.Header.Set(middleware.CSRFHeader, csrfToken)
					req.AddCookie(&http.Cookie{Name: "docklet_csrf", Value: csrfToken})
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				return rec
			}
			const peer = "192.0.2.20:5000"
			reads := func() {
				for i := range 40 {
					rec := request(http.MethodGet, peer, true)
					require.Equal(t, http.StatusOK, rec.Code, "authenticated GET %d: %s", i+1, rec.Body.String())
					require.Empty(t, rec.Header().Get("Retry-After"))
					require.Contains(t, rec.Body.String(), `"success":true`)
				}
			}
			postBurst := func() {
				for i := range 30 {
					rec := request(http.MethodPost, peer, false)
					require.Equal(t, http.StatusUnauthorized, rec.Code, "POST %d must reach token authentication: %s", i+1, rec.Body.String())
					require.Contains(t, rec.Body.String(), "invalid agent token")
				}
				rec := request(http.MethodPost, peer, false)
				require.Equal(t, http.StatusTooManyRequests, rec.Code, "POST 31 must exhaust the ingestion burst")
				require.Equal(t, "60", rec.Header().Get("Retry-After"))
				require.JSONEq(t, `{"error":"rate limit exceeded"}`, rec.Body.String())
			}
			if readsFirst {
				reads()
				postBurst()
			} else {
				postBurst()
				reads()
			}
			require.Equal(t, http.StatusUnauthorized, request(http.MethodGet, peer, false).Code,
				"event reads still require a session after the ingestion burst")
			require.Equal(t, http.StatusUnauthorized, request(http.MethodPost, "192.0.2.21:5000", false).Code,
				"a distinct IP retains its own ingestion budget")
		})
	}
}

func TestEventIngestionLimitAllowsValidAgentPayload(t *testing.T) {
	router, sessionCookie := newEventIngestionLimitRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/events", strings.NewReader(`{"type":"container.start","title":"Example event"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.HeaderAgentToken, "example-agent-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	read := httptest.NewRequest(http.MethodGet, "/api/events", http.NoBody)
	read.AddCookie(sessionCookie)
	listed := httptest.NewRecorder()
	router.ServeHTTP(listed, read)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	require.Contains(t, listed.Body.String(), `"title":"Example event"`)
	require.Contains(t, listed.Body.String(), `"environmentId":"example"`)
}
