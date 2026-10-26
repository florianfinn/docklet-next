// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Restore-point discovery and explicit pin release, including orphaned stacks.

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

type PointInput struct {
	EnvironmentID string `path:"id"`
	Kind          string `path:"kind"`
	TargetID      string `path:"targetId"`
}
type pointDeleteInput struct {
	PointInput
	PointID string `path:"pointId"`
}
type HostPointsInput struct {
	EnvironmentID string `path:"id"`
}
type hostPointDeleteInput struct {
	HostPointsInput
	PointID string `path:"pointId"`
}
type baselineInput struct {
	EnvironmentID string `path:"id"`
	ProjectID     string `path:"projectId"`
}

func pointAccess(ctx context.Context, env string, write, host bool) (bool, error) {
	ps, _ := middleware.PermissionsFromContext(ctx)
	if ps == nil {
		return false, huma.Error403Forbidden("Restore point access required")
	}
	required := []string{authz.PermImageUpdatesRead, authz.PermProjectsRead}
	if write {
		required = []string{authz.PermImageUpdatesCheck, authz.PermImagesPull, authz.PermActivitiesRead, authz.PermProjectsDeploy, authz.PermProjectsUpdate}
	}
	if host {
		if !ps.IsGlobalAdmin() {
			return false, huma.Error403Forbidden("Host administrator required")
		}
		required = []string{authz.PermImageUpdatesRead}
	}
	for _, grant := range required {
		if !ps.Allows(grant, env) {
			return false, huma.Error403Forbidden("Restore point grants required")
		}
	}
	return ps.IsGlobalAdmin(), nil
}
func registerRestorePointRoutes(api huma.API, s *UpdaterService) {
	list := func(ctx context.Context, env, target string, host bool) (*handlerutil.Out[[]dto.RestorePoint], error) {
		admin, err := pointAccess(ctx, env, false, host)
		if err != nil {
			return nil, err
		}
		if s == nil || s.deps.Projects == nil {
			return nil, huma.Error503ServiceUnavailable("Restore points unavailable")
		}
		rows, err := s.deps.Projects.ListImageRestorePoints(ctx, target, admin)
		if err != nil {
			return nil, err
		}
		return &handlerutil.Out[[]dto.RestorePoint]{Body: base.ApiResponse[[]dto.RestorePoint]{Success: true, Data: rows}}, nil
	}
	remove := func(ctx context.Context, env, target, id string, host bool) (*handlerutil.Out[base.MessageResponse], error) {
		admin, err := pointAccess(ctx, env, true, host)
		if err != nil {
			return nil, err
		}
		if s == nil || s.deps.Projects == nil {
			return nil, huma.Error503ServiceUnavailable("Restore points unavailable")
		}
		if err = s.deps.Projects.DeleteImageRestorePoint(ctx, target, id, admin); err != nil {
			return nil, err
		}
		return &handlerutil.Out[base.MessageResponse]{Body: base.ApiResponse[base.MessageResponse]{Success: true, Data: base.MessageResponse{Message: "Restore point deleted"}}}, nil
	}
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "list-restore-points", Method: http.MethodGet, Path: "/environments/{id}/updater/{kind}/{targetId}/restore-points", Tags: []string{"Updater"}, Summary: "List stack restore-point metadata"}, authz.PermImageUpdatesRead, func(ctx context.Context, in *PointInput) (*handlerutil.Out[[]dto.RestorePoint], error) {
		if in.Kind != "project" {
			return nil, imageDTO.Failure("update.invalid-target", 400)
		}
		return list(ctx, in.EnvironmentID, in.TargetID, false)
	})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "delete-restore-point", Method: http.MethodDelete, Path: "/environments/{id}/updater/{kind}/{targetId}/restore-points/{pointId}", Tags: []string{"Updater"}, Summary: "Delete an unheld stack restore point"}, authz.PermProjectsUpdate, func(ctx context.Context, in *pointDeleteInput) (*handlerutil.Out[base.MessageResponse], error) {
		if in.Kind != "project" {
			return nil, imageDTO.Failure("update.invalid-target", 400)
		}
		return remove(ctx, in.EnvironmentID, in.TargetID, in.PointID, false)
	})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "list-host-restore-points", Method: http.MethodGet, Path: "/environments/{id}/updater/restore-points", Tags: []string{"Updater"}, Summary: "List host restore points including orphaned targets"}, authz.PermImageUpdatesRead, func(ctx context.Context, in *HostPointsInput) (*handlerutil.Out[[]dto.RestorePoint], error) {
		return list(ctx, in.EnvironmentID, "", true)
	})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "delete-host-restore-point", Method: http.MethodDelete, Path: "/environments/{id}/updater/restore-points/{pointId}", Tags: []string{"Updater"}, Summary: "Delete an unheld orphan or host restore point"}, authz.PermImageUpdatesRead, func(ctx context.Context, in *hostPointDeleteInput) (*handlerutil.Out[base.MessageResponse], error) {
		return remove(ctx, in.EnvironmentID, "", in.PointID, true)
	})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "get-update-baseline", Method: http.MethodGet, Path: "/environments/{id}/updater/projects/{projectId}/baseline", Tags: []string{"Updater"}, Summary: "Read baseline availability and current input revision"}, authz.PermProjectsRead, func(ctx context.Context, in *baselineInput) (*handlerutil.Out[dto.BaselineStatus], error) {
		if _, err := pointAccess(ctx, in.EnvironmentID, false, false); err != nil {
			return nil, err
		}
		if s == nil || s.deps.Projects == nil {
			return nil, huma.Error503ServiceUnavailable("Baseline unavailable")
		}
		result, err := s.deps.Projects.ImageBaselineStatus(ctx, in.ProjectID, s.deps.Projects.LifecycleRegistration(ctx))
		return &handlerutil.Out[dto.BaselineStatus]{Body: base.ApiResponse[dto.BaselineStatus]{Success: err == nil, Data: result}}, err
	})
}
