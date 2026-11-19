// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original pending forwarding, machine ingress and post-claim static key tests over the production wiring.

package bootstrap

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/coder/websocket"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
)

// pendingRoutingFixture turns the claimed routing fixture into an upgraded, pending hub.
func pendingRoutingFixture(t *testing.T, afterClaim func(context.Context) error) signInRoutingFixture {
	t.Helper()
	f := newSignInRoutingFixture(t)
	f.cfg.AuthMode = ""
	require.NoError(t, f.db.Where("key = ?", session.SettingSetupIdentity).Delete(&settings.SettingVariable{}).Error)
	if afterClaim != nil {
		f.svc.SetAfterClaim(afterClaim)
	}
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	err := f.svc.InitializeSignIn(t.Context())
	slog.SetDefault(previous)
	require.NoError(t, err)
	require.True(t, f.svc.CurrentPolicy().Pending)
	require.Contains(t, output.String(), "First-run setup is pending")
	return f
}

func pendingForwardingServer(t *testing.T, f signInRoutingFixture) *httptest.Server {
	t.Helper()
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a pending request reached the agent")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(agent.Close)
	deps := api.HandlerDeps{Auth: auth.New(f.svc, nil, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles)}
	resolver := func(context.Context, string) (string, *string, bool, error) {
		return agent.URL, new("agent-example"), true, nil
	}
	router := echo.New()
	router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver, createAuthValidatorInternal(deps), nil, nil, nil))
	router.GET("/api/environments/:id/*", func(c *echo.Context) error { return c.NoContent(http.StatusInternalServerError) })
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server
}

func TestPendingForwardingKeepsTheTypedProblem(t *testing.T) {
	f := pendingRoutingFixture(t, nil)
	server := pendingForwardingServer(t, f)
	for _, credential := range []string{"", "bearer"} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/environments/example/containers", http.NoBody)
		require.NoError(t, err)
		if credential == "bearer" {
			req.Header.Set("Authorization", "Bearer "+f.tokens.AccessToken)
		}
		response, err := server.Client().Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, response.Body.Close())
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, response.StatusCode, credential)
		require.Contains(t, string(body), authtypes.CodeSetupPending, credential)
	}
}

func TestPendingWebSocketHandshakeIsRefused(t *testing.T) {
	f := pendingRoutingFixture(t, nil)
	server := pendingForwardingServer(t, f)
	conn, response, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http")+"/api/environments/example/containers/example/logs/ws", nil)
	if conn != nil {
		_ = conn.CloseNow()
	}
	require.Error(t, err, "the handshake must not upgrade while setup is pending")
	require.NotNil(t, response)
	body, _ := io.ReadAll(response.Body)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.Contains(t, string(body), authtypes.CodeSetupPending)
}

func TestPendingKeepsEnvironmentTokenIngress(t *testing.T) {
	f := pendingRoutingFixture(t, nil)
	validator := csrfTransportValidatorInternal(api.HandlerDeps{ApiKey: apikey.New(f.keys)})
	router := echo.New()
	router.Use(middleware.NewCSRFMiddleware(f.cfg).WithTransportValidator(validator).
		WithLocalCredentialValidator(auth.NewAuthMiddleware(f.svc, f.cfg).WithApiKeyValidator(f.keys).ValidateLocalHeaderCredential).Add())
	router.POST("/api/transport", func(c *echo.Context) error { return c.NoContent(http.StatusOK) })
	// The environment bootstrap key travels in X-API-Key; the transport validator never consults the policy.
	req := httptest.NewRequest(http.MethodPost, "/api/transport", http.NoBody)
	req.Header.Set(middleware.HeaderApiKey, f.bootstrapKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	personal := httptest.NewRequest(http.MethodPost, "/api/transport", http.NoBody)
	personal.Header.Set(middleware.HeaderApiKey, f.personalKey)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, personal)
	require.NotEqual(t, http.StatusOK, rec.Code, "a user credential gets no pending exemption")
}

func TestStaticAdminKeyIsAuthorizedRightAfterClaim(t *testing.T) {
	raw := "arc_" + strings.Repeat("example", 5)
	var keys *apikey.ApiKeyService
	var roles *role.RoleService
	f := pendingRoutingFixture(t, func(ctx context.Context) error {
		return staticKeyAfterClaimInternal(keys, roles, raw)(ctx)
	})
	keys, roles = f.keys, f.roles
	settingsSvc, err := settings.NewSettingsService(t.Context(), f.db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, settingsSvc.Stop(context.WithoutCancel(t.Context()))) })
	password := setupExamplePassword
	_, err = f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), authtypes.SetupIdentityRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeLocal}, Username: "owner", Password: &password}, authtypes.SessionMeta{})
	require.NoError(t, err)

	router := echo.New()
	humaAPI := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
	humaAPI.UseMiddleware(auth.NewHumaMiddleware(humaAPI, f.svc, f.keys, f.roles, nil, f.cfg))
	auth.RegisterAuth(humaAPI, f.users, f.svc, settingsSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/sign-in", nil)
	req.Header.Set(middleware.HeaderApiKey, raw)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "the static admin key works without a restart: %s", rec.Body.String())
}

// After a stored runtime change to a non-default proxy header, real Direct HTTP and Edge tunnel
// traffic reaches the agent without the active identity header.
func TestRuntimeIdentityHeaderIsStrippedOverRealTransports(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newSignInRoutingFixture(t)
			f.cfg.AuthMode = ""
			require.NoError(t, f.svc.InitializeSignIn(t.Context()))
			change := httptest.NewRequest(http.MethodPut, "/", nil)
			change.RemoteAddr = "127.0.0.1:5000"
			change.Header.Set("X-Example-User", "admin")
			_, err := f.svc.UpdateSignIn(t.Context(), change, f.adminID, authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeProxy, ProxyHeader: "X-Example-User", TrustedProxies: "127.0.0.1"}}, authtypes.SessionMeta{})
			require.NoError(t, err)

			var mu sync.Mutex
			var received []http.Header
			agent := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/environments/0/containers" {
					mu.Lock()
					received = append(received, r.Header.Clone())
					mu.Unlock()
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
			})
			direct := httptest.NewServer(agent)
			t.Cleanup(direct.Close)
			endpoint := direct.URL
			var registry *edge.TunnelRegistry
			if mode == "edge" {
				actualSettingsTunnelForEnvironment(t, agent, "agent-example", "example")
				registry, endpoint = edge.GetRegistry(), "edge://example"
			}
			deps := api.HandlerDeps{Auth: auth.New(f.svc, nil, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles)}
			token := "agent-example"
			router := echo.New()
			router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) { return endpoint, &token, true, nil },
				createAuthValidatorInternal(deps), nil, registry, nil,
				middleware.EnvironmentProxySecurity{ProxyIdentityHeader: f.cfg.ProxyIdentityHeader(), ProxyIdentityHeaderFunc: proxyIdentityHeaderSourceInternal(f.svc)}))
			router.GET("/api/environments/:id/*", func(c *echo.Context) error { return c.NoContent(http.StatusInternalServerError) })
			hub := httptest.NewServer(router)
			t.Cleanup(hub.Close)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, hub.URL+"/api/environments/example/containers", http.NoBody)
			require.NoError(t, err)
			req.Header.Set("X-Example-User", "admin")
			response, err := hub.Client().Do(req)
			require.NoError(t, err)
			body, _ := io.ReadAll(response.Body)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusOK, response.StatusCode, string(body))
			mu.Lock()
			defer mu.Unlock()
			require.Len(t, received, 1)
			require.Empty(t, received[0].Get("X-Example-User"), "the agent never sees the runtime identity header")
		})
	}
}

// Policy publication precedes the post-claim key reconciliation, so a concurrent request may cache the
// new static key without grants; the claim must drop that cache.
func TestStaticAdminKeyCacheFilledDuringClaimIsDropped(t *testing.T) {
	raw := "arc_" + strings.Repeat("example", 5)
	var keys *apikey.ApiKeyService
	var roles *role.RoleService
	f := pendingRoutingFixture(t, func(ctx context.Context) error {
		return staticKeyAfterClaimInternal(keys, roles, raw)(ctx)
	})
	keys, roles = f.keys, f.roles
	primed := false
	var keyID string
	// Backfill lists keys after reconciliation committed and before it writes grants.
	require.NoError(t, f.db.Callback().Query().After("gorm:query").Register("example:key_cache", func(tx *gorm.DB) {
		rows, ok := tx.Statement.Dest.(*[]apikey.ApiKey)
		if !ok || primed || f.svc.CurrentPolicy().Pending {
			return
		}
		for _, key := range *rows {
			if key.ManagedBy == nil {
				continue
			}
			primed, keyID = true, key.ID
			ps, err := roles.ResolveApiKeyPermissions(t.Context(), key.ID)
			require.NoError(t, err)
			require.False(t, ps.Allows(authz.PermSettingsRead, "0"), "empty grants are cached at this point")
		}
	}))
	t.Cleanup(func() { _ = f.db.Callback().Query().Remove("example:key_cache") })
	password := setupExamplePassword
	_, err := f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), authtypes.SetupIdentityRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeLocal}, Username: "owner", Password: &password}, authtypes.SessionMeta{})
	require.NoError(t, err)
	require.True(t, primed)
	ps, err := roles.ResolveApiKeyPermissions(t.Context(), keyID)
	require.NoError(t, err)
	require.True(t, ps.Allows(authz.PermSettingsRead, "0"), "post-claim invalidation discards the empty cache")
}

// setupExamplePassword is a synthetic password for test claims only.
const setupExamplePassword = "Example-Setup-Password1!"

type forwardingProbeKey struct{}

// The forwarding validator binds its viewer to the policy it authenticated under, even when local
// sign-in is published in between.
func TestForwardingViewerKeepsTheAuthenticationPolicy(t *testing.T) {
	f := newSignInRoutingFixture(t)
	f.cfg.AuthMode = ""
	require.NoError(t, f.svc.InitializeSignIn(t.Context()))
	_, err := f.svc.UpdateSignIn(t.Context(), httptest.NewRequest(http.MethodPut, "/", nil), f.adminID, authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeNone}}, authtypes.SessionMeta{})
	require.NoError(t, err)
	entered, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	require.NoError(t, f.db.Callback().Query().After("gorm:query").Register("example:pause_forwarding", func(tx *gorm.DB) {
		if tx.Statement.Context.Value(forwardingProbeKey{}) == true {
			once.Do(func() { close(entered); <-resume })
		}
	}))
	t.Cleanup(func() { _ = f.db.Callback().Query().Remove("example:pause_forwarding") })
	validator := createAuthValidatorInternal(api.HandlerDeps{Auth: auth.New(f.svc, nil, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles)})
	req := httptest.NewRequest(http.MethodGet, "/api/environments/example/containers", nil)
	req = req.WithContext(context.WithValue(req.Context(), forwardingProbeKey{}, true))
	c := echo.New().NewContext(req, httptest.NewRecorder())
	done := make(chan bool, 1)
	go func() {
		_, _, ok := validator(req.Context(), c)
		done <- ok
	}()
	<-entered
	password := setupExamplePassword
	_, err = f.svc.UpdateSignIn(t.Context(), httptest.NewRequest(http.MethodPut, "/", nil), f.adminID, authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeLocal}, Password: &password}, authtypes.SessionMeta{})
	close(resume)
	require.NoError(t, err)
	require.True(t, <-done, "the request authenticated under none mode")
	require.Error(t, sessionlife.Check(c.Request().Context()), "its viewer must not adopt the later local policy")
}

// Local Bearer and cookie forwarding keep the snapshot too when none is published between the
// session lookup and the viewer binding.
func TestForwardingLocalViewerKeepsTheAuthenticationPolicy(t *testing.T) {
	for _, credential := range []string{"bearer", "cookie"} {
		t.Run(credential, func(t *testing.T) {
			f := newSignInRoutingFixture(t)
			f.cfg.AuthMode = ""
			require.NoError(t, f.svc.InitializeSignIn(t.Context()))
			entered, resume := make(chan struct{}), make(chan struct{})
			var once sync.Once
			require.NoError(t, f.db.Callback().Query().After("gorm:query").Register("example:pause_local", func(tx *gorm.DB) {
				if _, ok := tx.Statement.Dest.(*session.UserSession); ok && tx.Statement.Context.Value(forwardingProbeKey{}) == true {
					once.Do(func() { close(entered); <-resume })
				}
			}))
			t.Cleanup(func() { _ = f.db.Callback().Query().Remove("example:pause_local") })
			validator := createAuthValidatorInternal(api.HandlerDeps{Auth: auth.New(f.svc, nil, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles)})
			req := httptest.NewRequest(http.MethodGet, "/api/environments/example/containers", nil)
			if credential == "bearer" {
				req.Header.Set("Authorization", "Bearer "+f.tokens.AccessToken)
			} else {
				req.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: f.tokens.BrowserToken})
			}
			req = req.WithContext(context.WithValue(req.Context(), forwardingProbeKey{}, true))
			c := echo.New().NewContext(req, httptest.NewRecorder())
			done := make(chan bool, 1)
			go func() {
				_, _, ok := validator(req.Context(), c)
				done <- ok
			}()
			<-entered
			_, err := f.svc.UpdateSignIn(t.Context(), httptest.NewRequest(http.MethodPut, "/", nil), f.adminID, authtypes.SignInUpdateRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeNone}}, authtypes.SessionMeta{})
			close(resume)
			require.NoError(t, err)
			require.True(t, <-done, "the request authenticated before the publication")
			require.Error(t, sessionlife.Check(c.Request().Context()), "local authentication keeps the policy it was checked against")
		})
	}
}
