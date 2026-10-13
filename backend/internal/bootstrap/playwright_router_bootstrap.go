// Source: getarcaneapp/arcane/backend/internal/bootstrap/playwright_router_bootstrap.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

//go:build playwright

package bootstrap

import (
	"context"
	"log/slog"

	"github.com/labstack/echo/v5"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/playwright"
)

func init() {
	ctx := context.Background() //nolint:forbidigo // Package init has no request context.
	registerPlaywrightRoutes = []func(apiGroup *echo.Group, deps api.HandlerDeps){
		func(apiGroup *echo.Group, deps api.HandlerDeps) {
			apiKeyService := deps.ApiKey.Service()
			userService := deps.User.Service()
			if apiKeyService == nil || userService == nil || deps.Federated == nil || deps.GitRepository.Service() == nil || deps.GitOpsSync.Service() == nil {
				slog.WarnContext(ctx, "Playwright service not available, skipping playwright routes")
				return
			}

			playwrightService := playwright.NewPlaywrightService(apiKeyService, userService, deps.GitRepository.Service(), deps.GitOpsSync.Service(), deps.Project.Service())
			playwright.SetupRoutes(apiGroup, playwrightService, deps.Federated)
			slog.InfoContext(ctx, "Playwright routes registered for E2E testing")
		},
	}
}
