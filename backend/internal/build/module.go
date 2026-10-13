// Source: getarcaneapp/arcane/backend/internal/build/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package build

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/build/children/workspace"
	"github.com/florianfinn/docklet-next/backend/internal/upload"
)

// RegisterBuildWorkspaces registers the build workspace file browser routes.
func RegisterBuildWorkspaces(api huma.API, buildService *BuildService, uploadService *upload.UploadService) {
	var workspaceService *workspace.Service
	if buildService != nil {
		workspaceService = buildService.workspace
	}
	workspace.Register(api, workspaceService, uploadService)
}
