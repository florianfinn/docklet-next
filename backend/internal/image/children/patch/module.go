// Source: getarcaneapp/arcane/backend/internal/image/children/patch/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package patch

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterRoutes registers image patching routes.
func RegisterRoutes(api huma.API, imagePatchService *Service, appCtx handlerutil.ActivityAppContext) {
	h := &ImagePatchHandler{
		imagePatchService: imagePatchService,
		appCtx:            appCtx.Context(),
	}

	handlerutil.RegisterSecured(api,
		handlerutil.Operation("list-image-patches", http.MethodGet, "/environments/{id}/images/patches", "List image patches", "Retrieves the paginated image patch history for the environment", "Images"),
		authz.PermImagesList, h.ListImagePatches)
	handlerutil.RegisterSecured(api,
		handlerutil.Operation(
			"list-image-patch-targets",
			http.MethodGet,
			"/environments/{id}/images/patch-targets",
			"List image patch targets",
			"Retrieves scanned images with fixable vulnerability counts and their latest patch run",
			"Images",
		),
		authz.PermVulnsRead, h.ListPatchTargets)
	handlerutil.RegisterSecured(api,
		handlerutil.Operation(
			"patch-image",
			http.MethodPost,
			"/environments/{id}/images/{imageId}/patch",
			"Patch image",
			"Patches OS package vulnerabilities in the image using Copacetic, producing a new patched tag",
			"Images",
		),
		authz.PermImagesPatch, h.PatchImage)
}
