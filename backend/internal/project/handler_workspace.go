// Source: getarcaneapp/arcane/backend/internal/project/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Split workspace/update error handlers; typed project volume name clashes.

package project

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/workspace"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/project"
	"github.com/florianfinn/docklet-next/types/volume"
	workspacetypes "github.com/florianfinn/docklet-next/types/workspace"
	"github.com/samber/mo"
)

type UpdateProjectInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	ProjectID     string `path:"projectId" doc:"Project ID"`
	Body          project.UpdateProject
}

func projectUpdateHTTPErrorInternal(err error) error {
	if _, ok := errors.AsType[*volume.ProjectVolumeRenameConflictError](err); ok {
		return &volume.ExistsError{Status: http.StatusConflict, Title: "Conflict", Detail: "A volume with this name already exists", Code: "volume.exists"}
	}
	if inUseErr, ok := errors.AsType[*volume.ProjectVolumeRenameInUseError](err); ok {
		return huma.Error409Conflict(inUseErr.Error())
	}
	if spaceErr, ok := errors.AsType[*volume.ProjectVolumeRenameInsufficientSpaceError](err); ok {
		return huma.NewError(http.StatusInsufficientStorage, spaceErr.Error())
	}
	return projectWorkspaceRequestHTTPErrorInternal(err)
}

// projectWorkspaceRequestHTTPErrorInternal maps workspace validation errors that
// can also surface during atomic project creation or configuration updates.
func projectWorkspaceRequestHTTPErrorInternal(err error) error {
	if errors.Is(err, common.ErrProjectWorkspaceConflict) {
		return huma.Error409Conflict(err.Error())
	}
	if errors.Is(err, common.ErrProjectWorkspaceForbidden) {
		return huma.Error403Forbidden(err.Error())
	}
	if errors.Is(err, common.ErrProjectWorkspaceBadRequest) {
		return huma.Error400BadRequest(err.Error())
	}
	return nil
}

type GetProjectWorkspaceInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	ProjectID     string `path:"projectId" doc:"Project ID"`
}

type GetProjectWorkspaceFileInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	ProjectID     string `path:"projectId" doc:"Project ID"`
	RelativePath  string `query:"relativePath" doc:"Path relative to the project workspace root"`
}

type UpdateProjectWorkspaceInput struct {
	EnvironmentID string         `path:"id" doc:"Environment ID"`
	ProjectID     string         `path:"projectId" doc:"Project ID"`
	RawBody       multipart.Form `contentType:"multipart/form-data"`
}

func projectWorkspaceHTTPErrorInternal(err error) error {
	switch {
	case errors.Is(err, common.ErrProjectWorkspaceConflict):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, common.ErrProjectArchived):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, common.ErrProjectWorkspaceForbidden):
		return huma.Error403Forbidden(err.Error())
	case errors.Is(err, common.ErrProjectWorkspaceNotFound), errors.Is(err, common.ErrProjectNotFound):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, common.ErrProjectWorkspaceBadRequest), errors.Is(err, common.ErrProjectEnvUnreadable):
		return huma.Error400BadRequest(err.Error())
	default:
		return huma.Error500InternalServerError("internal error")
	}
}

func (h *ProjectHandler) GetProjectWorkspace(ctx context.Context, input *GetProjectWorkspaceInput) (*handlerutil.Out[workspacetypes.Workspace], error) {
	result, err := h.projectService.GetProjectWorkspace(ctx, input.ProjectID)
	if err != nil {
		return nil, projectWorkspaceHTTPErrorInternal(err)
	}
	return &handlerutil.Out[workspacetypes.Workspace]{Body: base.ApiResponse[workspacetypes.Workspace]{Success: true, Data: *result}}, nil
}

func (h *ProjectHandler) GetProjectWorkspaceFile(ctx context.Context, input *GetProjectWorkspaceFileInput) (*handlerutil.Out[workspacetypes.FileContent], error) {
	result, err := h.projectService.GetProjectWorkspaceFile(ctx, input.ProjectID, input.RelativePath)
	if err != nil {
		return nil, projectWorkspaceHTTPErrorInternal(err)
	}
	return &handlerutil.Out[workspacetypes.FileContent]{Body: base.ApiResponse[workspacetypes.FileContent]{Success: true, Data: *result}}, nil
}

func (h *ProjectHandler) DownloadProjectWorkspaceFile(ctx context.Context, input *GetProjectWorkspaceFileInput) (*huma.StreamResponse, error) {
	reader, size, name, err := h.projectService.DownloadProjectWorkspaceFile(ctx, input.ProjectID, input.RelativePath)
	if err != nil {
		return nil, projectWorkspaceHTTPErrorInternal(err)
	}
	return handlerutil.DownloadResponse(reader, size, name), nil
}

func (h *ProjectHandler) UpdateProjectWorkspace(ctx context.Context, input *UpdateProjectWorkspaceInput) (*handlerutil.Out[workspacetypes.Workspace], error) {
	manifest, err := handlerutil.ParseMultipartJSONPart[project.WorkspaceUpdateManifest](input.RawBody, "manifest")
	if err != nil {
		return nil, err
	}
	maxFileSizeMB := workspace.DefaultMaxFileSizeMB
	if h.projectService != nil && h.projectService.config != nil {
		maxFileSizeMB = h.projectService.config.ProjectWorkspaceMaxFileSizeMB
	}
	uploads, err := handlerutil.ReadWorkspaceUploads(input.RawBody, workspace.MaxFileSizeBytes(maxFileSizeMB))
	if err != nil {
		return nil, err
	}
	user, err := handlerutil.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	var result *workspacetypes.Workspace
	runtimeCtx := utils.ActivityRuntimeContext(ctx, h.appCtx)
	activityID, err := activitylib.RunHandlerActivity(runtimeCtx, h.activityService, activitylib.HandlerOptions{
		EnvironmentID: input.EnvironmentID, Type: activitytypes.TypeResourceAction, ResourceType: "project", ResourceID: input.ProjectID, ResourceName: h.projectActivityNameInternal(
			runtimeCtx,
			input.ProjectID,
		), User: user,
		Step: "Updating project workspace", Message: "Updating project workspace", SuccessMessage: "Project workspace updated successfully",
		Metadata: database.JSON{"action": "update_project_workspace", "fileChangeCount": len(manifest.FileChanges)},
	}, func(runtimeCtx context.Context) error {
		var updateErr error
		result, updateErr = h.projectService.UpdateProjectWorkspace(runtimeCtx, input.ProjectID, manifest, uploads, *user)
		return updateErr
	})
	if err != nil {
		return nil, projectWorkspaceHTTPErrorInternal(err)
	}
	result.ActivityID = mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer()
	return &handlerutil.Out[workspacetypes.Workspace]{Body: base.ApiResponse[workspacetypes.Workspace]{Success: true, Data: *result}}, nil
}
