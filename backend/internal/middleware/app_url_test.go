// Source: docklet-next/issues/68
// Revision: af5fa9df01e730f7691d650985abd33d74f51ae5
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original normalized APP_URL contract across CORS, CSRF and WebSocket gates.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestSignInCanonicalBrowserOrigins(t *testing.T) {
	for _, tc := range []struct{ appURL, origin string }{
		{"HTTPS://Hub.example.com:443", "https://hub.example.com"},
		{"http://Hub.example.com:80", "http://hub.example.com"},
		{"https://Hub.example.com:8443", "https://hub.example.com:8443"},
		{"https://[::ffff:c000:201]", "https://[::ffff:c000:201]"},
	} {
		t.Run(tc.appURL, func(t *testing.T) {
			cfg := &config.Config{AuthMode: "none", Environment: "production", AppUrl: tc.appURL}
			require.NoError(t, cfg.ValidateSignIn())
			router := echo.New()
			router.Use(NewCORSMiddleware(cfg).Add(), NewCSRFMiddleware(cfg).Add())
			handler := func(c *echo.Context) error { return c.NoContent(200) }
			router.GET("/api/test", handler)
			router.POST("/api/test", handler)
			bootstrap := httptest.NewRecorder()
			router.ServeHTTP(bootstrap, httptest.NewRequest(http.MethodGet, "/api/test", http.NoBody))
			req := httptest.NewRequest(http.MethodPost, "/api/test", http.NoBody)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set(CSRFHeader, bootstrap.Header().Get(CSRFHeader))
			req.AddCookie(bootstrap.Result().Cookies()[0])
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.Equal(t, tc.origin, rec.Header().Get("Access-Control-Allow-Origin"))
			require.True(t, WebSocketOriginValidator(cfg, nil)(req))
			req.Header.Set("Origin", "https://other.example.com")
			require.False(t, WebSocketOriginValidator(cfg, nil)(req))
		})
	}
}
