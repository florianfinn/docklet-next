// Source: getarcaneapp/arcane/backend/internal/system/children/prune/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package prune

import (
	"context"
	"log/slog"

	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/system"

	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// Handler serves the system prune endpoint.
type Handler struct {
	service *Service
	appCtx  context.Context
}

func NewHandler(service *Service, appCtx context.Context) *Handler {
	return &Handler{service: service, appCtx: appCtx}
}

type PruneAllInput struct {
	EnvironmentID string                 `path:"id" doc:"Environment ID"`
	Body          system.PruneAllRequest `doc:"Prune options"`
}

// PruneAll removes unused Docker resources.
func (h *Handler) PruneAll(ctx context.Context, input *PruneAllInput) (*handlerutil.Out[system.PruneAllResult], error) {
	slog.InfoContext(ctx, "System prune operation initiated",
		"containers", input.Body.Containers,
		"images", input.Body.Images,
		"volumes", input.Body.Volumes,
		"networks", input.Body.Networks,
		"buildCache", input.Body.BuildCache)

	runtimeCtx := utils.ActivityRuntimeContext(ctx, h.appCtx)
	result := h.service.StartPruneAll(runtimeCtx, input.EnvironmentID, input.Body)

	slog.InfoContext(runtimeCtx, "System prune background activity started", "activityId", result.ActivityID)

	return &handlerutil.Out[system.PruneAllResult]{
		Body: base.ApiResponse[system.PruneAllResult]{
			Success: true,
			Data:    *result,
		},
	}, nil
}
