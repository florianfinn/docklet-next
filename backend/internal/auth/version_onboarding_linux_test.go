//go:build linux

// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	settingstypes "github.com/florianfinn/docklet-next/types/settings"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestVersionOnboardingCredentialBoundary(t *testing.T) {
	for _, mode := range []string{"hub", "agent"} {
		t.Run(mode, func(t *testing.T) {
			db, err := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "version.db")), database.MigrationOptions{})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			settingsService, err := settings.NewSettingsService(t.Context(), db)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, settingsService.Stop(context.WithoutCancel(t.Context()))) })
			cfg := &config.Config{AuthMode: "local", AgentMode: mode == "agent", AgentToken: "example"}
			users := user.NewUserService(db, role.NewRoleService(db), session.RevokeAllUserSessionsExceptInDB)
			svc := auth.NewAuthService(users, settingsService, nil, session.NewSessionService(db), cfg)
			directory := t.TempDir()
			_, err = settingsService.UpdateSettings(t.Context(), settingstypes.Update{ProjectsDirectory: &directory})
			require.NoError(t, err)
			versionService := version.NewVersionService(nil, true, "dev", "unknown", nil, nil, nil, settingsService)
			router := echo.New()
			api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
			api.UseMiddleware(auth.NewHumaMiddleware(api, svc, nil, nil, nil, cfg))
			version.RegisterVersion(api, versionService)
			fd, err := syscall.InotifyInit1(syscall.IN_NONBLOCK | syscall.IN_CLOEXEC)
			require.NoError(t, err)
			defer syscall.Close(fd)
			_, err = syscall.InotifyAddWatch(fd, directory, syscall.IN_CREATE)
			require.NoError(t, err)
			for _, credential := range []string{"anonymous", "invalid", "bootstrap", "agent-token", "cached", "api-key"} {
				t.Run(credential, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, "/api/app-version", nil)
					switch credential {
					case "invalid":
						req.Header.Set(middleware.HeaderAgentToken, "wrong")
					case "bootstrap":
						req.Header.Set(middleware.HeaderAgentBootstrap, cfg.AgentToken)
					case "agent-token", "cached":
						req.Header.Set(middleware.HeaderAgentToken, cfg.AgentToken)
					case "api-key":
						req.Header.Set(middleware.HeaderApiKey, cfg.AgentToken)
					}
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					require.Equal(t, http.StatusOK, rec.Code)
					require.Contains(t, rec.Body.String(), "agentContractVersion")
					require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
					allowed := mode == "agent" && (credential == "agent-token" || credential == "cached" || credential == "api-key")
					if allowed {
						require.Contains(t, rec.Body.String(), "onboardingChecks")
					} else {
						require.NotContains(t, rec.Body.String(), "onboardingChecks")
					}
					var events [4096]byte
					n, readErr := syscall.Read(fd, events[:])
					if mode == "agent" && credential == "agent-token" {
						require.NoError(t, readErr)
						require.Positive(t, n, "authenticated first read performs the write probe")
					} else {
						require.ErrorIs(t, readErr, syscall.EAGAIN, "public and cached reads must not create even transient files")
					}
				})
			}
			require.Nil(t, versionService.GetAppVersionInfo(t.Context()).OnboardingChecks, "internal hub consumers never probe")
		})
	}
}
