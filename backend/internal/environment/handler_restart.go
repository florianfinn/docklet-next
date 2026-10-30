// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original hub restart intake and durable observation API.

package environment

import (
	"context"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"gorm.io/gorm"
)

type restartAgentInput struct {
	ID string `path:"id"`
}
type restartAgentRunInput struct {
	ID    string `path:"id"`
	RunID string `path:"runId"`
}

func registerHostRestart(api huma.API, h *EnvironmentHandler) {
	huma.Register(api, huma.Operation{OperationID: "restartEnvironmentAgent", Method: "POST", Path: "/environments/{id}/agent/restart", Summary: "Restart a remote agent once", DefaultStatus: 202, Tags: []string{"Environments"}, Security: handlerutil.DefaultOperationSecurity(), Middlewares: middleware.RequireGlobalAdmin(api)}, h.RestartAgent)
	handlerutil.HostProblemResponses(api, api.OpenAPI().Paths["/environments/{id}/agent/restart"].Post, "409", "502")
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "getEnvironmentAgentRestart", Method: "GET", Path: "/environments/{id}/agent/restarts/{runId}", Summary: "Read a durable agent restart run", Tags: []string{"Environments"}, Security: handlerutil.DefaultOperationSecurity()}, authz.PermEnvironmentsRead, h.AgentRestartRun)
}
func restartRunDTO(row AgentRestartRequest) dto.AgentRestartRun {
	out := dto.AgentRestartRun{RunID: row.RunID, State: row.State, CreatedAt: row.CreatedAt, Deadline: row.Deadline, SettledAt: row.SettledAt}
	if row.Code != nil {
		out.Code = *row.Code
	}
	return out
}
func (h *EnvironmentHandler) RestartAgent(ctx context.Context, in *restartAgentInput) (*handlerutil.Out[dto.AgentRestartRun], error) {
	if h.environmentService == nil || h.environmentService.restarts == nil {
		return nil, huma.Error503ServiceUnavailable("Agent restart controller unavailable")
	}
	if in.ID != "0" {
		if _, err := h.environmentService.CheckAgentCompatibility(ctx, in.ID); err != nil {
			if verdict := handlerutil.AgentVerdict(err); verdict != nil {
				return nil, verdict
			}
			return nil, hostProblem("host.unreachable", 502)
		}
	}
	row, err := h.environmentService.restarts.intake(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &handlerutil.Out[dto.AgentRestartRun]{Body: base.ApiResponse[dto.AgentRestartRun]{Success: true, Data: restartRunDTO(row)}}, nil
}
func (h *EnvironmentHandler) AgentRestartRun(ctx context.Context, in *restartAgentRunInput) (*handlerutil.Out[dto.AgentRestartRun], error) {
	var row AgentRestartRequest
	if h.environmentService == nil {
		return nil, huma.Error503ServiceUnavailable("Agent restart store unavailable")
	}
	if err := h.environmentService.db.WithContext(ctx).First(&row, "run_id=? AND environment_id=?", in.RunID, in.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("Agent restart run not found")
		}
		return nil, huma.Error503ServiceUnavailable("Agent restart store unavailable")
	}
	return &handlerutil.Out[dto.AgentRestartRun]{Body: base.ApiResponse[dto.AgentRestartRun]{Success: true, Data: restartRunDTO(row)}}, nil
}
