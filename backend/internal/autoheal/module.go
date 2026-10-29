// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package autoheal

import (
	"context"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	dto "github.com/florianfinn/docklet-next/types/autoheal"
	"net/http"
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
	ID   string `path:"id"`
	Body dto.Update
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
	huma.Register(api, huma.Operation{OperationID: "get-auto-heal-settings", Method: http.MethodGet, Path: "/environments/{id}/auto-heal", Tags: []string{"Auto Heal"}, Security: handlerutil.DefaultOperationSecurity(), Middlewares: middleware.RequireGlobalAdmin(api)}, m.get)
	huma.Register(api, huma.Operation{OperationID: "update-auto-heal-settings", Method: http.MethodPut, Path: "/environments/{id}/auto-heal", Tags: []string{"Auto Heal"}, Security: handlerutil.DefaultOperationSecurity(), Middlewares: middleware.RequireGlobalAdmin(api)}, m.put)
}
func (m *Module) get(ctx context.Context, in *getInput) (*output, error) {
	if in.ID != "0" {
		v, err := m.proxy.JSON[dto.Settings](ctx, in.ID, http.MethodGet, "/api/environments/0/auto-heal", nil)
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
	if err := in.Body.Validate(); err != nil {
		return nil, problem(err)
	}
	if in.ID != "0" {
		v, err := m.proxy.JSON[dto.Settings](ctx, in.ID, http.MethodPut, "/api/environments/0/auto-heal", in.Body)
		if err != nil {
			return nil, err
		}
		return &output{Body: *v}, nil
	}
	if m.config != nil && m.config.UIConfigurationDisabled {
		return nil, huma.Error403Forbidden("settings.ui-disabled")
	}
	v, err := m.settings.UpdateAutoHeal(ctx, in.Body)
	if err != nil {
		return nil, problem(err)
	}
	return &output{Body: v}, nil
}
