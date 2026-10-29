// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original public read-only mode/bootstrap API tests.

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/health"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestSignInModeReadOnlyAPI(t *testing.T) {
	svc, _ := modeTestService(t)
	for _, mode := range []string{"none", "local", "proxy"} {
		t.Run(mode, func(t *testing.T) {
			svc.config.AuthMode = mode
			router := echo.New()
			router.Use(middleware.NewCSRFMiddleware(svc.config).Add())
			api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
			api.UseMiddleware(NewHumaMiddleware(api, svc, nil, nil, nil, svc.config))
			RegisterAuth(api, svc.userService, svc, svc.settingsService)
			health.RegisterRoutes(api, nil)
			huma.Register(api, huma.Operation{OperationID: "test-pairing", Method: http.MethodPost, Path: "/environments/pair", Security: []map[string][]string{}}, func(_ context.Context, input *struct {
				Key string `header:"X-API-Key"`
			}) (*struct{ Body string }, error) {
				if input.Key != "example" {
					return nil, huma.Error401Unauthorized("Invalid machine key")
				}
				return &struct{ Body string }{Body: "paired"}, nil
			})
			req := httptest.NewRequest(http.MethodGet, "/api/auth/mode", http.NoBody)
			req.RemoteAddr = "198.51.100.5:5000"
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), `"mode":"`+mode+`"`)
			require.Len(t, rec.Header().Get(middleware.CSRFHeader), 43)
			require.Len(t, rec.Result().Cookies(), 1)
			require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			operation := api.OpenAPI().Paths["/auth/mode"]
			require.NotNil(t, operation.Get)
			require.Nil(t, operation.Put)
			require.Nil(t, operation.Post)
			require.Contains(t, operation.Get.Responses["200"].Headers, middleware.CSRFHeader)
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				result := httptest.NewRecorder()
				router.ServeHTTP(result, httptest.NewRequest(method, "/api/health", http.NoBody))
				require.Equal(t, 200, result.Code)
			}
			for _, key := range []string{"invalid", "example"} {
				pair := httptest.NewRequest(http.MethodPost, "/api/environments/pair", http.NoBody)
				pair.Header.Set("X-API-Key", key)
				pair.Header.Set("Origin", svc.config.AppUrl)
				pair.Header.Set(middleware.CSRFHeader, rec.Header().Get(middleware.CSRFHeader))
				pair.AddCookie(rec.Result().Cookies()[0])
				result := httptest.NewRecorder()
				router.ServeHTTP(result, pair)
				expected := 401
				if key == "example" {
					expected = 200
				}
				require.Equal(t, expected, result.Code, result.Body.String())
			}
		})
	}
}
