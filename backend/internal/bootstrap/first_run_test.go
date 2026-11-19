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
	"regexp"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
)

// pendingRoutingFixture turns the claimed routing fixture into an upgraded, pending hub and returns
// the setup code from the startup log.
func pendingRoutingFixture(t *testing.T, afterClaim func(context.Context) error) (signInRoutingFixture, string) {
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
	code := regexp.MustCompile(`Setup code: ([A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4})`).FindStringSubmatch(output.String())
	require.Len(t, code, 2, output.String())
	return f, code[1]
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
	f, _ := pendingRoutingFixture(t, nil)
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
	f, _ := pendingRoutingFixture(t, nil)
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
	f, _ := pendingRoutingFixture(t, nil)
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
	f, code := pendingRoutingFixture(t, func(ctx context.Context) error {
		return staticKeyAfterClaimInternal(keys, roles, raw)(ctx)
	})
	keys, roles = f.keys, f.roles
	settingsSvc, err := settings.NewSettingsService(t.Context(), f.db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, settingsSvc.Stop(context.WithoutCancel(t.Context()))) })
	password := "Example-Setup-Password1!"
	_, err = f.svc.ClaimSetup(t.Context(), httptest.NewRequest(http.MethodPost, "/", nil), authtypes.SetupIdentityRequest{SignInPolicyInput: authtypes.SignInPolicyInput{Mode: authtypes.SignInModeLocal}, Code: code, Username: "owner", Password: &password}, authtypes.SessionMeta{})
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
