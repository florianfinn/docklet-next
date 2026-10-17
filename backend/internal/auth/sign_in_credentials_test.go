// Source: docklet-next/issues/68
// Revision: af5fa9df01e730f7691d650985abd33d74f51ae5
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original local credential exceptions and shipped CLI integration regression tests.

package auth

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	apikeytypes "github.com/florianfinn/docklet-next/types/apikey"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestSignInHeaderCredentialWrites(t *testing.T) {
	svc, tokens, db := modeTestFixture(t)
	admin, err := svc.userService.GlobalAdministrator(t.Context())
	require.NoError(t, err)
	keys := apikey.NewApiKeyService(db, svc.userService, role.NewRoleService(db))
	personal, err := keys.CreatePersonalApiKey(t.Context(), admin.ID, apikeytypes.CreateUserApiKey{Name: "example"})
	require.NoError(t, err)
	scoped, err := keys.CreateApiKey(t.Context(), admin.ID, authz.SudoPermissionSet(), apikeytypes.CreateApiKey{Name: "scoped", Permissions: []apikeytypes.PermissionGrant{{Permission: "containers:list"}}})
	require.NoError(t, err)
	expired, err := keys.CreatePersonalApiKey(t.Context(), admin.ID, apikeytypes.CreateUserApiKey{Name: "expired", ExpiresAt: new(time.Now().Add(-time.Hour))})
	require.NoError(t, err)
	expiredBearer := makeAccessToken(t, svc.signingKey, "access", admin.ID, "admin", nil, "", "", time.Now().Add(-time.Hour))
	for _, mode := range []string{"none", "local", "proxy"} {
		svc.config.AuthMode = mode
		for _, surface := range []string{"echo", "huma"} {
			router := modeTestRouter(t, svc, surface, true, keys)
			for _, tc := range []struct {
				name, header, value string
				localStatus         int
			}{
				{"bearer", "Authorization", "Bearer " + tokens.AccessToken, 200},
				{"personal", middleware.HeaderApiKey, personal.Key, 200},
				{"scoped", middleware.HeaderApiKey, scoped.Key, 200},
				{"invalid bearer", "Authorization", "Bearer invalid", 401},
				{"expired bearer", "Authorization", "Bearer " + expiredBearer, 401},
				{"invalid key", middleware.HeaderApiKey, "invalid", 401},
				{"expired key", middleware.HeaderApiKey, expired.Key, 401},
				{"cookie only", "", "", 403},
			} {
				t.Run(mode+"/"+surface+"/"+tc.name, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodPost, "/api/secure", http.NoBody)
					req.RemoteAddr = "192.0.2.10:5000"
					req.Header.Set("Remote-User", "operator")
					if tc.header == "" {
						req.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: tokens.BrowserToken})
					} else {
						req.Header.Set(tc.header, tc.value)
					}
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					want := 403
					if mode == "local" {
						want = tc.localStatus
					}
					require.Equal(t, want, rec.Code, rec.Body.String())
				})
			}
		}
	}
}

func TestSignInCLIThroughRealMiddleware(t *testing.T) {
	svc, _, db := modeTestFixture(t)
	keys := apikey.NewApiKeyService(db, svc.userService, role.NewRoleService(db))
	router := echo.New()
	router.Use(middleware.NewCORSMiddleware(svc.config).Add())
	router.Use(middleware.NewCSRFMiddleware(svc.config).WithLocalCredentialValidator(
		NewAuthMiddleware(svc, svc.config).WithApiKeyValidator(keys).ValidateLocalHeaderCredential).Add())
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
	api.UseMiddleware(NewHumaMiddleware(api, svc, keys, svc.userServiceRoleResolver(), nil, svc.config))
	RegisterAuth(api, svc.userService, svc, svc.settingsService)
	var mu sync.Mutex
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path)
		mu.Unlock()
		if r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" || r.Header.Get(middleware.CSRFHeader) != "" {
			t.Error("CLI unexpectedly supplied browser protection headers")
		}
		router.ServeHTTP(w, r)
	}))
	defer server.Close()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "cli.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("server_url: "+server.URL+"\n"), 0o600))
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	binary := filepath.Join(dir, "cli")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../../cli")
	output, err := build.CombinedOutput()
	require.NoError(t, err, "%s", output)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, append([]string{"--config", configPath, "--no-color"}, args...)...)
		cmd.Stdin = strings.NewReader("example-password\n")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
	run("auth", "login", "--username", "admin")
	invalidateAccess := func() {
		t.Helper()
		configBytes, err := os.ReadFile(configPath)
		require.NoError(t, err)
		lines := strings.Split(string(configBytes), "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "jwt_token:") {
				lines[i] = "jwt_token: example"
			}
		}
		require.NoError(t, os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0o600))
	}
	invalidateAccess()
	run("auth", "refresh")
	invalidateAccess()
	run("auth", "logout")
	admin, err := svc.userService.GlobalAdministrator(t.Context())
	require.NoError(t, err)
	key, err := keys.CreatePersonalApiKey(t.Context(), admin.ID, apikeytypes.CreateUserApiKey{Name: "cli"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, []byte("server_url: "+server.URL+"\napi_key: "+key.Key+"\n"), 0o600))
	run("auth", "logout")
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"/api/auth/login", "/api/auth/refresh", "/api/auth/logout", "/api/auth/refresh", "/api/auth/logout", "/api/auth/logout"}, seen)
}

func TestSignInBodyCredentialBoundary(t *testing.T) {
	for _, mode := range []string{"local", "none", "proxy"} {
		for _, path := range []string{"/api/auth/login", "/api/auth/refresh"} {
			for _, tc := range []struct {
				name, origin, cookie string
				method               string
				localStatus          int
			}{
				{"explicit body", "", "", http.MethodPost, 200},
				{"ambient cookie", "", "example=value", http.MethodPost, 403},
				{"browser origin", "https://hub.example.com", "", http.MethodPost, 403},
				{"foreign origin", "https://other.example.com", "", http.MethodPost, 403},
				{"wrong method", "", "", http.MethodPut, 403},
			} {
				t.Run(mode+path+"/"+tc.name, func(t *testing.T) {
					router := echo.New()
					router.Use(middleware.NewCSRFMiddleware(&config.Config{AuthMode: mode, AppUrl: "https://hub.example.com", Environment: "production"}).Add())
					router.Any(path, func(c *echo.Context) error { return c.NoContent(200) })
					req := httptest.NewRequest(tc.method, path, http.NoBody)
					req.Header.Set("Origin", tc.origin)
					if tc.cookie != "" {
						req.Header.Set("Cookie", tc.cookie)
					}
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					want := 403
					if mode == "local" {
						want = tc.localStatus
					}
					require.Equal(t, want, rec.Code)
				})
			}
		}
	}
}

func TestSignInBearerHeaderSelectsCredential(t *testing.T) {
	svc, adminTokens := modeTestService(t)
	member, err := svc.userService.CreateUser(t.Context(), &user.User{Username: "operator"})
	require.NoError(t, err)
	tokens, err := svc.CompleteLogin(t.Context(), member, authtypes.SessionMeta{}, session.UserSessionSourceLocal)
	require.NoError(t, err)
	for _, surface := range []string{"echo", "huma"} {
		for _, scheme := range []string{"Bearer", "bearer", "bEaReR"} {
			t.Run(surface+"/"+scheme, func(t *testing.T) {
				router := modeTestRouter(t, svc, surface, true)
				req := httptest.NewRequest(http.MethodGet, "/api/secure", http.NoBody)
				req.Header.Set("Authorization", scheme+" "+tokens.AccessToken)
				req.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: adminTokens.BrowserToken})
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				require.Equal(t, 200, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), member.ID+":operator")
			})
		}
	}
}

func TestSignInCookieAuthorityRequiresCSRF(t *testing.T) {
	svc, tokens, db := modeTestFixture(t)
	admin, err := svc.userService.GlobalAdministrator(t.Context())
	require.NoError(t, err)
	keys := apikey.NewApiKeyService(db, svc.userService, role.NewRoleService(db))
	key, err := keys.CreateApiKey(t.Context(), admin.ID, authz.SudoPermissionSet(), apikeytypes.CreateApiKey{Name: "scoped", Permissions: []apikeytypes.PermissionGrant{{Permission: "containers:list"}}})
	require.NoError(t, err)
	router := echo.New()
	router.Use(middleware.NewCSRFMiddleware(svc.config).WithLocalCredentialValidator(
		NewAuthMiddleware(svc, svc.config).WithApiKeyValidator(keys).ValidateLocalHeaderCredential).Add())
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
	api.UseMiddleware(NewHumaMiddleware(api, svc, keys, svc.userServiceRoleResolver(), nil, svc.config))
	RegisterAuth(api, svc.userService, svc, svc.settingsService)
	apikey.RegisterApiKeys(api, keys)
	bootstrap := httptest.NewRecorder()
	router.ServeHTTP(bootstrap, httptest.NewRequest(http.MethodGet, "/api/auth/mode", http.NoBody))
	for _, tc := range []struct {
		name, header, credential string
		cookie, csrf             bool
		status                   int
	}{
		{"ignored key with cookie", middleware.HeaderApiKey, key.Key, true, false, 403},
		{"bearer with cookie", "Authorization", "Bearer " + tokens.AccessToken, true, false, 403},
		{"key without session", middleware.HeaderApiKey, key.Key, false, false, 401},
		{"cookie with protection", middleware.HeaderApiKey, key.Key, true, true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/auth/me/api-keys", bytes.NewBufferString(`{"name":"example"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(tc.header, tc.credential)
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: tokens.BrowserToken})
			}
			if tc.csrf {
				req.Header.Set("Origin", "https://hub.example.com")
				req.Header.Set(middleware.CSRFHeader, bootstrap.Header().Get(middleware.CSRFHeader))
				req.AddCookie(bootstrap.Result().Cookies()[0])
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
		})
	}
}
