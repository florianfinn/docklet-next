// Source: getarcaneapp/arcane/backend/internal/registry/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; M15 simple-user rights and identity contracts.

// Package registry owns registry credentials, digest inspection, pull usage,
// synchronization, and the registry HTTP surface.
package registry

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/registry/children/browse"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

type Module struct {
	service       *ContainerRegistryService
	handler       *ContainerRegistryHandler
	browseHandler *browse.Handler
}

func New(service *ContainerRegistryService, syncRemoteRegistries func(context.Context) error) *Module {
	return &Module{
		service:       service,
		handler:       NewHandler(service, syncRemoteRegistries),
		browseHandler: browse.NewHandler(service.browse),
	}
}

func (m *Module) Handler() *ContainerRegistryHandler {
	if m == nil {
		return nil
	}
	return m.handler
}

func (m *Module) Service() *ContainerRegistryService {
	if m == nil {
		return nil
	}
	return m.service
}

func (m *Module) RegisterRoutes(api huma.API) {
	if m == nil {
		RegisterContainerRegistries(api, NewHandler(nil, nil))
		browse.RegisterRoutes(api, browse.NewHandler(nil))
		return
	}
	RegisterContainerRegistries(api, m.handler)
	browse.RegisterRoutes(api, m.browseHandler)
}

// RegisterAgentRoutes accepts credential synchronization from the manager only.
func (m *Module) RegisterAgentRoutes(api huma.API) {
	var h *ContainerRegistryHandler
	if m != nil {
		h = m.handler
	}
	if h == nil {
		h = NewHandler(nil, nil)
	}
	registerSyncRoute(api, h)
}

func registerSyncRoute(api huma.API, h *ContainerRegistryHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "syncContainerRegistries",
		Method:      "POST",
		Path:        "/container-registries/sync",
		Summary:     "Sync container registries",
		Description: "Sync container registries from a remote source",
		Tags:        []string{"Container Registries"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: append(append(middleware.RequirePermission(api, authz.PermRegistriesUpdate), middleware.RequirePermission(api, authz.PermRegistriesCreate)...), middleware.RequirePermission(api, authz.PermRegistriesDelete)...),
	}, h.SyncRegistries)
}

func RegisterContainerRegistries(api huma.API, h *ContainerRegistryHandler) {
	huma.Register(api, huma.Operation{
		OperationID: "listContainerRegistries",
		Method:      "GET",
		Path:        "/container-registries",
		Summary:     "List container registries",
		Description: "Get a paginated list of container registries",
		Tags:        []string{"Container Registries"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequirePermission(api, authz.PermRegistriesList),
	}, h.ListRegistries)

	huma.Register(api, huma.Operation{
		OperationID: "createContainerRegistry",
		Method:      "POST",
		Path:        "/container-registries",
		Summary:     "Create a container registry",
		Description: "Create a new container registry",
		Tags:        []string{"Container Registries"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequirePermission(api, authz.PermRegistriesCreate),
	}, h.CreateRegistry)

	registerSyncRoute(api, h)

	huma.Register(api, huma.Operation{
		OperationID: "getContainerRegistryPullUsage",
		Method:      "GET",
		Path:        "/container-registries/pull-usage",
		Summary:     "Get container registry pull usage",
		Description: "Get configured registry pull usage and rate limit visibility",
		Tags:        []string{"Container Registries"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequirePermission(api, authz.PermRegistriesRead),
	}, h.GetPullUsage)

	huma.Register(api, huma.Operation{
		OperationID: "getContainerRegistry",
		Method:      "GET",
		Path:        "/container-registries/{id}",
		Summary:     "Get a container registry",
		Description: "Get a container registry by ID",
		Tags:        []string{"Container Registries"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequirePermission(api, authz.PermRegistriesRead),
	}, h.GetRegistry)

	huma.Register(api, huma.Operation{
		OperationID: "updateContainerRegistry",
		Method:      "PUT",
		Path:        "/container-registries/{id}",
		Summary:     "Update a container registry",
		Description: "Update an existing container registry",
		Tags:        []string{"Container Registries"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequirePermission(api, authz.PermRegistriesUpdate),
	}, h.UpdateRegistry)

	huma.Register(api, huma.Operation{
		OperationID: "deleteContainerRegistry",
		Method:      "DELETE",
		Path:        "/container-registries/{id}",
		Summary:     "Delete a container registry",
		Description: "Delete a container registry by ID",
		Tags:        []string{"Container Registries"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequirePermission(api, authz.PermRegistriesDelete),
	}, h.DeleteRegistry)

	huma.Register(api, huma.Operation{
		OperationID: "testContainerRegistry",
		Method:      "POST",
		Path:        "/container-registries/{id}/test",
		Summary:     "Test a container registry",
		Description: "Test connectivity and authentication to a container registry",
		Tags:        []string{"Container Registries"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequirePermission(api, authz.PermRegistriesTest),
	}, h.TestRegistry)
}
