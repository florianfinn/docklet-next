// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Dedicated hub schedule storage/projection, including offline registered hosts.

package job

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"net/http"
)

type ImageScheduleInput struct {
	EnvironmentID string `path:"id"`
}
type imageScheduleWriteInput struct {
	ImageScheduleInput
	Body dto.UpdateScheduleInput
}

func registerImagePolicyScheduleRoutes(api huma.API, s *JobService) {
	path := "/environments/{id}/updater/schedules"
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "get-update-schedules", Method: http.MethodGet, Path: path, Tags: []string{"Updater"}, Summary: "Read the two hub-owned host schedules without agent I/O"}, authz.PermJobsManage, func(ctx context.Context, in *ImageScheduleInput) (*handlerutil.Out[dto.UpdateScheduleConfig], error) {
		result, err := s.ImagePolicySchedules(ctx, in.EnvironmentID)
		return &handlerutil.Out[dto.UpdateScheduleConfig]{Body: base.ApiResponse[dto.UpdateScheduleConfig]{Success: err == nil, Data: result}}, err
	})
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "set-update-schedules", Method: http.MethodPut, Path: path, Tags: []string{"Updater"}, Summary: "Set hub-zone host schedules and recorded actor with expected revision"}, authz.PermJobsManage, func(ctx context.Context, in *imageScheduleWriteInput) (*handlerutil.Out[dto.UpdateScheduleConfig], error) {
		if s == nil || s.imagePolicies == nil {
			return nil, huma.Error503ServiceUnavailable("Update schedules unavailable")
		}
		actor, _ := middleware.GetUserIDFromContext(ctx)
		key, _ := ctx.Value(middleware.ContextKeyApiKeyID).(string)
		if err := s.imagePolicies.SaveImagePolicySchedule(ctx, in.EnvironmentID, in.Body, actor, key); err != nil {
			return nil, err
		}
		result, err := s.ImagePolicySchedules(ctx, in.EnvironmentID)
		return &handlerutil.Out[dto.UpdateScheduleConfig]{Body: base.ApiResponse[dto.UpdateScheduleConfig]{Success: err == nil, Data: result}}, err
	})
}
