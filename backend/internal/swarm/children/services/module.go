// Source: getarcaneapp/arcane/backend/internal/swarm/children/services/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterRoutes registers the swarm service endpoints.
func RegisterRoutes(api huma.API, h *Handler) {
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "list-swarm-services",
			Method:      http.MethodGet,
			Path:        "/environments/{id}/swarm/services",
			Summary:     "List swarm services",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmRead,
		h.ListServices,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "get-swarm-service",
			Method:      http.MethodGet,
			Path:        "/environments/{id}/swarm/services/{serviceId}",
			Summary:     "Get swarm service",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmRead,
		h.GetService,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "create-swarm-service",
			Method:      http.MethodPost,
			Path:        "/environments/{id}/swarm/services",
			Summary:     "Create swarm service",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmServices,
		h.CreateService,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "update-swarm-service",
			Method:      http.MethodPut,
			Path:        "/environments/{id}/swarm/services/{serviceId}",
			Summary:     "Update swarm service",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmServices,
		h.UpdateService,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "delete-swarm-service",
			Method:      http.MethodDelete,
			Path:        "/environments/{id}/swarm/services/{serviceId}",
			Summary:     "Delete swarm service",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmServices,
		h.DeleteService,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "list-swarm-service-tasks",
			Method:      http.MethodGet,
			Path:        "/environments/{id}/swarm/services/{serviceId}/tasks",
			Summary:     "List tasks for a swarm service",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmRead,
		h.ListServiceTasks,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "rollback-swarm-service",
			Method:      http.MethodPost,
			Path:        "/environments/{id}/swarm/services/{serviceId}/rollback",
			Summary:     "Rollback a swarm service",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmServices,
		h.RollbackService,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "scale-swarm-service",
			Method:      http.MethodPost,
			Path:        "/environments/{id}/swarm/services/{serviceId}/scale",
			Summary:     "Scale a swarm service",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmServices,
		h.ScaleService,
	)
}
