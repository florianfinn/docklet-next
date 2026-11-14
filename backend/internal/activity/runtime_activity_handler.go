// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Generic cancellation resolves durable runtime ownership before fallback; authorize single-run reads by persisted actor or current target; mask receipt storage failures for simple users and log them.

package activity

import (
	"context"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	"gorm.io/gorm"
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
	ref, found, err := runtime.Reference(ctx, input.EnvironmentID, input.ActivityID)
	if err != nil {
		return nil, true, huma.Error503ServiceUnavailable("Runtime evidence unavailable")
	}
	if !found {
		return nil, false, nil
	}
	var receipt *RuntimeReceipt
	err = runtime.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		var readErr error
		receipt, readErr = runtimeReferenceRowTxInternal(tx, ref)
		return readErr
	})
	if err != nil {
		slog.WarnContext(ctx, "failed to load runtime activity receipt", "error", err)
		ps, _ := middleware.PermissionsFromContext(ctx)
		if access.FromPermissions(ps).IsUser() {
			return nil, true, huma.Error404NotFound("activity not found")
		}
		return nil, true, huma.Error503ServiceUnavailable("Runtime evidence unavailable")
	}
	if !h.canReadRuntimeActivityInternal(ctx, input.EnvironmentID, receipt) {
		return nil, true, huma.Error404NotFound("activity not found")
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

// Supporting host grants do not authorize a simple user to inspect unrelated
// runs. Target access is checked against current membership, never a stale parent.
func (h *ActivityHandler) canReadRuntimeActivityInternal(ctx context.Context, environmentID string, item *RuntimeReceipt) bool {
	ps, _ := middleware.PermissionsFromContext(ctx)
	principal := access.FromPermissions(ps)
	if !principal.IsUser() {
		return ps.Allows(authz.PermActivitiesRead, environmentID)
	}
	if item.CallerID != nil && *item.CallerID == principal.ID {
		return true
	}
	if item.TargetKind == nil || item.TargetID == nil || h.activityService.runtimeHTTP == nil {
		return false
	}
	target := RuntimeTarget{Kind: *item.TargetKind, ID: *item.TargetID}
	operation, params := "", access.Params{}
	switch target.Kind {
	case "stack":
		operation, params.ProjectID = "get-project", target.ID
	case "container":
		operation, params.ContainerID = "get-container", target.ID
	default:
		return false
	}
	if access.Decide(ctx, principal, operation, environmentID, params) != access.Allow {
		return false
	}
	// An assignment may outlive its target. The real local/Direct/Edge resolver
	// must still find the exact canonical resource before disclosing its run.
	readCtx, cancel := context.WithTimeout(ctx, activityAgentReadBudget)
	defer cancel()
	resolved, err := h.activityService.runtimeHTTP.ports.PrepareTarget(readCtx, environmentID, target)
	return err == nil && resolved.Target.Kind == target.Kind && resolved.Target.ID == target.ID
}
