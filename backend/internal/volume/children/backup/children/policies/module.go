// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/children/policies/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package policies

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterRoutes registers the volume backup policy endpoints.
func RegisterRoutes(api huma.API, h *Handler) {
	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "get-volume-backup-policy",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/volumes/{volumeName}/backup-policy",
		Summary:     "Get volume backup policies",
		Tags:        []string{"Volume Backup"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermVolumesRead, h.GetBackupPolicy)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "update-volume-backup-policy",
		Method:      http.MethodPut,
		Path:        "/environments/{id}/volumes/{volumeName}/backup-policy",
		Summary:     "Update volume backup policies",
		Tags:        []string{"Volume Backup"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermVolumesBackup, h.UpdateBackupPolicy)
}
