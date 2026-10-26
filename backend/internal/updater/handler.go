// Source: getarcaneapp/arcane/backend/internal/updater/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package updater

import (
	"context"
	imageDTO "github.com/florianfinn/docklet-next/types/image"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/project"
	"github.com/florianfinn/docklet-next/types/updater"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// UpdaterHandler provides Huma-based updater management endpoints.
type UpdaterHandler struct {
	updaterService *UpdaterService
	appCtx         context.Context
}

type RunUpdaterInput struct {
	EnvironmentID string           `path:"id" doc:"Environment ID"`
	Body          *updater.Options `doc:"Updater run options"`
}

type UpdateContainerInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	ContainerID   string `path:"containerId" doc:"Container ID to update"`
	Async         bool   `query:"async" doc:"Return an accepted activity immediately instead of waiting for the update"`
}

type updateContainerOutput struct {
	Status int
	Body   base.ApiResponse[any]
}

type GetUpdaterStatusInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
}

type GetUpdaterHistoryInput struct {
	EnvironmentID string `path:"id" doc:"Environment ID"`
	Limit         int    `query:"limit" default:"50" doc:"Number of history entries to return"`
}

// RunUpdater refuses the retired updater apply path.
func (h *UpdaterHandler) RunUpdater(ctx context.Context, input *RunUpdaterInput) (*handlerutil.Out[*updater.Result], error) {
	return nil, imageDTO.Failure("update.path-retired", 409)
}

// GetUpdaterStatus returns the current status of the updater.
func (h *UpdaterHandler) GetUpdaterStatus(ctx context.Context, input *GetUpdaterStatusInput) (*handlerutil.Out[updater.Status], error) {
	status := h.updaterService.GetStatus()

	return &handlerutil.Out[updater.Status]{
		Body: base.ApiResponse[updater.Status]{
			Success: true,
			Data:    status,
		},
	}, nil
}

// GetUpdaterHistory returns the history of update operations.
func (h *UpdaterHandler) GetUpdaterHistory(ctx context.Context, input *GetUpdaterHistoryInput) (*handlerutil.Out[[]AutoUpdateRecord], error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	history, err := h.updaterService.GetHistory(ctx, limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to get updater history: " + err.Error())
	}

	return &handlerutil.Out[[]AutoUpdateRecord]{
		Body: base.ApiResponse[[]AutoUpdateRecord]{
			Success: true,
			Data:    history,
		},
	}, nil
}

func (h *UpdaterHandler) updateContainerInternal(ctx context.Context, input *UpdateContainerInput) (*updateContainerOutput, error) {
	return nil, imageDTO.Failure("update.path-retired", 409)
}

// CheckProjectUpdates checks Compose policies through the updater service.
func (h *UpdaterHandler) CheckProjectUpdates(ctx context.Context, input *updater.CheckProjectInput) (*handlerutil.Out[*project.UpdateInfo], error) {
	result, err := h.updaterService.CheckPolicyProject(ctx, input.EnvironmentID, input.ProjectID)
	if err != nil {
		return nil, err
	}
	return &handlerutil.Out[*project.UpdateInfo]{Body: base.ApiResponse[*project.UpdateInfo]{Success: true, Data: result}}, nil
}
