// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Hub-only stack policy routes with complete write-grant conjunction.

package updater

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"net/http"
)

type policyWriteInput struct {
	PointInput
	Body dto.UpdatePolicyInput
}

func registerImagePolicyRoutes(api huma.API, s *UpdaterService) {
	access := func(ctx context.Context, in PointInput, write bool) error {
		if in.Kind != "project" {
			return imageDTO.Failure("update.invalid-target", 400)
		}
		if _, err := pointAccess(ctx, in.EnvironmentID, write, false); err != nil {
			return err
		}
		if write {
			ps, _ := middleware.PermissionsFromContext(ctx)
			if !ps.Allows(authz.PermJobsManage, in.EnvironmentID) {
				return huma.Error403Forbidden("Policy job grants required")
			}
		}
		_, err := s.hubPolicies()
		return err
	}
	path := "/environments/{id}/updater/policies/{kind}/{targetId}"
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "get-auto-update-policy", Method: http.MethodGet, Path: path, Tags: []string{"Updater"}, Summary: "Read hub-owned stack update policy and candidate status"}, authz.PermProjectsRead, func(ctx context.Context, in *PointInput) (*handlerutil.Out[dto.AutoUpdatePolicy], error) {
		if err := access(ctx, *in, false); err != nil {
			return nil, err
		}
		result, err := s.GetImagePolicy(ctx, in.EnvironmentID, in.TargetID)
		return &handlerutil.Out[dto.AutoUpdatePolicy]{Body: base.ApiResponse[dto.AutoUpdatePolicy]{Success: err == nil, Data: result}}, err
	})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "set-auto-update-policy", Method: http.MethodPut, Path: path, Tags: []string{"Updater"}, Summary: "Set a registration-bound stack policy with expected revision"}, authz.PermJobsManage, func(ctx context.Context, in *policyWriteInput) (*handlerutil.Out[dto.AutoUpdatePolicy], error) {
		if err := access(ctx, in.PointInput, true); err != nil {
			return nil, err
		}
		actor, _ := middleware.GetUserIDFromContext(ctx)
		key, _ := ctx.Value(middleware.ContextKeyApiKeyID).(string)
		result, err := s.SaveImagePolicy(ctx, in.EnvironmentID, in.TargetID, in.Body, actor, key)
		return &handlerutil.Out[dto.AutoUpdatePolicy]{Body: base.ApiResponse[dto.AutoUpdatePolicy]{Success: err == nil, Data: result}}, err
	})
}
