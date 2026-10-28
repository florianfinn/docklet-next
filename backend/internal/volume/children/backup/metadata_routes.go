// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original host-scoped overview, read-only engine metadata and asynchronous repository check routes.

package backup

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/volume"
)

type HostBackupInput struct {
	EnvironmentID string `path:"id"`
}

func registerMetadataRoutes(api huma.API, h *Handler) {
	operation := func(id, method, path string, status int) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: "/environments/{id}/volumes/backups/" + path, DefaultStatus: status, Tags: []string{"Volume Backup"}, Security: handlerutil.DefaultOperationSecurity()}
	}
	middleware.RegisterWithPermission(api, operation("get-volume-backup-overview", http.MethodGet, "overview", 200), authz.PermVolumesList, h.Overview)
	middleware.RegisterWithPermission(api, operation("get-volume-backup-repository", http.MethodGet, "repository", 200), authz.PermVolumesRead, h.Repository)
	middleware.RegisterWithPermission(api, operation("check-volume-backup-repository", http.MethodPost, "repository/check", 202), authz.PermVolumesBackup, h.CheckRepository)
}
func hostBackupOut[T any](data T) *handlerutil.Out[T] {
	return &handlerutil.Out[T]{Body: base.ApiResponse[T]{Success: true, Data: data}}
}
func (h *Handler) Overview(ctx context.Context, in *HostBackupInput) (*handlerutil.Out[volume.BackupOverview], error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if in.EnvironmentID != "0" {
		response, err := handlerutil.RemoteJSONProxy(h.environmentService.ProxyJSONRequest).JSON[base.ApiResponse[volume.BackupOverview]](ctx, in.EnvironmentID, http.MethodGet, "/api/environments/0/volumes/backups/overview", nil)
		if err != nil {
			return nil, err
		}
		return &handlerutil.Out[volume.BackupOverview]{Body: *response}, nil
	}
	result, err := h.service.Overview(ctx)
	if err != nil {
		return nil, neutralBackupError(err, "volume.backup.engine-unavailable")
	}
	return hostBackupOut(result), nil
}
func (h *Handler) Repository(ctx context.Context, in *HostBackupInput) (*handlerutil.Out[volume.BackupRepository], error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if in.EnvironmentID != "0" {
		response, err := handlerutil.RemoteJSONProxy(h.environmentService.ProxyJSONRequest).JSON[base.ApiResponse[volume.BackupRepository]](ctx, in.EnvironmentID, http.MethodGet, "/api/environments/0/volumes/backups/repository", nil)
		if err != nil {
			return nil, err
		}
		return &handlerutil.Out[volume.BackupRepository]{Body: *response}, nil
	}
	result := volume.BackupRepository{Mode: "volume", Name: h.service.deps.BackupVolumeName, EngineImage: h.service.deps.Engine.EngineImage(ctx)}
	cli, err := h.service.deps.Docker.GetClient(ctx)
	if err != nil {
		return nil, dto.NewProblem(503, "volume.backup.engine-unavailable", err)
	}
	// Metadata inspection mirrors the configured /backups mount without ensuring
	// a volume, pulling an image, mounting storage or starting any helper.
	storage := h.service.resolveBackupStorageMountInternal(ctx, cli, "/repository", true)
	if storage.mode == backupStorageModeArcaneMount {
		result.Mode = "mount"
		result.Name = storage.mount.Source
	}
	var setting struct{ Value string }
	if err = h.service.deps.DB.WithContext(ctx).Table("settings").Where("key = ?", "volumeBackupRepositoryCheck").Scan(&setting).Error; err != nil {
		return nil, err
	}
	if setting.Value != "" {
		if err = json.Unmarshal([]byte(setting.Value), &result.LastCheck); err != nil {
			return nil, dto.NewProblem(500, "volume.backup.engine-output-invalid", err)
		}
	}
	var active struct{ ID string }
	if err = h.service.deps.DB.WithContext(ctx).Table("activities").Where("resource_type = ? AND resource_id = ? AND status IN ?", "backup_target", "local", []string{"pending", "running"}).Order("created_at DESC").Limit(1).Scan(&active).Error; err != nil {
		return nil, err
	}
	if active.ID != "" {
		result.Running = &volume.BackupRunning{Kind: "check", ActivityID: active.ID}
	}
	return hostBackupOut(result), nil
}
func (h *Handler) CheckRepository(ctx context.Context, in *HostBackupInput) (*handlerutil.Out[dto.TargetTestOperation], error) {
	if in.EnvironmentID != "0" {
		response, err := handlerutil.RemoteJSONProxy(h.environmentService.ProxyJSONRequest).JSON[base.ApiResponse[dto.TargetTestOperation]](ctx, in.EnvironmentID, http.MethodPost, "/api/environments/0/volumes/backups/repository/check", nil)
		if err != nil {
			return nil, err
		}
		return &handlerutil.Out[dto.TargetTestOperation]{Body: *response}, nil
	}
	result, err := h.service.startRepositoryCheck(utils.ActivityRuntimeContext(ctx, h.appCtx), "local")
	if err != nil {
		return nil, neutralBackupError(err, "volume.backup.repository-check-failed")
	}
	return hostBackupOut(result), nil
}
