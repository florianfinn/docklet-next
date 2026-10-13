// Source: getarcaneapp/arcane/backend/internal/swarm/children/secrets/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package secrets

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterRoutes registers the swarm secret endpoints.
func RegisterRoutes(api huma.API, h *Handler) {
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "list-swarm-secrets",
			Method:      http.MethodGet,
			Path:        "/environments/{id}/swarm/secrets",
			Summary:     "List swarm secrets",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmRead,
		h.ListSecrets,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "get-swarm-secret",
			Method:      http.MethodGet,
			Path:        "/environments/{id}/swarm/secrets/{secretId}",
			Summary:     "Get swarm secret",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmRead,
		h.GetSecret,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "create-swarm-secret",
			Method:      http.MethodPost,
			Path:        "/environments/{id}/swarm/secrets",
			Summary:     "Create swarm secret",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmSecrets,
		h.CreateSecret,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "delete-swarm-secret",
			Method:      http.MethodDelete,
			Path:        "/environments/{id}/swarm/secrets/{secretId}",
			Summary:     "Delete swarm secret",
			Tags: []string{
				"Swarm",
			},
			Security: handlerutil.DefaultOperationSecurity(),
		},
		authz.PermSwarmSecrets,
		h.DeleteSecret,
	)
}
