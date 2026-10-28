// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/children/restore/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace
; M12 persisted volume lifecycle and legacy restore cut.
package restore

import (
	"context"
	"os"
	"path/filepath"
	"uuid"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/upload"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/volume"
)

type Handler struct {
	service         *Service
	activityService *activity.ActivityService
	uploadService   *upload.UploadService
	appCtx          context.Context
}
type RestoreBackupInput struct {
	EnvironmentID string                 `path:"id" doc:"Environment ID"`
	VolumeName    string                 `path:"volumeName" doc:"Volume name"`
	BackupID      string                 `path:"backupId" doc:"Backup ID"`
	Body          *volume.RestoreRequest `json:"body,omitempty"`
}
type RestoreBackupFilesInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	VolumeName    string `path:"volumeName" doc:"Volume name"`
	BackupID      string `path:"backupId" doc:"Backup ID"`
	Body          backup.RestoreSelection
}
type UploadAndRestoreInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	VolumeName    string `path:"volumeName" doc:"Volume name"`
	Body          volume.UploadRestoreRequest
}

func NewHandler(service *Service, a *activity.ActivityService, u *upload.UploadService, ctx context.Context) *Handler {
	return &Handler{service: service, activityService: a, uploadService: u, appCtx: ctx}
}
func (h *Handler) start(ctx context.Context, r StartRequest) (*handlerutil.Out[volume.RestoreOperation], error) {
	actor, err := handlerutil.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	r.User = *actor
	accepted, err := h.service.lifecycle.Start(utils.ActivityRuntimeContext(ctx, h.appCtx), r)
	if err != nil {
		if status, ok := err.(huma.StatusError); ok {
			return nil, status
		}
		return nil, backup.NewProblem(500, "volume.restore.interrupted-before-write", err)
	}
	return &handlerutil.Out[volume.RestoreOperation]{Body: base.ApiResponse[volume.RestoreOperation]{Success: true, Data: accepted}}, nil
}
func (h *Handler) RestoreBackup(ctx context.Context, i *RestoreBackupInput) (*handlerutil.Out[volume.RestoreOperation], error) {
	r := StartRequest{EnvironmentID: i.EnvironmentID, VolumeName: i.VolumeName, BackupID: i.BackupID, Kind: "full"}
	if i.Body != nil {
		r.Mode = i.Body.Mode
		r.NewVolumeName = i.Body.NewVolumeName
	}
	return h.start(ctx, r)
}
func (h *Handler) RestoreBackupFiles(ctx context.Context, i *RestoreBackupFilesInput) (*handlerutil.Out[volume.RestoreOperation], error) {
	return h.start(ctx, StartRequest{EnvironmentID: i.EnvironmentID, VolumeName: i.VolumeName, BackupID: i.BackupID, Kind: "files", Selection: i.Body})
}
func (h *Handler) UploadAndRestore(ctx context.Context, i *UploadAndRestoreInput) (*handlerutil.Out[volume.RestoreOperation], error) {
	if _, err := handlerutil.RequireUser(ctx); err != nil {
		return nil, err
	}
	id := uuid.New().String()
	input, err := h.uploadService.TakeRestoreInput(ctx, i.Body.UploadID, id)
	if err != nil {
		if status, ok := err.(huma.StatusError); ok {
			return nil, status
		}
		if mapped := upload.SessionHTTPError(err); mapped != nil {
			return nil, mapped
		}
		return nil, backup.NewProblem(500, "volume.backup.upload-invalid", err)
	}
	accepted, err := h.start(ctx, StartRequest{EnvironmentID: i.EnvironmentID, VolumeName: i.VolumeName, Kind: "upload", InputPath: input, RestoreID: id, Mode: i.Body.Mode, NewVolumeName: i.Body.NewVolumeName})
	// Admission failure leaves no worker to own the input. Once accepted, only
	// the worker or startup reconciliation may remove it.
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(input))
	}
	return accepted, err
}
