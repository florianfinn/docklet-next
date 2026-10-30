// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original typed connection-check results and activity tracking.

package environment

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/environment"
)

// TestConnection is shared by deliberate connect/reconnect actions. Periodic
// health checks call the service directly and do not create user activities.
func (h *EnvironmentHandler) TestConnection(ctx context.Context, input *TestConnectionInput) (*handlerutil.Out[dto.Test], error) {
	var apiURL *string
	if input.Body != nil {
		apiURL = input.Body.ApiUrl
	}
	if apiURL != nil {
		permissions, ok := middleware.PermissionsFromContext(ctx)
		if !ok || !permissions.Allows(authz.PermEnvironmentsUpdate, "") {
			return nil, huma.Error403Forbidden("permission denied: " + authz.PermEnvironmentsUpdate)
		}
	}
	projected := dto.Environment{ID: input.ID}
	if row, err := h.environmentService.GetEnvironmentByID(ctx, input.ID); err == nil {
		projected.IsEdge, projected.Status = row.IsEdge, row.Status
		h.environmentService.ApplyHostRuntimeState(ctx, &projected)
	}
	var runID string
	if h.activityService != nil {
		actor, _ := userctx.CurrentUserFromContext(ctx)
		run, err := h.activityService.StartActivity(ctx, activitylib.StartRequest{EnvironmentID: input.ID, Type: activityDTO.TypeResourceAction, ResourceType: new("environment"), ResourceID: new(input.ID), StartedBy: actor, Step: "checking", MessageCode: string(activityDTO.CodeEventEnvironmentConnect)})
		if err != nil {
			return nil, huma.Error503ServiceUnavailable("Connection check activity unavailable")
		}
		runID = run.ID
	}
	status, err := h.environmentService.TestConnection(ctx, input.ID, apiURL)
	var failure error
	switch {
	case err != nil:
		if verdict := handlerutil.AgentVerdict(err); verdict != nil {
			failure = verdict
		} else if apiURL != nil {
			apiErr := common.ToAPIError(err)
			failure = huma.NewError(apiErr.HTTPStatus(), apiErr.Message)
		} else {
			failure = hostProblem("host.unreachable", 502)
		}
	case status == string(EnvironmentStatusUpdateRequired):
		failure = handlerutil.AgentVerdict(ErrAgentUpdateRequired)
	case status != "online":
		failure = hostProblem("host.unreachable", 502)
	}
	if failure == nil && projected.Status == string(EnvironmentStatusStandby) {
		status = "standby-opened"
	}
	if runID != "" {
		outcome := activityDTO.StatusSuccess
		var problem *string
		if failure != nil {
			outcome = activityDTO.StatusFailed
			problem = new(failure.Error())
		}
		_, completeErr := h.activityService.CompleteActivity(context.WithoutCancel(ctx), runID, outcome, status, problem)
		if completeErr != nil {
			return nil, huma.Error500InternalServerError("Connection check activity could not be settled")
		}
	}
	if failure != nil {
		return nil, failure
	}
	return &handlerutil.Out[dto.Test]{Body: base.ApiResponse[dto.Test]{Success: true, Data: dto.Test{Status: status}}}, nil
}
