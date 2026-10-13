// Source: getarcaneapp/arcane/backend/internal/system/children/upgrade/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package upgrade

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterRoutes registers the self-upgrade and update-all endpoints.
func RegisterRoutes(api huma.API, h *Handler) {
	RegisterAgentRoutes(api, h)
	registerUpdateAllRoutes(api, h)
}

// RegisterAgentRoutes exposes only checks and self-upgrade for one agent.
func RegisterAgentRoutes(api huma.API, h *Handler) {
	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "check-upgrade",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/system/upgrade/check",
		Summary:     "Check for system upgrade",
		Description: "Check if a system upgrade is available",
		Tags:        []string{"System"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermSystemRead, h.CheckUpgradeAvailable)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID:   "trigger-upgrade",
		Method:        http.MethodPost,
		Path:          "/environments/{id}/system/upgrade",
		Summary:       "Trigger system upgrade",
		Description:   "Trigger a system upgrade",
		DefaultStatus: http.StatusAccepted,
		Tags:          []string{"System"},
		Security:      handlerutil.DefaultOperationSecurity(),
	}, authz.PermSystemUpgrade, h.TriggerUpgrade)

}

func registerUpdateAllRoutes(api huma.API, h *Handler) {

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID:   "trigger-update-all",
		Method:        http.MethodPost,
		Path:          "/environments/{id}/system/upgrade/all",
		Summary:       "Update all environments",
		Description:   "Upgrade every Arcane environment, starting with the manager",
		DefaultStatus: http.StatusAccepted,
		Tags:          []string{"System"},
		Security:      handlerutil.DefaultOperationSecurity(),
	}, authz.PermSystemUpgrade, h.TriggerUpdateAll)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "update-all-status",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/system/upgrade/all/status",
		Summary:     "Get update-all status",
		Description: "Get the status of the latest update-all-environments job",
		Tags:        []string{"System"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermSystemRead, h.GetUpdateAllStatus)
}
