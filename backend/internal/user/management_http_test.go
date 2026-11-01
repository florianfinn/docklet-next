// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package user_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	domain "github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestUserManagementHTTPModeContracts(t *testing.T) {
	svc, _ := setupUserAndRoleServices(t)
	router := echo.New()
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("users", "test"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	api.UseMiddleware(middleware.AccessMiddleware(api))
	cfg := &config.Config{AuthMode: "local"}
	module := domain.New(svc, nil, nil)
	module.Config = cfg
	module.RegisterRoutes(api)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, 400, request("POST", "/users", `{"username":"local@example.com","password":"<Example-password-123!>"}`).Code)
	require.Equal(t, 400, request("POST", "/users", `{"username":"viewer"}`).Code)
	w := request("POST", "/users", `{"username":"viewer","password":"<Example-password-123!>"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	local, err := svc.GetUserByUsername(t.Context(), "viewer")
	require.NoError(t, err)
	require.True(t, local.RequiresPasswordChange)
	require.NotEmpty(t, local.PasswordHash)
	require.Equal(t, 400, request("PUT", "/users/"+local.ID, `{"username":"local@example.com"}`).Code)
	cfg.AuthMode = "proxy"
	w = request("POST", "/users", `{"username":"viewer@example.com"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	proxy, err := svc.GetUserByUsername(t.Context(), "viewer@example.com")
	require.NoError(t, err)
	require.Empty(t, proxy.PasswordHash)
	require.False(t, proxy.RequiresPasswordChange)
	require.Equal(t, 403, request("PUT", "/users/"+proxy.ID, `{"password":"<Example-password-123!>"}`).Code)
	require.Equal(t, 403, request("POST", "/users/"+proxy.ID+"/reset-password", `{"password":"<Example-password-123!>"}`).Code)
	cfg.AuthMode = "local"
	w = request("POST", "/users/"+local.ID+"/reset-password", `{"password":"<Changed-password-123!>"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	stored, err := svc.GetUserByID(t.Context(), local.ID)
	require.NoError(t, err)
	require.True(t, stored.RequiresPasswordChange)
	require.NotEqual(t, local.PasswordHash, stored.PasswordHash)
	w = request("PUT", "/users/"+local.ID+"/app-assignments", `{"assignments":[{"environmentId":"0","projectId":"project-uuid","level":"full"}]}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	w = request("GET", "/users/"+local.ID, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var response struct {
		Data struct {
			IsAdmin     bool              `json:"isAdmin"`
			Assignments []json.RawMessage `json:"assignments"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.False(t, response.Data.IsAdmin)
	require.Len(t, response.Data.Assignments, 1)
}
