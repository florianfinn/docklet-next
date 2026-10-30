// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation; typed UI-configuration refusal producer.

package autoheal

import (
	"context"
	"encoding/json/v2"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	dto "github.com/florianfinn/docklet-next/types/autoheal"
	environmentDTO "github.com/florianfinn/docklet-next/types/environment"
	"net/http"
	"reflect"
)

type Module struct {
	settings *settings.SettingsService
	proxy    handlerutil.RemoteJSONProxy
	config   *config.Config
}

func New(s *settings.SettingsService, env *environment.EnvironmentService, cfg *config.Config) *Module {
	return &Module{settings: s, proxy: env.ProxyJSONRequest, config: cfg}
}

type getInput struct {
	ID string `path:"id"`
}
type putInput struct {
	RawBody []byte
	ID      string `path:"id"`
}
type output struct{ Body dto.Settings }

func problem(err error) error {
	if p, ok := errors.AsType[*dto.Problem](err); ok {
		return p
	}
	return huma.Error500InternalServerError("autoheal.settings-unavailable")
}
func (m *Module) RegisterRoutes(api huma.API) {
	if m == nil {
		m = &Module{}
	}
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "get-auto-heal-settings", Method: http.MethodGet, Path: "/environments/{id}/auto-heal", Tags: []string{"Auto Heal"}, Security: handlerutil.DefaultOperationSecurity(), Middlewares: middleware.RequireGlobalAdmin(api)}, authz.PermSettingsRead, m.get)
	inputSchema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.Update](), true, "")
	requestBody := &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{"application/json": {Schema: inputSchema}}}
	middleware.RegisterWithPermission(api, huma.Operation{RequestBody: requestBody, OperationID: "update-auto-heal-settings", SkipValidateBody: true, Method: http.MethodPut, Path: "/environments/{id}/auto-heal", Tags: []string{"Auto Heal"}, Security: handlerutil.DefaultOperationSecurity(), Middlewares: middleware.RequireGlobalAdmin(api)}, authz.PermSettingsWrite, m.put)
	failureSchema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.Problem](), true, "")
	genericSchema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[huma.ErrorModel](), true, "")
	for _, op := range []*huma.Operation{api.OpenAPI().Paths["/environments/{id}/auto-heal"].Get, api.OpenAPI().Paths["/environments/{id}/auto-heal"].Put} {
		for _, status := range []string{"400", "403", "409", "502"} {
			op.Responses[status] = &huma.Response{Description: "Auto Heal or framework refusal", Content: map[string]*huma.MediaType{"application/problem+json": {Schema: &huma.Schema{OneOf: []*huma.Schema{failureSchema, genericSchema}}}}}
		}
	}
}
func (m *Module) get(ctx context.Context, in *getInput) (*output, error) {
	if in.ID != "0" {
		v, err := m.remote(ctx, in.ID, http.MethodGet, nil)
		if err != nil {
			return nil, err
		}
		return &output{Body: *v}, nil
	}
	v, err := m.settings.AutoHealSettings(ctx)
	if err != nil {
		return nil, problem(err)
	}
	return &output{Body: v}, nil
}
func (m *Module) put(ctx context.Context, in *putInput) (*output, error) {
	var body dto.Update
	if len(in.RawBody) > 0 {
		if err := json.Unmarshal(in.RawBody, &body, json.RejectUnknownMembers(true)); err != nil {
			p := &dto.Problem{Code: "autoheal.settings-invalid", Status: 400}
			if semantic, ok := errors.AsType[*json.SemanticError](err); ok {
				p.Field = string(semantic.JSONPointer)
			}
			return nil, p
		}
	}
	if err := body.Validate(); err != nil {
		return nil, problem(err)
	}
	if in.ID != "0" {
		v, err := m.remote(ctx, in.ID, http.MethodPut, body)
		if err != nil {
			return nil, err
		}
		return &output{Body: *v}, nil
	}
	if m.config != nil && m.config.UIConfigurationDisabled {
		return nil, &environmentDTO.HostProblem{Code: "settings.ui-configuration-disabled", Status: 403}
	}
	v, err := m.settings.UpdateAutoHeal(ctx, body)
	if err != nil {
		return nil, problem(err)
	}
	return &output{Body: v}, nil
}

func (m *Module) remote(ctx context.Context, id, method string, request any) (*dto.Settings, error) {
	var body []byte
	var err error
	if request != nil {
		body, err = json.Marshal(request)
		if err != nil {
			return nil, problem(err)
		}
	}
	var result dto.Settings
	if err = m.proxy(ctx, id, method, "/api/environments/0/auto-heal", body, &result); err == nil {
		return &result, nil
	}
	if access := handlerutil.AgentAccess(err); access != nil {
		return nil, access
	}
	if status, ok := errors.AsType[*remenv.StatusError](err); ok {
		var p dto.Problem
		if json.Unmarshal(status.Body, &p) == nil && p.Status == status.StatusCode && (p.Code == "autoheal.settings-invalid" || p.Code == "autoheal.settings-env-managed") {
			return nil, &p
		}
	}
	return nil, handlerutil.TranslateRemoteProxyError(err)
}
