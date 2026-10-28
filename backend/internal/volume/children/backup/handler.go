// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations.

package backup

import (
	"cmp"
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
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
	config             *config.Config
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

type DeleteBackupInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	BackupID      string `path:"backupId" doc:"Backup ID"`
}

type DownloadBackupInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	BackupID      string `path:"backupId" doc:"Backup ID"`
}

func NewHandler(
	service *Service,
	activityService *activity.ActivityService,
	environmentService *environment.EnvironmentService,
	uploadService *upload.UploadService,
	appCtx context.Context,
	configs ...*config.Config,
) *Handler {
	var cfg *config.Config
	if len(configs) > 0 {
		cfg = configs[0]
	}
	return &Handler{config: cfg, service: service, activityService: activityService, environmentService: environmentService, uploadService: uploadService, appCtx: appCtx}
}

func (h *Handler) DownloadBackup(ctx context.Context, input *DownloadBackupInput) (*huma.StreamResponse, error) {
	ctx = context.WithValue(ctx, edgeDownloadKey{}, h.config != nil && h.config.EdgeAgent)
	user, _ := userctx.CurrentUserFromContext(ctx)
	reader, size, err := h.service.DownloadBackup(ctx, input.BackupID, user)
	if err != nil {
		return nil, neutralBackupError(err, "volume.backup.repository-unavailable")
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
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if input.Limit > 100 {
		return nil, backuptypes.NewProblem(422, "volume.backup.listing-too-large", nil)
	}
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
		return nil, neutralBackupError(err, "volume.backup.repository-unavailable")
	}

	for i := range backups {
		if backups[i].Format != volume.BackupFormatRustic || backups[i].SnapshotUnavailableAt != nil {
			backups[i].Status = "unavailable"
			backups[i].ErrorCode = "volume.backup.snapshot-not-found"
		}
		if backups[i].Error != "" || backups[i].ErrorCode != "" {
			backups[i].Error = cmp.Or(backups[i].ErrorCode, "volume.backup.interrupted")
		}
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
	var entry volume.BackupEntry
	run := func() error {
		if input.EnvironmentID != "0" {
			if h.environmentService == nil {
				return backuptypes.NewProblem(503, "backup.target.sync-failed", nil)
			}
			response, err := handlerutil.RemoteJSONProxy(h.environmentService.ProxyJSONRequest).JSON[base.ApiResponse[volume.BackupEntry]](ctx, input.EnvironmentID, http.MethodPost, "/api/environments/0/volumes/"+url.PathEscape(input.VolumeName)+"/backups", request)
			if err != nil {
				return err
			}
			entry = response.Data
			return nil
		}
		var e error
		entry, e = h.service.StartBackup(utils.ActivityRuntimeContext(ctx, h.appCtx), input.EnvironmentID, input.VolumeName, *user, request)
		return e
	}
	target := cmp.Or(request.TargetID, string(request.Destination))
	agent := h.config != nil && (h.config.AgentMode || h.config.EdgeAgent)
	if !agent && input.EnvironmentID != "0" {
		if target == "" {
			response, e := handlerutil.RemoteJSONProxy(h.environmentService.ProxyJSONRequest).JSON[base.ApiResponse[volume.BackupPolicyCollection]](ctx, input.EnvironmentID, http.MethodGet, "/api/environments/0/volumes/"+url.PathEscape(input.VolumeName)+"/backup-policy", nil)
			if e != nil {
				return nil, e
			}
			target = "local"
			if len(response.Data.Policies) > 0 {
				target = cmp.Or(response.Data.Policies[0].TargetID, "local")
			}
			request.TargetID = target
		}
		err = h.service.withActiveTarget(ctx, target, func(fenced context.Context) error {
			ctx = fenced
			if input.EnvironmentID != "0" && h.environmentService != nil {
				if _, e := backuptarget.SyncHost(ctx, h.service.deps.DB, input.EnvironmentID, h.environmentService.ProxyJSONRequest); e != nil {
					return e
				}
			}
			return run()
		})
	} else {
		err = run()
	}
	if err != nil {
		return nil, neutralBackupError(err, "volume.backup.interrupted")
	}
	return &handlerutil.Out[volume.BackupEntry]{Body: base.ApiResponse[volume.BackupEntry]{Success: true, Data: entry}}, nil
}

func (h *Handler) DeleteBackup(ctx context.Context, input *DeleteBackupInput) (*handlerutil.Out[base.MessageResponse], error) {
	user, _ := userctx.CurrentUserFromContext(ctx)
	budget := 30 * time.Minute
	if h.config != nil && h.config.EdgeAgent {
		budget = 4 * time.Minute
	}
	runtimeCtx, cancel := context.WithTimeout(utils.ActivityRuntimeContext(ctx, h.appCtx), budget)
	defer cancel()
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
		return nil, neutralBackupError(err, "volume.backup.repository-unavailable")
	}
	return &handlerutil.Out[base.MessageResponse]{
		Body: base.ApiResponse[base.MessageResponse]{
			Success: true,
			Data:    base.MessageResponse{Message: "Backup deleted successfully", ActivityID: mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer()},
		},
	}, nil
}
