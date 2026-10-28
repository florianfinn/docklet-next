// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations.

package backup

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/browser"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterRoutes registers the volume backup endpoints, including policies,
// file browsing, and restores.
func RegisterRoutes(api huma.API, h *Handler) {
	registerMetadataRoutes(api, h)
	var policiesService *policies.Service
	var browserService *browser.Service
	var restoreService *restore.Service
	if h.service != nil {
		policiesService, browserService, restoreService = h.service.policies, h.service.browser, h.service.restore
	}
	policyHandler := policies.NewHandler(policiesService, h.activityService, h.environmentService, h.appCtx)
	if h.service != nil {
		policyHandler.AroundUpdate = h.service.aroundPolicyUpdate(h.environmentService, h.config)
		if h.config != nil && (h.config.AgentMode || h.config.EdgeAgent) {
			policyHandler.BeforeLocalUpdate = h.service.checkPolicySet
		}
	}
	policies.RegisterRoutes(api, policyHandler)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "list-volume-backups",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/volumes/{volumeName}/backups",
		Summary:     "List volume backups",
		Tags:        []string{"Volume Backup"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermVolumesRead, h.ListBackups)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID:   "create-volume-backup",
		DefaultStatus: http.StatusAccepted,
		Method:        http.MethodPost,
		Path:          "/environments/{id}/volumes/{volumeName}/backups",
		Summary:       "Create volume backup",
		Tags:          []string{"Volume Backup"},
		Security:      handlerutil.DefaultOperationSecurity(),
	}, authz.PermVolumesBackup, h.CreateBackup)

	restore.RegisterRoutes(api, restore.NewHandler(restoreService, h.activityService, h.uploadService, h.appCtx))

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "delete-volume-backup",
		Method:      http.MethodDelete,
		Path:        "/environments/{id}/volumes/backups/{backupId}",
		Summary:     "Delete volume backup",
		Tags:        []string{"Volume Backup"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermVolumesBackup, h.DeleteBackup)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "download-volume-backup",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/volumes/backups/{backupId}/download",
		Summary:     "Download volume backup",
		Tags:        []string{"Volume Backup"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermVolumesRead, h.DownloadBackup)

	browser.RegisterRoutes(api, browser.NewHandler(browserService))
}
