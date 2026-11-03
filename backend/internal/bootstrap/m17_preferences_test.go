// Source: docklet-next/issues/339
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original personal preferences through real authentication and access middleware.

package bootstrap

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	authDTO "github.com/florianfinn/docklet-next/types/auth"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func preferencesRouter(t *testing.T, f signInRoutingFixture) *echo.Echo {
	t.Helper()
	router := echo.New()
	hosts := environment.NewEnvironmentService(f.db, http.DefaultClient, nil, nil, nil, f.keys)
	api.SetupAPI(router, router.Group("/api"), handlerutil.NewActivityAppContext(t.Context()), f.cfg, api.HandlerDeps{Auth: auth.New(f.svc, f.users, nil), ApiKey: apikey.New(f.keys), Role: role.New(f.roles), Environment: environment.New(hosts, nil, nil, nil, f.cfg, nil)})
	return router
}
func preferenceRequest(router http.Handler, method, section, body, credential, peer, identity string) *httptest.ResponseRecorder {
	path := "/api/auth/me/ui-preferences"
	if section != "" {
		path += "/" + section
	}
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if strings.HasPrefix(credential, "Bearer ") {
		request.Header.Set("Authorization", credential)
	} else if credential != "" {
		request.Header.Set(middleware.HeaderApiKey, credential)
	}
	request.RemoteAddr = peer
	request.Header.Set("Remote-User", identity)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
func TestPF4PF5PF6PF8RealPreferencePrincipalMatrix(t *testing.T) {
	f := newSignInRoutingFixture(t)
	viewer, err := f.users.CreateUser(t.Context(), &user.User{Username: "viewer"})
	require.NoError(t, err)
	tokens, err := f.svc.CompleteLogin(t.Context(), viewer, authDTO.SessionMeta{}, "local")
	require.NoError(t, err)
	for _, tc := range []struct {
		name, mode, credential, peer, identity, owner string
		adminList                                     *string
	}{
		{"local administrator", "local", "Bearer " + f.tokens.AccessToken, "198.51.100.5:5000", "", f.adminID, nil},
		{"simple user", "local", "Bearer " + tokens.AccessToken, "198.51.100.5:5000", "", viewer.ID, nil},
		{"personal key", "local", f.personalKey, "198.51.100.5:5000", "", f.adminID, nil},
		{"none", "none", "", "198.51.100.5:5000", "", f.adminID, nil},
		{"proxy shared", "proxy", "", "192.0.2.10:5000", "anyone", f.adminID, nil},
		{"proxy listed administrator", "proxy", "", "192.0.2.10:5000", "Operator", f.adminID, new("Operator")},
		{"proxy registered user", "proxy", "", "192.0.2.10:5000", "viewer", viewer.ID, new("Operator")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.cfg.AuthMode = tc.mode
			f.cfg.AuthProxyAdminUsers = tc.adminList
			router := preferencesRouter(t, f)
			response := preferenceRequest(router, "PUT", "sidebar", `{"value":{"open":true}}`, tc.credential, tc.peer, tc.identity)
			require.Equal(t, 200, response.Code, response.Body.String())
			document, err := f.users.UIPreferences(t.Context(), tc.owner)
			require.NoError(t, err)
			require.Equal(t, true, document.Sections["sidebar"]["open"])
			response = preferenceRequest(router, "GET", "", "", tc.credential, tc.peer, tc.identity)
			require.Equal(t, 200, response.Code)
			require.Contains(t, response.Body.String(), `"sidebar":{"open":true}`)
		})
	}
	f.cfg.AuthMode = "local"
	require.NoError(t, f.db.Model(&environment.Environment{}).Where("id='example'").Update("access_token", "example-host-token").Error)
	router := preferencesRouter(t, f)
	for _, tc := range []struct {
		section, body, code string
		status              int
	}{{"unknown", `{"value":{}}`, "preferences.unknown-section", 400}, {"views", `{"value":[]}`, "preferences.invalid", 400}, {"views", `{"value":{"large":"` + strings.Repeat("x", 16*1024) + `"}}`, "preferences.too-large", 413}, {"views", `{invalid`, "preferences.invalid", 400}} {
		r := preferenceRequest(router, "PUT", tc.section, tc.body, "Bearer "+tokens.AccessToken, "198.51.100.5:5000", "")
		require.Equal(t, tc.status, r.Code, r.Body.String())
		var problem map[string]any
		require.NoError(t, json.Unmarshal(r.Body.Bytes(), &problem))
		require.Equal(t, tc.code, problem["code"])
	}
	transport := preferenceRequest(router, "GET", "", "", "example-host-token", "198.51.100.5:5000", "")
	require.Equal(t, 403, transport.Code, transport.Body.String())
	require.Contains(t, transport.Body.String(), "preferences.no-user")
	require.NoError(t, f.db.Table("users").Where("id=?", viewer.ID).Update("ui_preferences", `{"sections":[]}`).Error)
	malformed := preferenceRequest(router, "GET", "", "", "Bearer "+tokens.AccessToken, "198.51.100.5:5000", "")
	require.Equal(t, 200, malformed.Code, malformed.Body.String())
	require.Contains(t, malformed.Body.String(), `"sections":{}`)
	anonymous := preferenceRequest(router, "GET", "", "", "", "198.51.100.5:5000", "")
	require.Equal(t, 401, anonymous.Code)
	// Every request chooses its owner from authentication; another-user paths do not exist.
	owner, err := f.users.UIPreferences(context.Background(), f.adminID)
	require.NoError(t, err)
	require.NotContains(t, owner.Sections, "views")
}
