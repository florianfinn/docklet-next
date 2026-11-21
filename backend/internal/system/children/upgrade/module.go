// Source: getarcaneapp/arcane/backend/internal/system/children/upgrade/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; remove fleet upgrade routes, retain authenticated recovery commands; hub-only release list refresh.

package upgrade

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterRoutes registers the Compose self-update endpoints and the hub-only
// release list refresh, which stays outside the frozen agent contract.
func RegisterRoutes(api huma.API, h *Handler) {
	RegisterAgentRoutes(api, h)
	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID:   "refresh-releases",
		Method:        http.MethodPost,
		Path:          "/system/releases/refresh",
		Summary:       "Refresh the release list",
		Description:   "Fetch the release list again before the next update check (at most every 30 seconds)",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"System"},
		Security:      handlerutil.DefaultOperationSecurity(),
	}, authz.PermSystemRead, h.RefreshReleases)
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
