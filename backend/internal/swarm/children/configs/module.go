// Source: getarcaneapp/arcane/backend/internal/swarm/children/configs/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package configs

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterRoutes registers the swarm config endpoints.
func RegisterRoutes(api huma.API, h *Handler) {
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "list-swarm-configs",
			Method:      http.MethodGet,
			Path:        "/environments/{id}/swarm/configs",
			Summary:     "List swarm configs",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmRead,
		h.ListConfigs,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "get-swarm-config",
			Method:      http.MethodGet,
			Path:        "/environments/{id}/swarm/configs/{configId}",
			Summary:     "Get swarm config",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmRead,
		h.GetConfig,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "create-swarm-config",
			Method:      http.MethodPost,
			Path:        "/environments/{id}/swarm/configs",
			Summary:     "Create swarm config",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmConfigs,
		h.CreateConfig,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "delete-swarm-config",
			Method:      http.MethodDelete,
			Path:        "/environments/{id}/swarm/configs/{configId}",
			Summary:     "Delete swarm config",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmConfigs,
		h.DeleteConfig,
	)
}
