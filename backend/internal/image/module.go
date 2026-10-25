// Source: getarcaneapp/arcane/backend/internal/image/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed standalone build workshop.

// Package image owns Docker image operations, metadata, attestations, patching, and routes.
package image

import (
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	uploadtypes "github.com/florianfinn/docklet-next/types/upload"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/image/children/attestations"
	"github.com/florianfinn/docklet-next/backend/internal/image/children/patch"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/upload"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

type Module struct {
	service     *ImageService
	docker      *docker.DockerClientService
	imageUpdate *imageupdate.ImageUpdateService
	settings    *settings.SettingsService
	activity    *activity.ActivityService
	upload      *upload.UploadService

	attestationsHandler *attestations.Handler
}

func New(
	service *ImageService,
	dockerService *docker.DockerClientService,
	imageUpdate *imageupdate.ImageUpdateService,
	settingsService *settings.SettingsService,
	activityService *activity.ActivityService,
	uploadService *upload.UploadService,
) *Module {
	return &Module{
		service:             service,
		docker:              dockerService,
		imageUpdate:         imageUpdate,
		settings:            settingsService,
		activity:            activityService,
		upload:              uploadService,
		attestationsHandler: attestations.NewHandler(service.newAttestationsServiceInternal()),
	}
}

func (m *Module) Service() *ImageService {
	if m == nil {
		return nil
	}
	return m.service
}

func (m *Module) RegisterRoutes(api huma.API, appCtx handlerutil.ActivityAppContext) {
	if m == nil {
		RegisterImages(api, nil, nil, nil, nil, nil, nil, appCtx)
		attestations.RegisterRoutes(api, attestations.NewHandler(nil))
		patch.RegisterRoutes(api, nil, appCtx)
		return
	}
	RegisterImages(api, m.docker, m.service, m.imageUpdate, m.settings, m.activity, m.upload, appCtx)
	attestations.RegisterRoutes(api, m.attestationsHandler)
	var patchService *patch.Service
	if m.service != nil {
		patchService = m.service.patch
	}
	patch.RegisterRoutes(api, patchService, appCtx)
}

// RegisterImages registers image management routes using Huma.
func RegisterImages(
	api huma.API,
	dockerService *docker.DockerClientService,
	imageService *ImageService,
	imageUpdateService *imageupdate.ImageUpdateService,
	settingsService *settings.SettingsService,
	activityService *activity.ActivityService,
	uploadService *upload.UploadService,
	appCtx handlerutil.ActivityAppContext,
) {
	h := &ImageHandler{
		dockerService:      dockerService,
		imageService:       imageService,
		imageUpdateService: imageUpdateService,
		settingsService:    settingsService,
		activityService:    activityService,
		uploadService:      uploadService,
		appCtx:             appCtx.Context(),
	}

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "list-images",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/images",
		Summary:     "List images",
		Description: "Get a paginated list of Docker images",
		Tags:        []string{"Images"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermImagesList, h.ListImages)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "get-image-usage-counts",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/images/counts",
		Summary:     "Get image usage counts",
		Description: "Get counts of images in use, unused, total, and total size",
		Tags:        []string{"Images"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermImagesList, h.GetImageUsageCounts)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "search-images",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/images/search",
		Summary:     "Search images",
		Description: "Search Docker Hub images",
		Tags:        []string{"Images"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermImagesRead, h.SearchImages)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "tag-image",
		Method:      http.MethodPost,
		Path:        "/environments/{id}/images/{name}/tag",
		Summary:     "Tag image",
		Description: "Add a repository tag to an image",
		Tags:        []string{"Images"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermImagesTag, h.TagImage)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "get-image-history",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/images/{name}/history",
		Summary:     "Get image history",
		Description: "Get Docker image layer history",
		Tags:        []string{"Images"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermImagesRead, h.GetImageHistory)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "export-image",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/images/{name}/export",
		Summary:     "Export image",
		Description: "Download a Docker image as a tar archive",
		Tags:        []string{"Images"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermImagesRead, h.ExportImage)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "get-image",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/images/{imageId}",
		Summary:     "Get image by ID",
		Description: "Get a Docker image by its ID",
		Tags:        []string{"Images"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermImagesRead, h.GetImage)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "remove-image",
		Method:      http.MethodDelete,
		Path:        "/environments/{id}/images/{imageId}",
		Summary:     "Remove an image",
		Description: "Remove a Docker image by ID",
		Tags:        []string{"Images"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermImagesDelete, h.RemoveImage)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "pull-image",
		Method:      http.MethodPost,
		Path:        "/environments/{id}/images/pull",
		Summary:     "Pull an image",
		Description: "Pull an image as newline-delimited JSON: activity identity, progress, then typed error or done=true. EOF alone is not success.",
		Responses: map[string]*huma.Response{"200": {
			Description: "Newline-delimited pull frames",
			Content:     map[string]*huma.MediaType{"application/x-json-stream": {Schema: api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[PullFrame](), true, "")}},
		}},
		Tags:     []string{"Images"},
		Security: handlerutil.DefaultOperationSecurity(),
	}, authz.PermImagesPull, h.PullImage)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "prune-images",
		Method:      http.MethodPost,
		Path:        "/environments/{id}/images/prune",
		Summary:     "Prune unused images",
		Description: "Remove unused Docker images",
		Tags:        []string{"Images"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermImagesPrune, h.PruneImages)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "upload-image",
		Method:      http.MethodPost,
		Path:        "/environments/{id}/images/upload",
		Summary:     "Upload an image",
		Description: "Load a Docker image tar archive from a complete chunked upload session. multipart/form-data bodies " +
			"are still accepted for backward compatibility; that form is deprecated and will be removed in a " +
			"future release.",
		Tags:        []string{"Images"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: upload.LegacyMultipartMiddleware(api, h.uploadService, uploadtypes.KindImage),
	}, authz.PermImagesUpload, h.UploadImage)
}
