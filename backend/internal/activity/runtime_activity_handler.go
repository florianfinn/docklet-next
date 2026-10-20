// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Generic cancellation resolves durable runtime ownership before fallback.

package activity

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
)

func (h *ActivityHandler) cancelRuntimeInternal(ctx context.Context, input *CancelActivityInput) (*handlerutil.Out[activitytypes.Activity], bool, error) {
	runtime := h.activityService.runtime
	if runtime == nil {
		return nil, false, nil
	}
	// Do not expose receipt presence or remote details to a caller without both
	// existing grants, even when this helper is invoked outside route middleware.
	ps, _ := middleware.PermissionsFromContext(ctx)
	if !ps.Allows(authz.PermActivitiesRead, input.EnvironmentID) || !ps.Allows(authz.PermActivitiesCancel, input.EnvironmentID) {
		return nil, true, huma.Error403Forbidden("Activity access required")
	}
	ref, owned, err := runtime.Reference(ctx, input.EnvironmentID, input.ActivityID)
	if err != nil {
		return nil, true, huma.Error503ServiceUnavailable("Runtime outcome unavailable")
	}
	if !owned {
		return nil, false, nil
	}
	actor, ok := userctx.CurrentUserFromContext(ctx)
	if !ok || actor == nil {
		return nil, true, huma.Error403Forbidden("Authenticated actor required")
	}
	var snapshot activitytypes.RuntimeSnapshot
	if input.EnvironmentID != "0" && runtime.remote != nil {
		snapshot, err = runtime.remote.Cancel(ctx, ref, *actor)
	} else {
		snapshot, err = runtime.Cancel(ctx, ref, *actor)
	}
	if err != nil {
		return nil, true, huma.Error503ServiceUnavailable("Runtime cancellation unavailable")
	}
	h.applyActivitySourceLabelInternal(ctx, input.EnvironmentID, &snapshot.Activity)
	return &handlerutil.Out[activitytypes.Activity]{Body: base.ApiResponse[activitytypes.Activity]{Success: true, Data: snapshot.Activity}}, true, nil
}

func (h *ActivityHandler) getRuntimeActivityInternal(ctx context.Context, input *GetActivityInput) (*activitytypes.Detail, bool, error) {
	runtime := h.activityService.runtime
	if runtime == nil {
		return nil, false, nil
	}
	ps, _ := middleware.PermissionsFromContext(ctx)
	if !ps.Allows(authz.PermActivitiesRead, input.EnvironmentID) {
		return nil, true, huma.Error403Forbidden("Activity access required")
	}
	ref, found, err := runtime.Reference(ctx, input.EnvironmentID, input.ActivityID)
	if err != nil {
		return nil, true, huma.Error503ServiceUnavailable("Runtime evidence unavailable")
	}
	if !found {
		return nil, false, nil
	}
	detail, err := runtime.GetRuntimeDetail(ctx, ref)
	if err != nil {
		return nil, true, huma.Error503ServiceUnavailable("Runtime detail unavailable")
	}
	model := detail.Activity.Activity
	knowledge := detail.Activity.RuntimeKnowledge
	model.RuntimeKnowledge = &knowledge
	h.applyActivitySourceLabelInternal(ctx, input.EnvironmentID, &model)
	output := make([]activitytypes.OutputLine, len(detail.Output))
	for i, line := range detail.Output {
		output[i] = line.OutputLine
	}
	return &activitytypes.Detail{Activity: model, Messages: detail.Messages, StepTrace: detail.StepTrace, Output: output, OutputTruncated: detail.OutputTruncated}, true, nil
}
