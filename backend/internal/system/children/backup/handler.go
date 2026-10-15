// Source: getarcaneapp/arcane/backend/internal/system/children/backup/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Replaced Rustic recovery handlers with the PostgreSQL archive contract.

package backup

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
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
type UploadSystemBackupInput struct {
	Body backup.UploadSystemBackupRequest
}
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

// Mutation and file routes remain unavailable until the archive engine lands.
func (h *Handler) UpdatePolicies(context.Context, *UpdateSystemBackupPoliciesInput) (*SystemBackupPoliciesOutput, error) {
	return nil, huma.Error501NotImplemented("backup.not_implemented")
}
func (h *Handler) Create(context.Context, *CreateSystemBackupInput) (*SystemBackupOutput, error) {
	return nil, huma.Error501NotImplemented("backup.not_implemented")
}
func (h *Handler) Upload(context.Context, *UploadSystemBackupInput) (*SystemBackupOperationOutput, error) {
	return nil, huma.Error501NotImplemented("backup.not_implemented")
}
func (h *Handler) Download(context.Context, *BackupIDInput) (*huma.StreamResponse, error) {
	return nil, huma.Error501NotImplemented("backup.not_implemented")
}
func (h *Handler) Restore(context.Context, *RestoreSystemBackupInput) (*SystemBackupOperationOutput, error) {
	return nil, huma.Error501NotImplemented("backup.not_implemented")
}
func (h *Handler) Delete(context.Context, *BackupIDInput) (*SystemBackupOperationOutput, error) {
	return nil, huma.Error501NotImplemented("backup.not_implemented")
}
func (h *Handler) Storage(context.Context, *struct{}) (*SystemBackupStorageOutput, error) {
	return nil, huma.Error501NotImplemented("backup.not_implemented")
}
func (h *Handler) Maintenance(context.Context, *struct{}) (*SystemBackupMaintenanceOutput, error) {
	return nil, huma.Error501NotImplemented("backup.not_implemented")
}
