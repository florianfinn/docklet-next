// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original administrator endpoints for the tailnet route (status, credential, reset).

package tailnet

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

// Module registers the hub-only routes; it is not part of the agent route allowlist.
type Module struct {
	service *Service
}

func NewModule(service *Service) *Module { return &Module{service: service} }

type statusOutput = handlerutil.Out[tailnettypes.Status]
type settingsOutput = handlerutil.Out[tailnettypes.Settings]
type emptyInput struct{}
type updateInput struct {
	Body tailnettypes.SettingsUpdate
}

func (m *Module) RegisterRoutes(api huma.API) {
	if m == nil {
		m = &Module{}
	}
	tags := []string{"Tailnet"}
	ops := []struct {
		op   huma.Operation
		perm string
	}{
		{huma.Operation{OperationID: "get-tailnet-status", Method: http.MethodGet, Path: "/tailnet/status", Summary: "Hub tailnet status"}, authz.PermSettingsRead},
		{huma.Operation{OperationID: "get-tailnet-settings", Method: http.MethodGet, Path: "/settings/connections/tailnet", Summary: "Tailnet route settings"}, authz.PermSettingsRead},
		{huma.Operation{OperationID: "update-tailnet-settings", Method: http.MethodPut, Path: "/settings/connections/tailnet", Summary: "Replace the tailnet credential"}, authz.PermSettingsWrite},
		{huma.Operation{OperationID: "reset-tailnet", Method: http.MethodPost, Path: "/tailnet/reset", Summary: "Delete the hub's local tailnet identity"}, authz.PermSettingsWrite},
	}
	for i := range ops {
		ops[i].op.Tags = tags
		ops[i].op.Security = handlerutil.DefaultOperationSecurity()
		ops[i].op.Middlewares = middleware.RequireGlobalAdmin(api)
	}
	middleware.RegisterWithPermission(api, ops[0].op, ops[0].perm, m.status)
	middleware.RegisterWithPermission(api, ops[1].op, ops[1].perm, m.settings)
	middleware.RegisterWithPermission(api, ops[2].op, ops[2].perm, m.update)
	middleware.RegisterWithPermission(api, ops[3].op, ops[3].perm, m.reset)
	ProblemResponses(api, api.OpenAPI().Paths["/settings/connections/tailnet"].Put, "422")
}

// ProblemResponses documents typed tailnet refusals next to framework errors on an operation.
func ProblemResponses(api huma.API, op *huma.Operation, extra ...string) {
	if op == nil {
		return
	}
	problem := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[tailnettypes.Problem](), true, "")
	generic := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[huma.ErrorModel](), true, "")
	if op.Responses == nil {
		op.Responses = map[string]*huma.Response{}
	}
	for _, status := range append([]string{"409", "502"}, extra...) {
		op.Responses[status] = &huma.Response{Description: "Tailnet or framework refusal", Content: map[string]*huma.MediaType{"application/problem+json": {Schema: &huma.Schema{OneOf: []*huma.Schema{problem, generic}}}}}
	}
}

func (m *Module) unavailable() error {
	if m.service == nil {
		return huma.Error404NotFound("The tailnet route is available on the hub only")
	}
	return nil
}

func (m *Module) status(ctx context.Context, _ *emptyInput) (*statusOutput, error) {
	if err := m.unavailable(); err != nil {
		return nil, err
	}
	return &statusOutput{Body: base.ApiResponse[tailnettypes.Status]{Success: true, Data: m.service.Status(ctx)}}, nil
}

func (m *Module) settings(ctx context.Context, _ *emptyInput) (*settingsOutput, error) {
	if err := m.unavailable(); err != nil {
		return nil, err
	}
	out, err := m.service.Settings(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Tailnet settings unavailable", "error", err)
		return nil, huma.Error500InternalServerError("Tailnet settings unavailable")
	}
	return &settingsOutput{Body: base.ApiResponse[tailnettypes.Settings]{Success: true, Data: out}}, nil
}

func (m *Module) update(ctx context.Context, in *updateInput) (*settingsOutput, error) {
	if err := m.unavailable(); err != nil {
		return nil, err
	}
	out, err := m.service.Update(ctx, in.Body)
	if errors.Is(err, ErrInvalidCredential) {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	if err != nil {
		slog.ErrorContext(ctx, "Tailnet settings update failed", "error", Redact(err.Error()))
		return nil, huma.Error500InternalServerError("Tailnet settings update failed")
	}
	return &settingsOutput{Body: base.ApiResponse[tailnettypes.Settings]{Success: true, Data: out}}, nil
}

func (m *Module) reset(ctx context.Context, _ *emptyInput) (*statusOutput, error) {
	if err := m.unavailable(); err != nil {
		return nil, err
	}
	out, err := m.service.Reset(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Tailnet reset failed", "error", err)
		return nil, huma.Error500InternalServerError("Tailnet reset failed")
	}
	return &statusOutput{Body: base.ApiResponse[tailnettypes.Status]{Success: true, Data: out}}, nil
}
