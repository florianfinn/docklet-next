// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package bootstrap

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyLogoutKeepsOriginAndCSRFChecks(t *testing.T) {
	f := newSignInRoutingFixture(t)
	f.cfg.AuthMode = "proxy"
	router := echo.New()
	router.Use(middleware.NewCSRFMiddleware(f.cfg).Add())
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("logout", "test"))
	api.UseMiddleware(auth.NewHumaMiddleware(api, f.svc, f.keys, f.roles, nil, f.cfg))
	auth.RegisterAuth(api, f.users, f.svc, nil)
	bootstrap := httptest.NewRecorder()
	router.ServeHTTP(bootstrap, httptest.NewRequest("GET", "/api/auth/mode", nil))
	require.Equal(t, 200, bootstrap.Code)
	cookies := bootstrap.Result().Cookies()
	token := bootstrap.Header().Get(middleware.CSRFHeader)
	require.NotEmpty(t, token)
	for _, tc := range []struct {
		origin string
		csrf   bool
		status int
	}{{"", true, 403}, {"https://other.example.com", true, 403}, {f.cfg.AppUrl, false, 403}, {f.cfg.AppUrl, true, 200}} {
		r := httptest.NewRequest("POST", "/api/auth/logout", http.NoBody)
		r.Header.Set("Origin", tc.origin)
		if tc.csrf {
			r.Header.Set(middleware.CSRFHeader, token)
		}
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		require.Equal(t, tc.status, w.Code, w.Body.String())
		if tc.status == 200 {
			cleared := false
			for _, c := range w.Result().Cookies() {
				if c.Name == "docklet_csrf" || c.Name == "__Host-docklet_csrf" {
					cleared = c.MaxAge < 0
				}
			}
			require.True(t, cleared)
		}
	}
}
