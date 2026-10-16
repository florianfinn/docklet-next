// Source: getarcaneapp/arcane/backend/internal/middleware/csrf_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Test explicit Origin, double-submit CSRF and validated transport exceptions.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"
)

func TestCSRFModeMatrix(t *testing.T) {
	for _, mode := range []string{"none", "local", "proxy"} {
		router := echo.New()
		router.Use(NewCSRFMiddleware(&config.Config{AuthMode: mode, Environment: "production", AppUrl: "https://hub.example.com"}).Add())
		handler := func(c *echo.Context) error { return c.NoContent(200) }
		router.GET("/api/test", handler)
		router.POST("/api/test", handler)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/test", http.NoBody))
		require.Equal(t, 200, rec.Code)
		token := rec.Header().Get(CSRFHeader)
		require.Len(t, token, 43)
		csrfCookie := rec.Result().Cookies()[0]
		for _, tc := range []struct {
			name, origin, token string
			cookie              bool
			status              int
		}{
			{"missing origin", "", token, true, 403},
			{"wrong origin", "https://other.example.com", token, true, 403},
			{"null origin", "null", token, true, 403},
			{"missing token", "https://hub.example.com", "", true, 403},
			{"wrong token", "https://hub.example.com", "invalid", true, 403},
			{"missing cookie", "https://hub.example.com", token, false, 403},
			{"valid", "https://hub.example.com", token, true, 200},
		} {
			for _, credential := range []string{"", "Authorization", "X-API-Key", "Remote-User", "X-Arcane-Agent-Token"} {
				t.Run(mode+"/"+tc.name+"/"+credential, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodPost, "/api/test", http.NoBody)
					req.Header.Set("Origin", tc.origin)
					req.Header.Set(CSRFHeader, tc.token)
					if credential != "" {
						req.Header.Set(credential, "example")
					}
					if tc.cookie {
						req.AddCookie(csrfCookie)
					}
					result := httptest.NewRecorder()
					router.ServeHTTP(result, req)
					require.Equal(t, tc.status, result.Code, result.Body.String())
				})
			}
		}
	}
}

func TestCSRFTransportBoundary(t *testing.T) {
	for _, agent := range []bool{false, true} {
		router := echo.New()
		router.Use(NewCSRFMiddleware(&config.Config{AgentMode: agent}).WithTransportValidator(func(req *http.Request) bool { return req.Header.Get(HeaderAgentToken) == "example-transport" }).Add())
		handler := func(c *echo.Context) error { return c.NoContent(200) }
		router.POST("/api/test", handler)
		router.POST("/api/webhooks/trigger/:token", handler)
		for _, tc := range []struct {
			path, token string
			status      int
		}{
			{"/api/test", "invalid", 403},
			{"/api/test", "example-transport", 200},
			{"/api/webhooks/trigger/example", "", 200},
		} {
			req := httptest.NewRequest(http.MethodPost, tc.path, http.NoBody)
			req.Header.Set(HeaderAgentToken, tc.token)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			want := tc.status
			if agent {
				want = 200
			}
			require.Equal(t, want, rec.Code)
		}
	}
}
