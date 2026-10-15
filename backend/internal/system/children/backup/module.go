// Source: getarcaneapp/arcane/backend/internal/system/children/backup/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Replaced Rustic recovery routes with PostgreSQL archive routes.

package backup

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/system/children/backup/children/volumes"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

func RegisterRoutes(api huma.API, h *Handler) {
	adminOnly := middleware.RequireGlobalAdmin(api)
	op := func(id, method, path, summary string) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: path, Summary: summary,
			Tags: []string{"Hub Backups"}, Middlewares: adminOnly}
	}
	middleware.RegisterWithPermission(api, op("list-system-backups", http.MethodGet, "/backups", "List Hub database archives"), authz.PermSystemBackupsRead, h.List)
	middleware.RegisterWithPermission(api, op("list-backup-history", http.MethodGet, "/backups/history", "List backup history"), authz.PermSystemBackupsRead, h.ListHistory)
	middleware.RegisterWithPermission(api, op("get-system-backup-policies", http.MethodGet, "/backups/policies", "Get Hub database backup policies"), authz.PermSystemBackupsRead, h.GetPolicies)
	middleware.RegisterWithPermission(api, op("update-system-backup-policies", http.MethodPut, "/backups/policies", "Update Hub database backup policies"), authz.PermSystemBackupsManage, h.UpdatePolicies)
	middleware.RegisterWithPermission(api, op("get-system-backup-storage", http.MethodGet, "/backups/storage", "Get local archive storage usage"), authz.PermSystemBackupsRead, h.Storage)
	middleware.RegisterWithPermission(api, op("get-system-backup-maintenance", http.MethodGet, "/backups/maintenance", "Get Hub restore maintenance status"), authz.PermSystemBackupsRead, h.Maintenance)
	create := op("create-system-backup", http.MethodPost, "/backups", "Create a Hub database archive")
	create.DefaultStatus = http.StatusAccepted
	middleware.RegisterWithPermission(api, create, authz.PermSystemBackupsManage, h.Create)
	upload := op("import-system-backup-upload", http.MethodPost, "/backups/upload", "Upload and import a Hub database archive")
	upload.DefaultStatus = http.StatusAccepted
	upload.Description = "Stream one PostgreSQL custom-format archive as application/octet-stream, up to 2 GiB."
	upload.MaxBodyBytes = backuptypes.MaxSystemArchiveBytes + 1
	upload.Errors = []int{http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType}
	maxArchiveBytes := int(backuptypes.MaxSystemArchiveBytes)
	// RawBody would buffer the entire archive before the handler can stream it.
	upload.RequestBody = &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{
		"application/octet-stream": {Schema: &huma.Schema{
			Type: "string", Format: "binary", MaxLength: &maxArchiveBytes,
		}},
	}}
	upload.Metadata = map[string]any{authz.MetaRequiredPermission: authz.PermSystemBackupsRestore}
	upload.Middlewares = append(upload.Middlewares, middleware.RequirePermission(api, authz.PermSystemBackupsRestore)...)
	upload.Middlewares = append(upload.Middlewares, h.streamUploadMiddleware(api)...)
	huma.Register(api, upload, h.Upload)
	download := op("download-system-backup", http.MethodGet, "/backups/{id}/download", "Download a Hub database archive")
	download.Responses = map[string]*huma.Response{"200": {
		Description: "PostgreSQL custom-format archive",
		Headers: map[string]*huma.Param{"Content-Disposition": {
			Description: "attachment with a server-selected archive filename",
			Required:    true,
			Schema:      &huma.Schema{Type: "string", Pattern: "^attachment(?:;|$)"},
		}},
		Content: map[string]*huma.MediaType{"application/octet-stream": {Schema: &huma.Schema{Type: "string", Format: "binary"}}},
	}}
	middleware.RegisterWithPermission(api, download, authz.PermSystemBackupsRead, h.Download)
	restore := op("restore-system-backup", http.MethodPost, "/backups/{id}/restore", "Restore a Hub database archive")
	restore.DefaultStatus = http.StatusAccepted
	middleware.RegisterWithPermission(api, restore, authz.PermSystemBackupsRestore, h.Restore)
	deleteBackup := op("delete-system-backup", http.MethodDelete, "/backups/{id}", "Delete a Hub database archive")
	deleteBackup.DefaultStatus = http.StatusAccepted
	middleware.RegisterWithPermission(api, deleteBackup, authz.PermSystemBackupsManage, h.Delete)
	var volumeService *volumes.Service
	if h.service != nil {
		volumeService = h.service.volumes
	}
	volumes.RegisterRoutes(api, volumes.NewHandler(volumeService, h.appCtx))
}
