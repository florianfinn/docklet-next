// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original wiring of fresh container and host readers for D6.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/container"
)

func operationCapabilityReaders(d HandlerDeps) activity.OperationCapabilityReaders {
	containers, hosts := d.Container.Service(), d.Environment.Service()
	return activity.OperationCapabilityReaders{
		GuardOrigin: containers.GuardRuntimeOrigin,
		HostReason: func(ctx context.Context, env string) string {
			if env == "0" {
				return ""
			}
			host, err := hosts.GetEnvironmentByID(ctx, env)
			if err != nil || !host.Enabled {
				return "agent.unavailable"
			}
			switch host.Status {
			case "token_rejected":
				return "agent.invalid-token"
			case "update-required":
				return "agent.update-required"
			case "offline", "error", "pending":
				return "agent.unavailable"
			}
			return ""
		},
		Origin: func(ctx context.Context, env, id string) (dto.OriginFields, error) {
			var response base.ApiResponse[dto.Details]
			if env == "0" {
				inspect, err := containers.GetContainerByID(ctx, id)
				if err != nil {
					return dto.OriginFields{}, err
				}
				if inspect == nil || inspect.Config == nil {
					return dto.OriginFields{}, activity.ErrRuntimeTransport
				}
				response.Success = true
				response.Data = dto.Details{ID: inspect.ID, Name: inspect.Name, Labels: inspect.Config.Labels, OriginFields: origin.Detect(inspect.Config.Labels)}
			} else {
				if err := hosts.ProxyJSONRequest(ctx, env, http.MethodGet, "/api/environments/0/containers/"+url.PathEscape(id), nil, &response); err != nil {
					return dto.OriginFields{}, err
				}
				if !response.Success || response.Data.ID == "" {
					return dto.OriginFields{}, activity.ErrRuntimeTransport
				}
			}
			body, err := json.Marshal(response)
			if err != nil {
				return dto.OriginFields{}, err
			}
			body, err = containers.OverlayInventory(ctx, env, body)
			if err != nil {
				return dto.OriginFields{}, err
			}
			if err = json.Unmarshal(body, &response); err != nil {
				return dto.OriginFields{}, err
			}
			return dto.OriginFields{Origin: origin.Detect(response.Data.Labels).Origin, OriginSource: response.Data.OriginSource}, nil
		},
	}
}
