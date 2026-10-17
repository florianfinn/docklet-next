//go:build linux

// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package auth

import (
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	settingstypes "github.com/florianfinn/docklet-next/types/settings"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestVersionOnboardingCredentialBoundary(t *testing.T) {
	for _, mode := range []string{"hub", "agent"} {
		t.Run(mode, func(t *testing.T) {
			svc, _ := modeTestService(t)
			svc.config.AgentMode = mode == "agent"
			svc.config.AgentToken = "example"
			directory := t.TempDir()
			_, err := svc.settingsService.UpdateSettings(t.Context(), settingstypes.Update{ProjectsDirectory: &directory})
			require.NoError(t, err)
			versionService := version.NewVersionService(nil, true, "dev", "unknown", nil, nil, nil, svc.settingsService)
			router := echo.New()
			api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "1.0.0"))
			api.UseMiddleware(NewHumaMiddleware(api, svc, nil, nil, nil, svc.config))
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
						req.Header.Set(middleware.HeaderAgentBootstrap, svc.config.AgentToken)
					case "agent-token", "cached":
						req.Header.Set(middleware.HeaderAgentToken, svc.config.AgentToken)
					case "api-key":
						req.Header.Set(middleware.HeaderApiKey, svc.config.AgentToken)
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
