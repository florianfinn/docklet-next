// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Receipt-first hub detail, cancellation and explicit administrator disposition; M15 simple-user rights and identity contracts.

package activity

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
)

type runtimeHubInput struct {
	EnvironmentID string `path:"id"`
	RunID         string `path:"runId" format:"uuid"`
	Refresh       bool   `query:"refresh" default:"false"`
}
type runtimeReleaseInput struct {
	EnvironmentID string `path:"id"`
	RunID         string `path:"runId" format:"uuid"`
	Body          struct {
		Reason       string `json:"reason" minLength:"1" maxLength:"2048"`
		Acknowledged bool   `json:"acknowledged"`
	}
}

func runtimeHubReference(ctx context.Context, service *RuntimeService, environment, run string, cancel bool) (RuntimeRunReference, error) {
	ps, _ := middleware.PermissionsFromContext(ctx)
	if (ps == nil || ps.UserID == "") && (!ps.Allows(authz.PermActivitiesRead, environment) || (cancel && !ps.Allows(authz.PermActivitiesCancel, environment))) {
		return RuntimeRunReference{}, huma.Error403Forbidden("Activity access required")
	}
	if service == nil {
		return RuntimeRunReference{}, huma.Error503ServiceUnavailable("Runtime is not ready")
	}
	if ps != nil && ps.UserID != "" {
		if d := access.Decide(ctx, access.FromPermissions(ps), "get-hub-runtime-run", environment, access.Params{RunID: run}); d != access.Allow {
			return RuntimeRunReference{}, access.Problem(d.Status(), string(d))
		}
	}
	ref, found, err := service.Reference(ctx, environment, run)
	if err != nil {
		return RuntimeRunReference{}, huma.Error503ServiceUnavailable("Runtime evidence unavailable")
	}
	if !found {
		return RuntimeRunReference{}, huma.Error404NotFound("Runtime run not found")
	}
	return ref, nil
}

func RegisterRuntimeHubRoutes(api huma.API, service *RuntimeService) {
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "get-hub-runtime-run", Method: http.MethodGet, Path: "/environments/{id}/runtime-runs/{runId}", Tags: []string{"Runtime"}, Summary: "Read a durable runtime run"}, authz.PermActivitiesRead,
		func(ctx context.Context, input *runtimeHubInput) (*handlerutil.Out[activitytypes.RuntimeHubDetail], error) {
			ref, err := runtimeHubReference(ctx, service, input.EnvironmentID, input.RunID, false)
			if err != nil {
				return nil, err
			}
			if input.Refresh && input.EnvironmentID != "0" && service.remote != nil {
				_, _ = service.remote.Refresh(ctx, ref)
			}
			detail, err := service.GetRuntimeHubDetail(ctx, ref)
			if err != nil {
				return nil, huma.Error503ServiceUnavailable("Runtime detail unavailable")
			}
			return &handlerutil.Out[activitytypes.RuntimeHubDetail]{Body: base.ApiResponse[activitytypes.RuntimeHubDetail]{Success: true, Data: *detail}}, nil
		})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "cancel-hub-runtime-run", Method: http.MethodPost, Path: "/environments/{id}/runtime-runs/{runId}/cancel", Tags: []string{"Runtime"}, Summary: "Request runtime cancellation"}, authz.PermActivitiesCancel,
		func(ctx context.Context, input *runtimeHubInput) (*handlerutil.Out[activitytypes.RuntimeSnapshot], error) {
			ref, err := runtimeHubReference(ctx, service, input.EnvironmentID, input.RunID, true)
			if err != nil {
				return nil, err
			}
			actor, ok := userctx.CurrentUserFromContext(ctx)
			if !ok || actor == nil {
				return nil, huma.Error403Forbidden("Authenticated actor required")
			}
			var snapshot activitytypes.RuntimeSnapshot
			if input.EnvironmentID == "0" {
				snapshot, err = service.Cancel(ctx, ref, *actor)
			} else if service.remote != nil {
				snapshot, err = service.remote.Cancel(ctx, ref, *actor)
			} else {
				err = ErrRuntimeTransport
			}
			if err != nil {
				return nil, huma.Error503ServiceUnavailable("Runtime cancellation unavailable")
			}
			return &handlerutil.Out[activitytypes.RuntimeSnapshot]{Body: base.ApiResponse[activitytypes.RuntimeSnapshot]{Success: true, Data: snapshot}}, nil
		})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "release-runtime-run", Method: http.MethodPost, Path: "/environments/{id}/runtime-runs/{runId}/release", Tags: []string{"Runtime"}, Summary: "Acknowledge administrator runtime recovery"}, authz.PermActivitiesCancel,
		func(ctx context.Context, input *runtimeReleaseInput) (*handlerutil.Out[activitytypes.RuntimeSnapshot], error) {
			ref, err := runtimeHubReference(ctx, service, input.EnvironmentID, input.RunID, true)
			if err != nil {
				return nil, err
			}
			ps, _ := middleware.PermissionsFromContext(ctx)
			if !ps.IsGlobalAdmin() {
				return nil, huma.Error403Forbidden("Global administrator required")
			}
			if service.remote == nil {
				return nil, huma.Error503ServiceUnavailable("Runtime recovery unavailable")
			}
			snapshot, err := service.remote.Release(ctx, ref, input.Body.Reason, input.Body.Acknowledged)
			if err != nil {
				return nil, huma.Error409Conflict("Runtime recovery refused")
			}
			return &handlerutil.Out[activitytypes.RuntimeSnapshot]{Body: base.ApiResponse[activitytypes.RuntimeSnapshot]{Success: true, Data: snapshot}}, nil
		})
}
