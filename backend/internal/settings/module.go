// Source: getarcaneapp/arcane/backend/internal/settings/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; M19 backend contracts and target identity integration.

// Package settings owns persisted application settings and their HTTP surface.
package settings

import (
	dto "github.com/florianfinn/docklet-next/types/settings"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// Module joins the settings service with its route dependencies.
type Module struct {
	service         *SettingsService
	proxyRemoteJSON handlerutil.RemoteJSONProxy
	config          *config.Config
}

// New builds the settings domain around its initialized service.
func New(service *SettingsService, proxyRemoteJSON handlerutil.RemoteJSONProxy, cfg *config.Config) *Module {
	return &Module{service: service, proxyRemoteJSON: proxyRemoteJSON, config: cfg}
}

// Service exposes settings operations to collaborating domains.
func (m *Module) Service() *SettingsService {
	if m == nil {
		return nil
	}
	return m.service
}

// RegisterRoutes mounts settings endpoints for runtime and schema discovery.
func (m *Module) RegisterRoutes(api huma.API) {
	if m == nil {
		RegisterSettings(api, nil, nil, nil)
		return
	}
	RegisterSettings(api, m.service, m.proxyRemoteJSON, m.config)
}

// RegisterSettings registers settings management routes using Huma.
func RegisterSettings(api huma.API, settingsService *SettingsService, proxyRemoteJSON handlerutil.RemoteJSONProxy, cfg *config.Config) {
	h := &SettingsHandler{
		settingsService: settingsService,
		proxyRemoteJSON: proxyRemoteJSON,
		cfg:             cfg,
	}

	// Environment-scoped settings endpoints
	huma.Register(api, huma.Operation{
		OperationID: "get-public-settings",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/settings/public",
		Summary:     "Get public settings",
		Description: "Get all public settings for an environment",
		Tags:        []string{"Settings"},
		Security:    []map[string][]string{},
	}, h.GetPublicSettings)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "get-settings",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/settings",
		Summary:     "Get settings",
		Description: "Get all settings for an environment",
		Tags:        []string{"Settings"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermSettingsRead, h.GetSettings)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "update-settings",
		Method:      http.MethodPut,
		Path:        "/environments/{id}/settings",
		Summary:     "Update settings",
		Description: "Update settings for an environment",
		Tags:        []string{"Settings"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermSettingsWrite, h.UpdateSettings)

	ref := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.Problem](), true, "")
	api.OpenAPI().Paths["/environments/{id}/settings"].Put.Responses["400"] = &huma.Response{Description: "Invalid setting", Content: map[string]*huma.MediaType{"application/problem+json": {Schema: ref}}}
	handlerutil.HostProblemResponses(api, api.OpenAPI().Paths["/environments/{id}/settings"].Put, "403", "502")

}
