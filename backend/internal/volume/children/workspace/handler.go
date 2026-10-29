// Source: getarcaneapp/arcane/backend/internal/volume/children/workspace/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; Typed helper incompatibility responses; M12 persisted volume lifecycle and legacy restore cut.

package workspace

import (
	"context"
	"errors"
	"mime/multipart"
	"path"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/florianfinn/docklet-next/types/workspace"
	"github.com/samber/mo"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	workspacepkg "github.com/florianfinn/docklet-next/backend/pkg/workspace"
)

type Handler struct {
	service         *Service
	activityService *activity.ActivityService
	appCtx          context.Context
}

type GetVolumeWorkspaceInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	VolumeName    string `path:"volumeName" doc:"Volume name"`
}

type GetVolumeWorkspaceFileInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	VolumeName    string `path:"volumeName" doc:"Volume name"`
	RelativePath  string `query:"relativePath" doc:"Path relative to the volume workspace root"`
}

type UpdateVolumeWorkspaceInput struct {
	EnvironmentID string         `path:"id" doc:"Environment ID"`
	VolumeName    string         `path:"volumeName" doc:"Volume name"`
	RawBody       multipart.Form `contentType:"multipart/form-data"`
}

func NewHandler(service *Service, activityService *activity.ActivityService, appCtx context.Context) *Handler {
	return &Handler{service: service, activityService: activityService, appCtx: appCtx}
}

func volumeWorkspaceHTTPErrorInternal(err error) error {
	var unavailable *volume.HelperUnavailableError
	if errors.As(err, &unavailable) {
		return unavailable
	}
	var incompatible *volume.HelperIncompatibleError
	if errors.As(err, &incompatible) {
		return incompatible
	}
	switch {
	case errors.Is(err, common.ErrVolumeWorkspaceConflict):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, common.ErrVolumeWorkspaceForbidden):
		return huma.Error403Forbidden(err.Error())
	case errors.Is(err, common.ErrVolumeWorkspaceNotFound):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, common.ErrVolumeWorkspaceBadRequest):
		return huma.Error400BadRequest(err.Error())
	default:
		return huma.Error500InternalServerError("internal error")
	}
}

func (h *Handler) GetVolumeWorkspace(ctx context.Context, input *GetVolumeWorkspaceInput) (*handlerutil.Out[workspace.Workspace], error) {
	result, err := h.service.GetVolumeWorkspace(ctx, input.VolumeName)
	if err != nil {
		return nil, volumeWorkspaceHTTPErrorInternal(err)
	}
	return &handlerutil.Out[workspace.Workspace]{Body: base.ApiResponse[workspace.Workspace]{Success: true, Data: *result}}, nil
}

func (h *Handler) GetVolumeWorkspaceFile(ctx context.Context, input *GetVolumeWorkspaceFileInput) (*handlerutil.Out[workspace.FileContent], error) {
	result, err := h.service.GetVolumeWorkspaceFile(ctx, input.VolumeName, input.RelativePath)
	if err != nil {
		return nil, volumeWorkspaceHTTPErrorInternal(err)
	}
	return &handlerutil.Out[workspace.FileContent]{Body: base.ApiResponse[workspace.FileContent]{Success: true, Data: *result}}, nil
}

func (h *Handler) DownloadVolumeWorkspaceFile(ctx context.Context, input *GetVolumeWorkspaceFileInput) (*huma.StreamResponse, error) {
	reader, size, err := h.service.DownloadVolumeWorkspaceFile(ctx, input.VolumeName, input.RelativePath)
	if err != nil {
		return nil, volumeWorkspaceHTTPErrorInternal(err)
	}
	return handlerutil.DownloadResponse(reader, size, path.Base(input.RelativePath)), nil
}

func requireVolumeWorkspacePermissionsInternal(ctx context.Context, environmentID string, changes []volume.WorkspaceFileChange) error {
	permissions, ok := middleware.PermissionsFromContext(ctx)
	if !ok || permissions == nil {
		return huma.Error403Forbidden("insufficient permissions")
	}
	required, valid := authz.VolumeWorkspaceRequiredPermissions(changes)
	if !valid {
		return huma.Error422UnprocessableEntity("unsupported volume workspace operation")
	}
	for _, permission := range required {
		if !permissions.Allows(permission, environmentID) {
			return huma.Error403Forbidden("insufficient permissions for volume workspace operation")
		}
	}
	return nil
}

func (h *Handler) UpdateVolumeWorkspace(ctx context.Context, input *UpdateVolumeWorkspaceInput) (*handlerutil.Out[workspace.Workspace], error) {
	manifest, err := handlerutil.ParseMultipartJSONPart[volume.WorkspaceUpdateManifest](input.RawBody, "manifest")
	if err != nil {
		return nil, err
	}
	if requireVolumeWorkspacePermissionsErr := requireVolumeWorkspacePermissionsInternal(ctx, input.EnvironmentID, manifest.FileChanges); requireVolumeWorkspacePermissionsErr != nil {
		return nil, requireVolumeWorkspacePermissionsErr
	}
	maxFileSizeBytes := workspacepkg.MaxFileSizeBytes(workspacepkg.DefaultMaxFileSizeMB)
	if h.service != nil {
		maxFileSizeBytes = h.service.MaxFileSizeBytes()
	}
	uploads, err := handlerutil.ReadWorkspaceUploads(input.RawBody, maxFileSizeBytes)
	if err != nil {
		return nil, err
	}
	user, err := handlerutil.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	var result *workspace.Workspace
	runtimeCtx := utils.ActivityRuntimeContext(ctx, h.appCtx)
	activityID, err := activitylib.RunHandlerActivity(runtimeCtx, h.activityService, activitylib.HandlerOptions{
		EnvironmentID: input.EnvironmentID, Type: activitytypes.TypeResourceAction, ResourceType: "volume", ResourceID: input.VolumeName, ResourceName: input.VolumeName, User: user,
		Step: "Updating volume workspace", Message: "Updating volume workspace", SuccessMessage: "Volume workspace updated successfully",
		Metadata: database.JSON{"action": "update_volume_workspace", "fileChangeCount": len(manifest.FileChanges)},
	}, func(runtimeCtx context.Context) error {
		var updateErr error
		result, updateErr = h.service.UpdateVolumeWorkspace(runtimeCtx, input.VolumeName, manifest, uploads, *user)
		return updateErr
	})
	if err != nil {
		return nil, volumeWorkspaceHTTPErrorInternal(err)
	}
	result.ActivityID = mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer()
	return &handlerutil.Out[workspace.Workspace]{Body: base.ApiResponse[workspace.Workspace]{Success: true, Data: *result}}, nil
}
