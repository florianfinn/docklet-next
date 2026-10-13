// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package backup

import (
	"cmp"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/samber/mo"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/upload"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
)

type Handler struct {
	service            *Service
	activityService    *activity.ActivityService
	environmentService *environment.EnvironmentService
	uploadService      *upload.UploadService
	appCtx             context.Context
}

type ListBackupsInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	VolumeName    string `path:"volumeName" doc:"Volume name"`
	Search        string `query:"search" doc:"Search query"`
	Sort          string `query:"sort" doc:"Column to sort by"`
	Order         string `query:"order" default:"asc" doc:"Sort direction"`
	Start         int    `query:"start" default:"0" doc:"Start index"`
	Limit         int    `query:"limit" default:"20" doc:"Limit"`
	Type          string `query:"type" doc:"Management origin filter"`
}

type VolumeBackupPaginatedResponse struct {
	Success    bool                    `json:"success"`
	Data       []volume.Backup         `json:"data"`
	Pagination base.PaginationResponse `json:"pagination"`
	Warnings   []string                `json:"warnings,omitempty"`
}

type ListBackupsOutput struct {
	Body VolumeBackupPaginatedResponse
}

type CreateBackupInput struct {
	EnvironmentID string                      `path:"id" doc:"Environment ID"`
	VolumeName    string                      `path:"volumeName" doc:"Volume name"`
	Body          *volume.CreateBackupRequest `json:"body,omitempty"`
}

type DiscoverVolumeBackupsInput struct {
	EnvironmentID string                        `path:"id" doc:"Environment ID"`
	Body          volume.DiscoverBackupsRequest `doc:"Destination to scan"`
}

type DiscoverVolumeBackupsOutput struct {
	Body base.ApiResponse[volume.DiscoverBackupsResponse]
}

type DeleteBackupInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	BackupID      string `path:"backupId" doc:"Backup ID"`
}

type DownloadBackupInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	BackupID      string `path:"backupId" doc:"Backup ID"`
}

type UploadBackupInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	BackupID      string `path:"backupId" doc:"Backup ID"`
	Body          volume.UploadBackupRequest
}

func NewHandler(
	service *Service,
	activityService *activity.ActivityService,
	environmentService *environment.EnvironmentService,
	uploadService *upload.UploadService,
	appCtx context.Context,
) *Handler {
	return &Handler{service: service, activityService: activityService, environmentService: environmentService, uploadService: uploadService, appCtx: appCtx}
}

func (h *Handler) DownloadBackup(ctx context.Context, input *DownloadBackupInput) (*huma.StreamResponse, error) {
	user, _ := userctx.CurrentUserFromContext(ctx)
	reader, size, err := h.service.DownloadBackup(ctx, input.BackupID, user)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}

	return &huma.StreamResponse{
		Body: func(humaCtx huma.Context) {
			defer func() { _ = reader.Close() }()

			humaCtx.SetHeader("Content-Type", "application/x-gzip")
			humaCtx.SetHeader("Content-Disposition", "attachment; filename="+input.BackupID+".tar.gz")
			humaCtx.SetHeader("Content-Length", strconv.FormatInt(size, 10))

			writer := humaCtx.BodyWriter()
			_, _ = io.Copy(writer, reader)
		},
	}, nil
}

func (h *Handler) ListBackups(ctx context.Context, input *ListBackupsInput) (*ListBackupsOutput, error) {
	params := pagination.QueryParams{
		Search: input.Search,
		Sort:   input.Sort,
		Order:  pagination.SortOrder(input.Order),
		Start:  input.Start,
		Limit:  input.Limit,
		Filters: map[string]string{
			"type": input.Type,
		},
	}

	params.Limit = cmp.Or(params.Limit, 20)

	backups, paginationResp, err := h.service.List(ctx, input.VolumeName, params)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}

	warning := h.service.backupMountWarningInternal(ctx)

	return &ListBackupsOutput{
		Body: VolumeBackupPaginatedResponse{
			Success:    true,
			Data:       backups,
			Pagination: handlerutil.PaginationResponse(paginationResp),
			Warnings: func() []string {
				if warning == "" {
					return nil
				}
				return []string{warning}
			}(),
		},
	}, nil
}

func (h *Handler) CreateBackup(ctx context.Context, input *CreateBackupInput) (*handlerutil.Out[volume.BackupEntry], error) {
	user, err := handlerutil.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	request := volume.CreateBackupRequest{}
	if input.Body != nil {
		request = *input.Body
	}
	entry, err := h.service.StartBackup(utils.ActivityRuntimeContext(ctx, h.appCtx), input.EnvironmentID, input.VolumeName, *user, request)
	if errors.Is(err, h.service.deps.AlreadyRunning) {
		return nil, huma.Error409Conflict(err.Error())
	}
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	return &handlerutil.Out[volume.BackupEntry]{
		Body: base.ApiResponse[volume.BackupEntry]{Success: true, Data: entry},
	}, nil
}

func (h *Handler) DeleteBackup(ctx context.Context, input *DeleteBackupInput) (*handlerutil.Out[base.MessageResponse], error) {
	user, _ := userctx.CurrentUserFromContext(ctx)
	runtimeCtx := utils.ActivityRuntimeContext(ctx, h.appCtx)
	activityID, err := activitylib.RunHandlerActivity(runtimeCtx, h.activityService, activitylib.HandlerOptions{
		EnvironmentID:  input.EnvironmentID,
		Type:           activitytypes.TypeResourceAction,
		ResourceType:   "volume_backup",
		ResourceID:     input.BackupID,
		ResourceName:   input.BackupID,
		User:           user,
		Step:           "Deleting backup",
		Message:        "Deleting volume backup",
		SuccessMessage: "Backup deleted successfully",
		Metadata: database.JSON{
			"action":   "delete_volume_backup",
			"backupId": input.BackupID,
		},
	}, func(runtimeCtx context.Context) error {
		return h.service.DeleteBackup(runtimeCtx, input.BackupID, user)
	})
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	return &handlerutil.Out[base.MessageResponse]{
		Body: base.ApiResponse[base.MessageResponse]{
			Success: true,
			Data:    base.MessageResponse{Message: "Backup deleted successfully", ActivityID: mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer()},
		},
	}, nil
}

func (h *Handler) DiscoverBackups(ctx context.Context, input *DiscoverVolumeBackupsInput) (*DiscoverVolumeBackupsOutput, error) {
	if _, err := handlerutil.RequireUser(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Body.S3DestinationID) == "" {
		return nil, huma.Error400BadRequest("select an S3 destination to discover")
	}
	created, failures, err := h.service.DiscoverRemoteBackups(ctx, input.Body.S3DestinationID)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	return &DiscoverVolumeBackupsOutput{
		Body: base.ApiResponse[volume.DiscoverBackupsResponse]{
			Success: true,
			Data:    volume.DiscoverBackupsResponse{Count: created, Errors: failures},
		},
	}, nil
}

func (h *Handler) UploadBackup(ctx context.Context, input *UploadBackupInput) (*handlerutil.Out[*volume.Backup], error) {
	if h.service == nil {
		return nil, huma.Error500InternalServerError("service not available")
	}
	user, _ := userctx.CurrentUserFromContext(ctx)
	var uploaded *volume.Backup
	runtimeCtx := utils.ActivityRuntimeContext(ctx, h.appCtx)
	activityID, err := activitylib.RunHandlerActivity(runtimeCtx, h.activityService, activitylib.HandlerOptions{
		EnvironmentID:  input.EnvironmentID,
		Type:           activitytypes.TypeResourceAction,
		ResourceType:   "volume_backup",
		ResourceID:     input.BackupID,
		ResourceName:   input.BackupID,
		User:           user,
		Step:           "Uploading backup",
		Message:        "Uploading volume backup to S3",
		SuccessMessage: "Volume backup uploaded successfully",
		Metadata: database.JSON{
			"action":          "upload_volume_backup",
			"backupId":        input.BackupID,
			"s3DestinationId": input.Body.S3DestinationID,
		},
	}, func(runtimeCtx context.Context) error {
		var uploadErr error
		uploaded, uploadErr = h.service.UploadBackup(runtimeCtx, input.BackupID, input.Body.S3DestinationID)
		return uploadErr
	})
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	uploaded.ActivityID = mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer()
	return &handlerutil.Out[*volume.Backup]{Body: base.ApiResponse[*volume.Backup]{Success: true, Data: uploaded}}, nil
}
