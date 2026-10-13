// Source: getarcaneapp/arcane/backend/internal/system/children/backup/children/volumes/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package volumes

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/types/backup"

	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// Handler serves the system-managed volume backup endpoints.
type Handler struct {
	service *Service
	appCtx  context.Context
}

func NewHandler(service *Service, appCtx context.Context) *Handler {
	return &Handler{service: service, appCtx: appCtx}
}

type SystemVolumeBackupConfigOutput struct {
	Body backup.SystemVolumeBackupPolicyCollection
}

type UpdateSystemVolumeBackupConfigInput struct {
	Body backup.UpdateSystemVolumeBackupPolicies
}

type SystemVolumeBackupOptionsOutput struct {
	Body []backup.SystemVolumeBackupOption
}

type SystemVolumeBackupRunOutput struct {
	Body backup.BackupRunAccepted
}

type RunSystemVolumeBackupsInput struct {
	Body *backup.RunSystemVolumeBackupsRequest `json:"body,omitempty"`
}

func (h *Handler) GetSystemVolumeConfig(ctx context.Context, _ *struct{}) (*SystemVolumeBackupConfigOutput, error) {
	config, err := h.service.GetConfig(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	return &SystemVolumeBackupConfigOutput{Body: *config}, nil
}

func (h *Handler) UpdateSystemVolumeConfig(ctx context.Context, input *UpdateSystemVolumeBackupConfigInput) (*SystemVolumeBackupConfigOutput, error) {
	config, err := h.service.UpdateConfig(ctx, input.Body.Policies)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	return &SystemVolumeBackupConfigOutput{Body: *config}, nil
}

func (h *Handler) ListSystemVolumeOptions(ctx context.Context, _ *struct{}) (*SystemVolumeBackupOptionsOutput, error) {
	options, err := h.service.ListOptions(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	return &SystemVolumeBackupOptionsOutput{Body: options}, nil
}

func (h *Handler) RunSystemVolumeBackups(ctx context.Context, input *RunSystemVolumeBackupsInput) (*SystemVolumeBackupRunOutput, error) {
	request := backup.RunSystemVolumeBackupsRequest{}
	if input.Body != nil {
		request = *input.Body
	}
	user, err := handlerutil.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	result, err := h.service.Start(utils.ActivityRuntimeContext(ctx, h.appCtx), *user, request)
	if errors.Is(err, h.service.alreadyRunning) {
		return nil, huma.Error409Conflict(err.Error())
	}
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	return &SystemVolumeBackupRunOutput{Body: *result}, nil
}
