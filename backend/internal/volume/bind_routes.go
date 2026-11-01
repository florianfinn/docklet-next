// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/volume"
	"net/http"
)

type listBindsInput struct {
	EnvironmentID string `path:"id"`
}
type measureBindInput struct {
	EnvironmentID string `path:"id"`
	Body          struct {
		Key string `json:"key"`
	}
}

func registerBindRoutes(api huma.API, s *VolumeService) {
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "list-volume-binds", Method: http.MethodGet, Path: "/environments/{id}/volumes/binds", Tags: []string{"Volumes"}, Security: handlerutil.DefaultOperationSecurity()}, authz.PermVolumesRead, func(ctx context.Context, i *listBindsInput) (*handlerutil.Out[[]volume.Bind], error) {
		rows, err := s.ListBinds(ctx)
		if err != nil {
			return nil, err
		}
		return &handlerutil.Out[[]volume.Bind]{Body: base.ApiResponse[[]volume.Bind]{Success: true, Data: rows}}, nil
	})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "measure-volume-bind", Method: http.MethodPost, Path: "/environments/{id}/volumes/binds/size", Tags: []string{"Volumes"}, Security: handlerutil.DefaultOperationSecurity()}, authz.PermVolumesRead, func(ctx context.Context, i *measureBindInput) (*handlerutil.Out[volume.BindMeasurement], error) {
		result, err := s.MeasureBind(ctx, i.Body.Key)
		if err != nil {
			return nil, err
		}
		return &handlerutil.Out[volume.BindMeasurement]{Body: base.ApiResponse[volume.BindMeasurement]{Success: true, Data: result}}, nil
	})
}
