// Source: getarcaneapp/arcane/backend/internal/system/children/backup/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Replaced Rustic recovery handlers with the SQLite archive contract.

package backup

import (
	"context"
	"errors"
	"io"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	sharedbackup "github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	userdomain "github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

type Handler struct {
	service *Service
	appCtx  context.Context
}

type ListSystemBackupsInput struct {
	Search string `query:"search"`
	Sort   string `query:"sort" default:"createdAt"`
	Order  string `query:"order" default:"desc"`
	Start  int    `query:"start" default:"0"`
	Limit  int    `query:"limit" default:"20"`
}

type ListBackupHistoryInput struct {
	Search string `query:"search"`
	Sort   string `query:"sort" default:"createdAt"`
	Order  string `query:"order" default:"desc"`
	Start  int    `query:"start" default:"0"`
	Limit  int    `query:"limit" default:"20"`
	Type   string `query:"type"`
}

type SystemBackupPoliciesOutput struct {
	Body backup.SystemBackupPolicyCollection
}
type UpdateSystemBackupPoliciesInput struct {
	Body backup.UpdateSystemBackupPolicies
}
type CreateSystemBackupInput struct {
	Body backup.CreateSystemBackupRequest
}
type SystemBackupOutput struct{ Body backup.SystemBackupRun }
type UploadSystemBackupInput struct{}
type SystemBackupOperationOutput struct{ Body backup.SystemBackupOperation }
type SystemBackupStorageOutput struct{ Body backup.SystemBackupStorage }
type SystemBackupMaintenanceOutput struct {
	Body backup.SystemBackupMaintenance
}
type BackupIDInput struct {
	ID string `path:"id" pattern:"^[A-Za-z0-9_-]{1,128}$"`
}
type RestoreSystemBackupInput struct {
	ID   string `path:"id" pattern:"^[A-Za-z0-9_-]{1,128}$"`
	Body backup.RestoreSystemBackupRequest
}

func NewHandler(service *Service, _ *activity.ActivityService, appCtx context.Context) *Handler {
	return &Handler{service: service, appCtx: appCtx}
}

func (h *Handler) ListHistory(ctx context.Context, input *ListBackupHistoryInput) (*handlerutil.Page[backup.HistoryEntry], error) {
	params := handlerutil.PaginationParams(input.Start, input.Limit, input.Sort, input.Order, input.Search)
	params.Filters = map[string]string{"type": input.Type}
	history, page, err := h.service.ListBackupHistory(ctx, params)
	if err != nil {
		return nil, huma.Error500InternalServerError("backup.history_unavailable")
	}
	return &handlerutil.Page[backup.HistoryEntry]{Body: base.Paginated[backup.HistoryEntry]{
		Success: true, Data: history, Pagination: handlerutil.PaginationResponse(page),
	}}, nil
}

func (h *Handler) List(ctx context.Context, input *ListSystemBackupsInput) (*handlerutil.Page[backup.SystemBackupRun], error) {
	params := handlerutil.PaginationParams(input.Start, input.Limit, input.Sort, input.Order, input.Search)
	runs, page, err := h.service.ListBackups(ctx, params)
	if err != nil {
		return nil, huma.Error500InternalServerError("backup.list_unavailable")
	}
	return &handlerutil.Page[backup.SystemBackupRun]{Body: base.Paginated[backup.SystemBackupRun]{
		Success: true, Data: runs, Pagination: handlerutil.PaginationResponse(page),
	}}, nil
}

func (h *Handler) GetPolicies(ctx context.Context, _ *struct{}) (*SystemBackupPoliciesOutput, error) {
	policies, err := h.service.GetPolicies(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("backup.policies_unavailable")
	}
	return &SystemBackupPoliciesOutput{Body: *policies}, nil
}

func (h *Handler) UpdatePolicies(ctx context.Context, input *UpdateSystemBackupPoliciesInput) (*SystemBackupPoliciesOutput, error) {
	if h.service == nil {
		return nil, huma.Error501NotImplemented("backup.not_implemented")
	}
	policies, err := h.service.UpdatePolicies(ctx, input.Body.Policies)
	if errors.Is(err, ErrSystemBackupTargetUnavailable) {
		return nil, huma.Error422UnprocessableEntity("backup.target_unavailable")
	}
	if errors.Is(err, ErrSystemBackupUnavailable) {
		return nil, huma.Error503ServiceUnavailable("backup.storage_unavailable")
	}
	if errors.Is(err, ErrSystemBackupToolsUnavailable) {
		return nil, huma.Error503ServiceUnavailable("backup.tools_unavailable")
	}
	if errors.Is(err, sharedbackup.ErrPolicyValidation) {
		return nil, huma.Error400BadRequest("backup.invalid_policy")
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("backup.failed")
	}
	return &SystemBackupPoliciesOutput{Body: *policies}, nil
}
func (h *Handler) Create(ctx context.Context, input *CreateSystemBackupInput) (*SystemBackupOutput, error) {
	if h.service == nil {
		return nil, huma.Error501NotImplemented("backup.not_implemented")
	}
	actor := usertypes.Actor{}
	if currentUser, exists := userdomain.CurrentUserFromContext(ctx); exists && currentUser != nil {
		actor = *currentUser.Actor()
	}
	run, err := h.service.StartBackup(ctx, actor, input.Body)
	if errors.Is(err, ErrSystemBackupAlreadyRunning) {
		return nil, huma.Error409Conflict("run.busy")
	}
	if errors.Is(err, ErrSystemBackupNotFound) {
		return nil, huma.Error404NotFound("backup.not_found")
	}
	if errors.Is(err, ErrSystemBackupTargetUnavailable) {
		return nil, huma.Error422UnprocessableEntity("backup.target_unavailable")
	}
	if errors.Is(err, ErrSystemBackupUnavailable) {
		return nil, huma.Error503ServiceUnavailable("backup.storage_unavailable")
	}
	if errors.Is(err, ErrSystemBackupToolsUnavailable) {
		return nil, huma.Error503ServiceUnavailable("backup.tools_unavailable")
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("backup.failed")
	}
	return &SystemBackupOutput{Body: *run}, nil
}
func (h *Handler) Upload(ctx context.Context, _ *UploadSystemBackupInput) (*SystemBackupOperationOutput, error) {
	operation, ok := ctx.Value(uploadResultKey{}).(*backup.SystemBackupOperation)
	if !ok || operation == nil {
		return nil, huma.Error501NotImplemented("backup.not_implemented")
	}
	return &SystemBackupOperationOutput{Body: *operation}, nil
}
func (h *Handler) Download(ctx context.Context, input *BackupIDInput) (*huma.StreamResponse, error) {
	if h.service == nil {
		return nil, huma.Error501NotImplemented("backup.not_implemented")
	}
	reader, size, name, err := h.service.OpenArchive(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("backup.not_found")
	}
	return &huma.StreamResponse{Body: func(out huma.Context) {
		defer reader.Close()
		out.SetHeader("Content-Type", "application/octet-stream")
		out.SetHeader("Content-Disposition", "attachment; filename=\""+name+"\"")
		out.SetHeader("Cache-Control", "no-store")
		out.SetHeader("Content-Length", strconv.FormatInt(size, 10))
		_, _ = io.Copy(out.BodyWriter(), reader)
	}}, nil
}
func (h *Handler) Restore(ctx context.Context, input *RestoreSystemBackupInput) (*SystemBackupOperationOutput, error) {
	if h.service == nil {
		return nil, huma.Error501NotImplemented("backup.not_implemented")
	}
	actor := usertypes.Actor{}
	if current, ok := userdomain.CurrentUserFromContext(ctx); ok && current != nil {
		actor = *current.Actor()
	}
	keyID, _ := ctx.Value(middleware.ContextKeyApiKeyID).(string)
	operation, err := h.service.StartRestore(ctx, actor, keyID, input.ID, input.Body.ConfirmBackupID)
	switch {
	case errors.Is(err, ErrSystemBackupNotFound):
		return nil, huma.Error404NotFound("backup.not_found")
	case errors.Is(err, ErrSystemBackupAlreadyRunning):
		return nil, huma.Error409Conflict("run.busy")
	case errors.Is(err, errInvalidArchive), errors.Is(err, errArchiveMajor), errors.Is(err, errMigration):
		return nil, huma.Error422UnprocessableEntity(uploadErrorCode(err))
	case errors.Is(err, ErrSystemBackupUnavailable):
		return nil, huma.Error503ServiceUnavailable("backup.storage_unavailable")
	case errors.Is(err, ErrSystemBackupToolsUnavailable):
		return nil, huma.Error503ServiceUnavailable("backup.tools_unavailable")
	case err != nil:
		return nil, huma.Error500InternalServerError("backup.restore_failed")
	}
	return &SystemBackupOperationOutput{Body: *operation}, nil
}
func (h *Handler) Delete(ctx context.Context, input *BackupIDInput) (*SystemBackupOperationOutput, error) {
	if h.service == nil {
		return nil, huma.Error501NotImplemented("backup.not_implemented")
	}
	operation, err := h.service.DeleteBackup(ctx, input.ID)
	if errors.Is(err, ErrSystemBackupAlreadyRunning) {
		return nil, huma.Error409Conflict("run.busy")
	}
	if errors.Is(err, ErrSystemBackupNotFound) {
		return nil, huma.Error404NotFound("backup.not_found")
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("backup.failed")
	}
	return &SystemBackupOperationOutput{Body: *operation}, nil
}
func (h *Handler) Storage(context.Context, *struct{}) (*SystemBackupStorageOutput, error) {
	if h.service == nil {
		return nil, huma.Error501NotImplemented("backup.not_implemented")
	}
	return &SystemBackupStorageOutput{Body: h.service.Storage()}, nil
}
func (h *Handler) Maintenance(context.Context, *struct{}) (*SystemBackupMaintenanceOutput, error) {
	if h.service == nil {
		return nil, huma.Error501NotImplemented("backup.not_implemented")
	}
	return &SystemBackupMaintenanceOutput{Body: h.service.MaintenanceGate().Status()}, nil
}
