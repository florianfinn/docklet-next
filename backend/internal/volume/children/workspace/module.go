// Source: getarcaneapp/arcane/backend/internal/volume/children/workspace/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package workspace

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterRoutes registers the volume workspace endpoints.
func RegisterRoutes(api huma.API, h *Handler) {
	basePath := "/environments/{id}/volumes/{volumeName}/workspace"
	tag := "Volume Workspace"
	handlerutil.RegisterSecured(api, handlerutil.Operation("get-volume-workspace", http.MethodGet, basePath, "Get volume workspace", "", tag), authz.PermVolumesRead, h.GetVolumeWorkspace)
	handlerutil.RegisterSecured(
		api,
		handlerutil.Operation(
			"get-volume-workspace-file",
			http.MethodGet,
			basePath+"/file",
			"Get volume workspace file",
			"",
			tag,
		),
		authz.PermVolumesRead,
		h.GetVolumeWorkspaceFile,
	)
	handlerutil.RegisterSecured(
		api,
		handlerutil.Operation(
			"download-volume-workspace-file",
			http.MethodGet,
			basePath+"/file/download",
			"Download volume workspace file",
			"",
			tag,
		),
		authz.PermVolumesRead,
		h.DownloadVolumeWorkspaceFile,
	)
	updateOperation := handlerutil.Operation("update-volume-workspace", http.MethodPut, basePath, "Update volume workspace", "", tag)
	updateOperation.RequestBody = handlerutil.WorkspaceMultipartRequestBody("JSON encoded volume workspace manifest")
	handlerutil.RegisterSecured(api, updateOperation, authz.PermVolumesRead, h.UpdateVolumeWorkspace)
}
