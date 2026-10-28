// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/children/restore/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; M12 persisted volume lifecycle and legacy restore cut.

package restore

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	uploadtypes "github.com/florianfinn/docklet-next/types/upload"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/upload"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterRoutes registers the volume backup restore endpoints.
func RegisterRoutes(api huma.API, h *Handler) {
	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID:   "restore-volume-backup",
		DefaultStatus: http.StatusAccepted,
		Method:        http.MethodPost,
		Path:          "/environments/{id}/volumes/{volumeName}/backups/{backupId}/restore",
		Summary:       "Restore volume backup",
		Tags:          []string{"Volume Backup"},
		Security:      handlerutil.DefaultOperationSecurity(),
	}, authz.PermVolumesBackup, h.RestoreBackup)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID:   "restore-volume-backup-files",
		DefaultStatus: http.StatusAccepted,
		Method:        http.MethodPost,
		Path:          "/environments/{id}/volumes/{volumeName}/backups/{backupId}/restore-files",
		Summary:       "Restore specific files from a volume backup",
		Tags:          []string{"Volume Backup"},
		Security:      handlerutil.DefaultOperationSecurity(),
	}, authz.PermVolumesBackup, h.RestoreBackupFiles)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID:   "upload-volume-backup",
		DefaultStatus: http.StatusAccepted,
		Method:        http.MethodPost,
		Path:          "/environments/{id}/volumes/{volumeName}/backups/upload",
		Summary:       "Upload and restore volume backup",
		Description: "Restore a volume from a complete chunked upload session containing a tar.gz backup archive. " +
			"multipart/form-data bodies are still accepted for backward compatibility; that form is deprecated " +
			"and will be removed in a future release.",
		Tags:        []string{"Volume Backup"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: upload.LegacyMultipartMiddleware(api, h.uploadService, uploadtypes.KindVolumeBackup),
	}, authz.PermVolumesUpload, h.UploadAndRestore)
}
