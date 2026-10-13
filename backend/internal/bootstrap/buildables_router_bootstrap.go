// Source: getarcaneapp/arcane/backend/internal/bootstrap/buildables_router_bootstrap.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

//go:build buildables

package bootstrap

import (
	"github.com/labstack/echo/v5"

	"github.com/florianfinn/docklet-next/backend/api"
)

func init() {
	registerBuildableRoutes = append(registerBuildableRoutes, func(apiGroup *echo.Group, deps api.HandlerDeps) {
		api.SetupBuildablesRoutes(apiGroup, deps.Auth.Service())
	})
}
