// Source: getarcaneapp/arcane/backend/internal/swarm/children/stacks/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package stacks

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterRoutes registers the swarm stack endpoints.
func RegisterRoutes(api huma.API, h *Handler) {
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "list-swarm-stacks",
			Method:      http.MethodGet,
			Path:        "/environments/{id}/swarm/stacks",
			Summary:     "List swarm stacks",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmRead,
		h.ListStacks,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "deploy-swarm-stack",
			Method:      http.MethodPost,
			Path:        "/environments/{id}/swarm/stacks",
			Summary:     "Deploy swarm stack",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmStacks,
		h.DeployStack,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "get-swarm-stack",
			Method:      http.MethodGet,
			Path:        "/environments/{id}/swarm/stacks/{name}",
			Summary:     "Get swarm stack",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmRead,
		h.GetStack,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "get-swarm-stack-source",
			Method:      http.MethodGet,
			Path:        "/environments/{id}/swarm/stacks/{name}/source",
			Summary:     "Get swarm stack source",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmStacks,
		h.GetStackSource,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "update-swarm-stack-source",
			Method:      http.MethodPut,
			Path:        "/environments/{id}/swarm/stacks/{name}/source",
			Summary:     "Update swarm stack source",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmStacks,
		h.UpdateStackSource,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "delete-swarm-stack",
			Method:      http.MethodDelete,
			Path:        "/environments/{id}/swarm/stacks/{name}",
			Summary:     "Delete swarm stack",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmStacks,
		h.DeleteStack,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "list-swarm-stack-services",
			Method:      http.MethodGet,
			Path:        "/environments/{id}/swarm/stacks/{name}/services",
			Summary:     "List swarm stack services",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmRead,
		h.ListStackServices,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "list-swarm-stack-tasks",
			Method:      http.MethodGet,
			Path:        "/environments/{id}/swarm/stacks/{name}/tasks",
			Summary:     "List swarm stack tasks",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmRead,
		h.ListStackTasks,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "render-swarm-stack-config",
			Method:      http.MethodPost,
			Path:        "/environments/{id}/swarm/stacks/config/render",
			Summary:     "Render/validate swarm stack config",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmRead,
		h.RenderStackConfig,
	)
}
