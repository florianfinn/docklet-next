// Source: getarcaneapp/arcane/backend/internal/system/children/backup/children/volumes/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package volumes

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
)

// RegisterRoutes registers the system-managed volume backup endpoints.
func RegisterRoutes(api huma.API, h *Handler) {
	adminOnly := middleware.RequireGlobalAdmin(api)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "get-system-volume-backup-config",
			Method:      http.MethodGet,
			Path:        "/backups/volumes/config",
			Summary:     "Get system-managed volume backup policies",
			Tags: []string{
				"System Backups",
			},
			Middlewares: adminOnly,
		},
		authz.PermSystemBackupsRead,
		h.GetSystemVolumeConfig,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "update-system-volume-backup-config",
			Method:      http.MethodPut,
			Path:        "/backups/volumes/config",
			Summary:     "Update system-managed volume backup policies",
			Tags: []string{
				"System Backups",
			},
			Middlewares: adminOnly,
		},
		authz.PermSystemBackupsManage,
		h.UpdateSystemVolumeConfig,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID: "list-system-volume-backup-options",
			Method:      http.MethodGet,
			Path:        "/backups/volumes/options",
			Summary:     "List volumes available to system-managed backups",
			Tags: []string{
				"System Backups",
			},
			Middlewares: adminOnly,
		},
		authz.PermSystemBackupsRead,
		h.ListSystemVolumeOptions,
	)
	middleware.RegisterWithPermission(
		api,
		huma.Operation{
			OperationID:   "run-system-volume-backups",
			DefaultStatus: http.StatusAccepted,
			Method:        http.MethodPost,
			Path:          "/backups/volumes/run",
			Summary:       "Run system-managed volume backups",
			Tags: []string{
				"System Backups",
			},
			Middlewares: adminOnly,
		},
		authz.PermSystemBackupsManage,
		h.RunSystemVolumeBackups,
	)
}
