// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original proxy module API and lifecycle wiring.

package proxy

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/features"
	moduledto "github.com/florianfinn/docklet-next/types/modules"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"reflect"
)

type HostInput struct {
	EnvironmentID string `path:"id"`
}
type HostInstanceInput struct {
	HostInput
	Body dto.Instance
}
type HostApplyInput struct {
	HostInput
	Body dto.ApplyRequest
}
type HostQueryInput struct {
	HostInput
	OperationID  string `path:"operationId"`
	Generation   string `query:"generation"`
	Registration string `query:"registration"`
	InstanceID   string `query:"instanceId"`
}
type HostFenceInput struct {
	HostInput
	Body dto.Query
}
type HostReleaseInput struct {
	HostInput
	Body dto.ReleaseRequest
}
type InstanceInput struct {
	InstanceID string `path:"instanceId"`
}
type CreateInstanceInput struct{ Body dto.CreateInstanceRequest }
type UpdateInstanceInput struct {
	InstanceInput
	Body dto.Settings
}
type CreateOperationInput struct {
	InstanceInput
	Body dto.OperationRequest
}
type OperationInput struct {
	OperationID string `path:"operationId"`
}
type ReleaseInput struct {
	OperationInput
	Body struct {
		Confirmed dto.FileState `json:"confirmed"`
	}
}

func out[T any](v T, err error) (*handlerutil.Out[T], error) {
	if err != nil {
		if missing(err) {
			return nil, huma.Error404NotFound("Not Found")
		}
		return nil, err
	}
	return &handlerutil.Out[T]{Body: base.ApiResponse[T]{Success: true, Data: v}}, nil
}
func register[I, O any](api huma.API, m *Module, agent bool, id, method, p string, fn func(context.Context, *I) (*handlerutil.Out[O], error)) {
	var s middleware.ModuleSettings
	if m != nil && m.Settings != nil {
		s = m.Settings
	}
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: id, Method: method, Path: p, Tags: []string{"Proxy"}, Security: handlerutil.DefaultOperationSecurity(), Middlewares: huma.Middlewares{middleware.ModuleGate(api, s, agent, features.Proxy), hubAdmissionResponse(s, agent)}}, authz.PermSettingsWrite, fn)
}
func (m *Module) RegisterRoutes(api huma.API, agent bool) {
	const host = "/environments/{id}/modules/proxy"
	register(api, m, agent, "discover-proxy-containers", "GET", host+"/discover", func(ctx context.Context, _ *HostInput) (*handlerutil.Out[[]dto.Discovery], error) {
		return out(m.Runtime.Discover(ctx))
	})
	register(api, m, agent, "get-proxy-inventory", "POST", host+"/inventory", func(ctx context.Context, i *HostInstanceInput) (*handlerutil.Out[dto.Inventory], error) {
		return out(m.Inventory(ctx, i.Body))
	})
	register(api, m, agent, "read-proxy-configuration", "POST", host+"/read", func(ctx context.Context, i *HostInstanceInput) (*handlerutil.Out[dto.ReadResult], error) {
		return out(m.Read(ctx, i.Body))
	})
	register(api, m, agent, "apply-proxy-operation", "POST", host+"/apply", func(ctx context.Context, i *HostApplyInput) (*handlerutil.Out[dto.Operation], error) {
		return out(m.Agent.Apply(ctx, i.Body))
	})
	register(api, m, agent, "get-host-proxy-operation", "GET", host+"/operations/{operationId}", func(ctx context.Context, i *HostQueryInput) (*handlerutil.Out[dto.Operation], error) {
		return out(m.Agent.Query(ctx, dto.Query{ID: i.OperationID, Generation: i.Generation, Registration: i.Registration, Instance: dto.Instance{ID: i.InstanceID}}))
	})
	register(api, m, agent, "fence-proxy-operation", "POST", host+"/fence", func(ctx context.Context, i *HostFenceInput) (*handlerutil.Out[dto.FenceResult], error) {
		return out(m.Agent.Fence(ctx, i.Body))
	})
	register(api, m, agent, "release-host-proxy-operation", "POST", host+"/release", func(ctx context.Context, i *HostReleaseInput) (*handlerutil.Out[dto.Operation], error) {
		return out(m.Agent.Release(ctx, i.Body))
	})
	// Cleanup is S1's /environments/{id}/modules/{module}/data management
	// route. It calls the registered hook even while the module is disabled.
	if !agent {
		const hub = "/modules/proxy"
		register(api, m, false, "discover-proxy-instances", "GET", hub+"/discover", func(ctx context.Context, i *struct {
			EnvironmentID string `query:"environmentId" required:"true"`
		}) (*handlerutil.Out[[]dto.Discovery], error) {
			return out(m.Discover(ctx, i.EnvironmentID))
		})
		register(api, m, false, "list-proxy-instances", "GET", hub+"/instances", func(ctx context.Context, _ *struct{}) (*handlerutil.Out[[]dto.Instance], error) {
			return out(m.Instances(ctx))
		})
		register(api, m, false, "create-proxy-instance", "POST", hub+"/instances", func(ctx context.Context, i *CreateInstanceInput) (*handlerutil.Out[dto.Instance], error) {
			return out(m.CreateInstance(ctx, i.Body))
		})
		register(api, m, false, "get-proxy-instance", "GET", hub+"/instances/{instanceId}", func(ctx context.Context, i *InstanceInput) (*handlerutil.Out[dto.Instance], error) {
			return out(m.Instance(ctx, i.InstanceID))
		})
		register(api, m, false, "update-proxy-instance", "PUT", hub+"/instances/{instanceId}", func(ctx context.Context, i *UpdateInstanceInput) (*handlerutil.Out[dto.Instance], error) {
			return out(m.UpdateInstance(ctx, i.InstanceID, i.Body))
		})
		register(api, m, false, "remove-proxy-instance", "DELETE", hub+"/instances/{instanceId}", func(ctx context.Context, i *InstanceInput) (*handlerutil.Out[bool], error) {
			return out(true, m.RemoveInstance(ctx, i.InstanceID))
		})
		register(api, m, false, "create-proxy-operation", "POST", hub+"/instances/{instanceId}/operations", func(ctx context.Context, i *CreateOperationInput) (*handlerutil.Out[dto.Operation], error) {
			return out(m.CreateOperation(ctx, i.InstanceID, i.Body))
		})
		register(api, m, false, "list-proxy-operations", "GET", hub+"/instances/{instanceId}/operations", func(ctx context.Context, i *InstanceInput) (*handlerutil.Out[[]dto.Operation], error) {
			return out(m.Operations(ctx, i.InstanceID))
		})
		register(api, m, false, "get-proxy-operation", "GET", hub+"/operations/{operationId}", func(ctx context.Context, i *OperationInput) (*handlerutil.Out[dto.Operation], error) {
			return out(m.Hub.Query(ctx, i.OperationID))
		})
		register(api, m, false, "release-proxy-operation", "POST", hub+"/operations/{operationId}/release", func(ctx context.Context, i *ReleaseInput) (*handlerutil.Out[dto.Operation], error) {
			actor, err := handlerutil.RequireUser(ctx)
			if err != nil {
				return nil, err
			}
			return out(m.Hub.Release(ctx, i.OperationID, i.Body.Confirmed, actor.ID))
		})
	}
	problem := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.Problem](), true, "")
	moduleProblem := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[moduledto.Problem](), true, "")
	frameworkProblem := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[huma.ErrorModel](), true, "")
	for _, item := range api.OpenAPI().Paths {
		for _, op := range []*huma.Operation{item.Get, item.Post, item.Put, item.Delete} {
			if op == nil || len(op.Tags) != 1 || op.Tags[0] != "Proxy" {
				continue
			}
			for _, status := range []string{"403", "409", "422", "503"} {
				op.Responses[status] = &huma.Response{Description: "Proxy refusal", Content: map[string]*huma.MediaType{"application/problem+json": {Schema: &huma.Schema{OneOf: []*huma.Schema{problem, moduleProblem, frameworkProblem}}}}}
			}
		}
	}
}
