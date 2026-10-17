// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original second-path, cache and restart regression tests.

package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestSignInRejectsCachedLocalTokensOutsideLocal(t *testing.T) {
	for _, mode := range []string{"none", "proxy"} {
		t.Run(mode, func(t *testing.T) {
			svc, tokens := modeTestService(t)
			_, _, err := svc.VerifyToken(t.Context(), tokens.AccessToken)
			require.NoError(t, err)
			_, _, err = svc.VerifyBrowserToken(t.Context(), tokens.BrowserToken)
			require.NoError(t, err)
			svc.config.AuthMode = mode
			_, _, err = svc.VerifyToken(t.Context(), tokens.AccessToken)
			require.Error(t, err)
			_, _, err = svc.VerifyBrowserToken(t.Context(), tokens.BrowserToken)
			require.Error(t, err)
			_, err = svc.RefreshToken(t.Context(), tokens.RefreshToken, authtypes.SessionMeta{})
			require.Error(t, err)
			_, err = svc.AuthenticateLocalPrimary(t.Context(), "admin", "example-password")
			require.ErrorIs(t, err, ErrLocalAuthDisabled)
			admin, err := svc.userService.GlobalAdministrator(t.Context())
			require.NoError(t, err)
			_, err = svc.CompleteLogin(t.Context(), admin, authtypes.SessionMeta{}, session.UserSessionSourceLocal)
			require.ErrorIs(t, err, ErrLocalAuthDisabled)
		})
	}
}

func TestSignInModeRestartRevokesSessions(t *testing.T) {
	for _, nextMode := range []string{"none", "proxy"} {
		t.Run(nextMode, func(t *testing.T) {
			svc, tokens := modeTestService(t)
			_, _, err := svc.VerifyToken(t.Context(), tokens.AccessToken)
			require.NoError(t, err)
			require.NoError(t, svc.InitializeSignIn(t.Context()))
			_, _, err = svc.VerifyToken(t.Context(), tokens.AccessToken)
			require.NoError(t, err, "same-mode restart preserves sessions")
			svc.config.AuthMode = nextMode
			require.NoError(t, svc.InitializeSignIn(t.Context()))
			svc.config.AuthMode = "local"
			require.NoError(t, svc.InitializeSignIn(t.Context()))
			_, _, err = svc.VerifyToken(t.Context(), tokens.AccessToken)
			require.Error(t, err)
			_, _, err = svc.VerifyBrowserToken(t.Context(), tokens.BrowserToken)
			require.Error(t, err)
			_, err = svc.RefreshToken(t.Context(), tokens.RefreshToken, authtypes.SessionMeta{})
			require.Error(t, err)
		})
	}
}

func TestSignInLocalEndpointsUnavailableOutsideLocal(t *testing.T) {
	for _, mode := range []string{"none", "proxy"} {
		svc, _ := modeTestService(t)
		svc.config.AuthMode = mode
		router := echo.New()
		api := humaecho.New(router, huma.DefaultConfig("test", "1.0.0"))
		api.UseMiddleware(NewHumaMiddleware(api, svc, nil, nil, nil, svc.config))
		RegisterAuth(api, svc.userService, svc, svc.settingsService)
		for _, endpoint := range []struct{ path, body string }{
			{"login", `{"username":"admin","password":"example"}`},
			{"refresh", `{"refreshToken":"example"}`},
			{"password", `{"currentPassword":"example","newPassword":"example"}`},
		} {
			t.Run(mode+"/"+endpoint.path, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "/auth/"+endpoint.path, strings.NewReader(endpoint.body))
				req.RemoteAddr = "192.0.2.10:5000"
				req.Header.Set("Remote-User", "operator")
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				require.Equal(t, 401, rec.Code, rec.Body.String())
			})
		}
	}
}

func TestSignInIgnoresMutableLocalEnabledSetting(t *testing.T) {
	svc, _ := modeTestService(t)
	require.NoError(t, svc.settingsService.SetBoolSetting(t.Context(), "authLocalEnabled", false))
	enabled, err := svc.IsLocalAuthEnabled(t.Context())
	require.NoError(t, err)
	require.True(t, enabled)
	require.NoError(t, svc.settingsService.PruneUnknownSettings(t.Context()))
	current, err := svc.settingsService.GetSettings(t.Context())
	require.NoError(t, err)
	// Refresh the snapshot after the direct startup transaction.
	require.NoError(t, svc.settingsService.UpdateSetting(t.Context(), "authLastMode", "local"))
	current, err = svc.settingsService.GetSettings(t.Context())
	require.NoError(t, err)
	require.Equal(t, "local", current.AuthLastMode.Value)
}

func TestSignInProxyHeaderConfiguration(t *testing.T) {
	svc, _ := modeTestService(t)
	svc.config.AuthMode = "proxy"
	svc.config.AuthProxyHeader = "X-Identity"
	for _, tc := range []struct {
		name, header string
		values       []string
		status       int
	}{
		{"configured header", "X-Identity", []string{"operator"}, 200},
		{"default header ignored", "Remote-User", []string{"operator"}, 401},
		{"duplicate identities", "X-Identity", []string{"operator", "other"}, 401},
		{"blank identity", "X-Identity", []string{" "}, 401},
		{"printable Unicode", "X-Identity", []string{"Öperator"}, 200},
		{"tab", "X-Identity", []string{"oper\tator"}, 401},
		{"leading tab", "X-Identity", []string{"\toperator"}, 401},
		{"trailing tab", "X-Identity", []string{"operator\t"}, 401},
		{"escape", "X-Identity", []string{"oper\x1bator"}, 401},
		{"DEL", "X-Identity", []string{"operator\x7f"}, 401},
		{"newline", "X-Identity", []string{"operator\n"}, 401},
		{"carriage return", "X-Identity", []string{"operator\r"}, 401},
		{"NUL", "X-Identity", []string{"oper\x00ator"}, 401},
		{"Unicode control", "X-Identity", []string{"operator\u0085"}, 401},
		{"Unicode format", "X-Identity", []string{"operator\u200b"}, 401},
		{"invalid UTF-8", "X-Identity", []string{"operator\xff"}, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/secure", http.NoBody)
			req.RemoteAddr = "192.0.2.10:5000"
			for _, value := range tc.values {
				req.Header.Add(tc.header, value)
			}
			for _, surface := range []string{"huma", "echo"} {
				router := modeTestRouter(t, svc, surface, false)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				require.Equal(t, tc.status, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSignInAgentTransportUnaffected(t *testing.T) {
	svc, _ := modeTestService(t)
	for _, edgeMode := range []bool{false, true} {
		for _, mode := range []string{"none", "local", "proxy"} {
			cfg := *svc.config
			cfg.AgentMode = true
			cfg.EdgeAgent = edgeMode
			cfg.AuthMode = mode
			cfg.AgentToken = "example"
			svc.config = &cfg
			router := echo.New()
			router.Use(NewAuthMiddleware(svc, &cfg).Add())
			router.POST("/api/agent", func(c *echo.Context) error { return c.NoContent(200) })
			for _, token := range []string{"", "wrong", "example"} {
				req := httptest.NewRequest(http.MethodPost, "/api/agent", http.NoBody)
				req.Header.Set("X-Arcane-Agent-Token", token)
				req.Header.Set("Remote-User", "operator")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				expected := 403
				if token == "example" {
					expected = 200
				}
				require.Equal(t, expected, rec.Code)
			}
			require.Equal(t, authtypes.SignInModeLocal, svc.SignInMode())
		}
	}
}
