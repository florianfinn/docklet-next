// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original administrator module management and agent cleanup surface.

package modules

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/modules"
	"reflect"
)

type DeleteInput struct {
	ID features.ID `path:"id"`
}
type HostDeleteInput struct {
	EnvironmentID string      `path:"id"`
	ID            features.ID `path:"module"`
}

// RegisterRoutes always registers management, including while every module is off.
func RegisterRoutes(api huma.API, service *Service, hosts *environment.EnvironmentService, agent bool) {
	if !agent {
		middleware.RegisterWithPermission(api, huma.Operation{OperationID: "list-modules", Method: "GET", Path: "/modules", Tags: []string{"Modules"}}, authz.PermSettingsRead, func(ctx context.Context, _ *struct{}) (*handlerutil.Out[[]dto.State], error) {
			states := make([]dto.State, 0, len(features.Modules()))
			for _, d := range features.Modules() {
				states = append(states, dto.State{ID: d.ID, Enabled: service.Enabled(ctx, d.ID), Kind: d.Kind})
			}
			return out(states), nil
		})
		middleware.RegisterWithPermission(api, huma.Operation{OperationID: "delete-module-data", Method: "DELETE", Path: "/modules/{id}/data", Tags: []string{"Modules"}}, authz.PermSettingsWrite, func(ctx context.Context, input *DeleteInput) (*handlerutil.Out[dto.DeleteResult], error) {
			if d, ok := features.Lookup(input.ID); !ok || d.Kind != features.Module {
				return nil, huma.Error404NotFound("Not Found")
			}
			result := dto.DeleteResult{PendingCleanup: []dto.PendingCleanup{}}
			err := service.deleteData(ctx, input.ID, func() error {
				if hosts == nil {
					return nil
				}
				var environments []environment.Environment
				if err := hosts.GetDB().WithContext(ctx).Where("id <> ?", "0").Order("id").Find(&environments).Error; err != nil {
					return err
				}
				for _, env := range environments {
					reason := "agent.unavailable"
					if env.Enabled {
						path := "/api/environments/0/modules/" + string(input.ID) + "/data"
						body, status, err := hosts.ProxyRequest(ctx, env.ID, "DELETE", path, nil)
						if err == nil && status >= 200 && status < 300 {
							continue
						}
						if err != nil {
							_, reason = remenv.AgentAccessFailure(err)
						} else if code := refusalCode(body); code != "" {
							reason = code
						}
					}
					result.PendingCleanup = append(result.PendingCleanup, dto.PendingCleanup{ID: env.ID, Name: env.Name, Reason: reason})
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			return out(result), nil
		})
	}
	hostDelete := huma.Operation{OperationID: "delete-host-module-data", Method: "DELETE", Path: "/environments/{id}/modules/{module}/data", Tags: []string{"Modules"}}
	if !agent {
		// The hub deletes its own host data only through the management route, which
		// also asks every agent; the local host form stays absent after access checks.
		hostDelete.Middlewares = huma.Middlewares{func(ctx huma.Context, _ func(huma.Context)) { _ = middleware.APINotFound(humaecho.Unwrap(ctx)) }}
	}
	middleware.RegisterWithPermission(api, hostDelete, authz.PermSettingsWrite, func(ctx context.Context, input *HostDeleteInput) (*handlerutil.Out[dto.DeleteResult], error) {
		if d, ok := features.Lookup(input.ID); !ok || d.Kind != features.Module {
			return nil, huma.Error404NotFound("Not Found")
		}
		if err := service.DeleteData(ctx, input.ID); err != nil {
			return nil, err
		}
		return out(dto.DeleteResult{PendingCleanup: []dto.PendingCleanup{}}), nil
	})
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.Problem](), true, "")
	for _, p := range []string{"/modules/{id}/data", "/environments/{id}/modules/{module}/data"} {
		if item := api.OpenAPI().Paths[p]; item != nil {
			item.Delete.Responses["409"] = &huma.Response{Description: "Module must be off and idle", Content: map[string]*huma.MediaType{"application/problem+json": {Schema: schema}}}
		}
	}
}

// refusalCode keeps a reachable agent's bounded module refusal as the cleanup reason.
func refusalCode(body []byte) string {
	var problem dto.Problem
	if json.Unmarshal(body, &problem) != nil {
		return ""
	}
	switch problem.Code {
	case "module-on", "module-busy", "feature-disabled":
		return problem.Code
	}
	return ""
}

func out[T any](data T) *handlerutil.Out[T] {
	return &handlerutil.Out[T]{Body: base.ApiResponse[T]{Success: true, Data: data}}
}
