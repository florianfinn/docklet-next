// Source: getarcaneapp/arcane/backend/internal/volume/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; split inventory reads, hide dedicated agent volumes and fail on missing consumer reads.; M12 target hiding, destination admission and transactional restore-history rename.

package volume

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/pkg/inventory"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumes"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
	"github.com/samber/mo"
	"go.getarcane.app/docker"
	"go.getarcane.app/kit/pkg"
	"gorm.io/gorm"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

func enrichVolumesWithUsageDataInternal(dockerVolumes, usageVolumes []volume.Volume) []volume.Volume {
	usageByName := make(map[string]*volume.UsageData, len(usageVolumes))
	for _, uv := range usageVolumes {
		if uv.Name == "" || uv.UsageData == nil {
			continue
		}
		// Keep first-seen value to preserve previous nested-loop behavior.
		if _, exists := usageByName[uv.Name]; !exists {
			usageByName[uv.Name] = uv.UsageData
		}
	}

	result := make([]volume.Volume, 0, len(dockerVolumes))
	for _, v := range dockerVolumes {
		if usageData, exists := usageByName[v.Name]; exists {
			v.UsageData = usageData
		}

		result = append(result, v)
	}
	return result
}

func (s *VolumeService) buildVolumeContainerMapInternal(ctx context.Context) (map[string][]string, []container.Summary, error) {
	containers, err := s.dockerService.ListContainers(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list containers: %w", err)
	}

	return volumeConsumersInternal(containers), containers, nil
}

func (s *VolumeService) buildVolumePaginationConfigInternal() pagination.Config[volumetypes.Volume] {
	return pagination.Config[volumetypes.Volume]{
		SearchAccessors: []pagination.SearchAccessor[volumetypes.Volume]{
			func(v volumetypes.Volume) (string, error) { return v.Name, nil },
			func(v volumetypes.Volume) (string, error) { return v.Driver, nil },
			func(v volumetypes.Volume) (string, error) { return v.Mountpoint, nil },
			func(v volumetypes.Volume) (string, error) { return v.Scope, nil },
		},
		SortBindings:    s.buildVolumeSortBindingsInternal(),
		FilterAccessors: buildVolumeFilterAccessorsInternal(),
	}
}

func (s *VolumeService) buildVolumeSortBindingsInternal() []pagination.SortBinding[volumetypes.Volume] {
	createdSortFn := s.compareVolumeCreatedInternal

	return []pagination.SortBinding[volumetypes.Volume]{
		{
			Key: "name",
			Fn:  func(a, b volumetypes.Volume) int { return strings.Compare(a.Name, b.Name) },
		},
		{
			Key: "driver",
			Fn:  func(a, b volumetypes.Volume) int { return strings.Compare(a.Driver, b.Driver) },
		},
		{
			Key: "mountpoint",
			Fn:  func(a, b volumetypes.Volume) int { return strings.Compare(a.Mountpoint, b.Mountpoint) },
		},
		{
			Key: "scope",
			Fn:  func(a, b volumetypes.Volume) int { return strings.Compare(a.Scope, b.Scope) },
		},
		{
			Key: "created",
			Fn:  createdSortFn,
		},
		{
			Key: "createdAt",
			Fn:  createdSortFn,
		},
		{
			Key: "inUse",
			Fn: func(a, b volumetypes.Volume) int {
				if a.InUse == b.InUse {
					return 0
				}
				return kit.Ternary(a.InUse, -1, 1)
			},
		},
		{
			Key: "size",
			Fn:  compareVolumeSizesInternal,
		},
	}
}

func compareVolumeSizesInternal(a, b volumetypes.Volume) int {
	aSize := a.Size
	bSize := b.Size

	if aSize == 0 && a.UsageData != nil {
		aSize = a.UsageData.Size
	}
	if bSize == 0 && b.UsageData != nil {
		bSize = b.UsageData.Size
	}

	if aSize == bSize {
		return strings.Compare(a.Name, b.Name)
	}
	return kit.Ternary(aSize < bSize, -1, 1)
}

func (s *VolumeService) compareVolumeCreatedInternal(a, b volumetypes.Volume) int {
	aTime, aOk := parseVolumeCreatedAtInternal(a.CreatedAt).Get()
	bTime, bOk := parseVolumeCreatedAtInternal(b.CreatedAt).Get()
	if aOk && bOk {
		if aTime.Before(bTime) {
			return -1
		}
		return kit.Ternary(aTime.After(bTime), 1, 0)
	}
	return strings.Compare(a.CreatedAt, b.CreatedAt)
}

func parseVolumeCreatedAtInternal(createdAt string) mo.Option[time.Time] {
	if createdAt == "" {
		return mo.None[time.Time]()
	}
	if parsed, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
		return mo.Some(parsed)
	}
	if parsed, err := time.Parse(time.RFC3339, createdAt); err == nil {
		return mo.Some(parsed)
	}
	return mo.None[time.Time]()
}

func buildVolumeFilterAccessorsInternal() []pagination.FilterAccessor[volumetypes.Volume] {
	return []pagination.FilterAccessor[volumetypes.Volume]{
		{
			Key: "inUse",
			Fn: func(v volumetypes.Volume, filterValue string) bool {
				return kit.Ternary(filterValue == "true", v.InUse, filterValue != "false" || !v.InUse)
			},
		},
	}
}

func calculateVolumeUsageCountsInternal(items []volumetypes.Volume) volumetypes.UsageCounts {
	counts := volumetypes.UsageCounts{
		Total: len(items),
	}
	for _, v := range items {
		if v.InUse {
			counts.Inuse++
		} else {
			counts.Unused++
		}
	}
	return counts
}

// CountUsageFromSnapshot mirrors the inventory usage semantics: internal
// volumes are hidden and workspace helpers never count as consumers.
func (s *VolumeService) CountUsageFromSnapshot(dockerVolumes []volume.Volume, containers []container.Summary) volumetypes.UsageCounts {
	consumers := volumeConsumersInternal(containers)

	counts := volumetypes.UsageCounts{}
	for _, v := range inventory.Volumes(dockerVolumes, containers) {
		if s.isInternalVolumeInternal(volumetypes.NewSummary(v)) {
			continue
		}
		counts.Total++
		if len(consumers[v.Name]) > 0 {
			counts.Inuse++
		} else {
			counts.Unused++
		}
	}
	return counts
}

func (s *VolumeService) isInternalVolumeInternal(v volumetypes.Volume) bool {
	if v.Labels[inventory.SystemLabel] == "agent" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(v.Name), strings.TrimSpace(s.backupVolumeName)) {
		return true
	}

	internal, _ := kit.ParseBool(v.Labels[libarcane.InternalResourceLabel])
	return internal
}

var generatedAnonymousVolumeNameInternal = regexp.MustCompile(`^[a-f0-9]{64}$`)

func isAnonymousVolumeInternal(v volume.Volume) bool {
	if _, ok := v.Labels["com.docker.volume.anonymous"]; ok {
		return true
	}
	return generatedAnonymousVolumeNameInternal.MatchString(v.Name)
}

func mountedVolumeNamesInternal(mounts []container.MountPoint) map[string]struct{} {
	names := make(map[string]struct{})
	for _, item := range mounts {
		if item.Type != mount.TypeVolume {
			continue
		}
		name := cmp.Or(strings.TrimSpace(item.Name), strings.TrimSpace(item.Source))
		if name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

func (s *VolumeService) ListVolumesPaginated(ctx context.Context, params pagination.QueryParams, includeInternal bool) ([]volumetypes.Volume, pagination.Response, volumetypes.UsageCounts, error) {
	startedAt := time.Now()
	slog.DebugContext(
		ctx,
		"volume service: list volumes paginated",
		"search",
		params.Search,
		"sort",
		params.Sort,
		"order",
		params.Order,
		"start",
		params.Start,
		"limit",
		params.Limit,
		"includeInternal",
		includeInternal,
	)
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, pagination.Response{}, volumetypes.UsageCounts{}, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	// Run volume list and container list in parallel for better performance
	type volumeListResult struct {
		volumes []volume.Volume
		err     error
	}
	type containerMapResult struct {
		containerMap map[string][]string
		containers   []container.Summary
		err          error
	}

	volChan := make(chan volumeListResult, 1)
	containerChan := make(chan containerMapResult, 1)

	localSettings := s.settingsService.GetSettingsConfig()
	apiCtx, cancel := context.WithTimeout(ctx, timeouts.GetDuration(localSettings.DockerAPITimeout.AsInt(), timeouts.DefaultDockerAPI))
	defer cancel()

	go func(ctx context.Context) {
		volListBody, volumeListErr := dockerClient.VolumeList(ctx, client.VolumeListOptions{})
		volChan <- volumeListResult{volumes: volListBody.Items, err: volumeListErr}
	}(apiCtx)

	go func(ctx context.Context) {
		containerMap, containers, buildVolumeContainerMapErr := s.buildVolumeContainerMapInternal(ctx)
		containerChan <- containerMapResult{containerMap: containerMap, containers: containers, err: buildVolumeContainerMapErr}
	}(apiCtx)

	// Wait for both results
	volResult := <-volChan
	if volResult.err != nil {
		return nil, pagination.Response{}, volumetypes.UsageCounts{}, fmt.Errorf("failed to list Docker volumes: %w", volResult.err)
	}

	containerResult := <-containerChan
	volumeContainerMap := containerResult.containerMap
	if containerResult.err != nil {
		return nil, pagination.Response{}, volumetypes.UsageCounts{}, containerResult.err
	}

	effectiveParams := params
	usageCacheSnapshot := "not_requested"

	// Size sorting consumes the current cache snapshot and refreshes it in the
	// background so this list request never waits for Docker's DiskUsage call.
	var usageVolumes []volume.Volume
	if params.Sort == "size" {
		if uv, found := docker.GetVolumeUsageDataStaleWhileRevalidate(apiCtx, dockerClient).Get(); found && (len(uv) > 0 || len(volResult.volumes) == 0) {
			usageVolumes = uv
			usageCacheSnapshot = "available"
		} else {
			usageCacheSnapshot = "missing"
			effectiveParams.Sort = "name"
			effectiveParams.Order = pagination.SortAsc
		}
	}

	enrichedVolumes := enrichVolumesWithUsageDataInternal(inventory.Volumes(volResult.volumes, containerResult.containers), usageVolumes)

	items := make([]volumetypes.Volume, 0, len(enrichedVolumes))
	for _, v := range enrichedVolumes {
		volDto := volumeSummaryWithConsumersInternal(v, volumeContainerMap[v.Name])
		if isBackupTargetVolume(v.Labels) || !includeInternal && s.isInternalVolumeInternal(volDto) {
			continue
		}
		items = append(items, volDto)
	}

	paginationConfig := s.buildVolumePaginationConfigInternal()
	result := paginationConfig.SearchOrderAndPaginate(items, effectiveParams)
	counts := calculateVolumeUsageCountsInternal(items)
	paginationResp := pagination.BuildResponse(result.TotalCount, result.TotalAvailable, effectiveParams)
	slog.DebugContext(
		ctx, "volume service: listed volumes",
		"dockerHost", dockerClient.DaemonHost(),
		"requestedSort", params.Sort,
		"requestedOrder", params.Order,
		"effectiveSort", effectiveParams.Sort,
		"effectiveOrder", effectiveParams.Order,
		"usageCacheSnapshot", usageCacheSnapshot,
		"dockerVolumes", len(volResult.volumes),
		"usageVolumes", len(usageVolumes),
		"includedVolumes", len(items),
		"matchedVolumes", result.TotalCount,
		"returnedVolumes", len(result.Items),
		"containerVolumeCount", len(volumeContainerMap),
		"filterCount", len(params.Filters),
		"currentPage", paginationResp.CurrentPage,
		"totalPages", paginationResp.TotalPages,
		"duration", time.Since(startedAt),
	)

	return result.Items, paginationResp, counts, nil
}

type volumeWorkspaceBusyError struct {
	name string
}

func (e *volumeWorkspaceBusyError) Error() string {
	return "volume workspace is busy; close active operations and retry rename: " + e.name
}

// RenameVolume copies an unused volume to a new name and removes the source.
func (s *VolumeService) RenameVolume(ctx context.Context, oldName, newName string, user usertypes.Actor) (*volumetypes.Volume, error) {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" || oldName == newName {
		return nil, common.ErrVolumeRenameInvalid
	}
	if strings.EqualFold(oldName, s.backupVolumeName) || strings.EqualFold(newName, s.backupVolumeName) {
		return nil, common.ErrVolumeRenameProtected
	}

	unlock, acquired := s.workspaceLocks.TryLock(oldName)
	if !acquired {
		return nil, &volumeWorkspaceBusyError{name: oldName}
	}
	defer unlock()
	unlockTarget, targetAcquired := s.workspaceLocks.TryLock(newName)
	if !targetAcquired {
		return nil, &volumeWorkspaceBusyError{name: newName}
	}
	defer unlockTarget()

	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		s.logVolumeRenameErrorInternal(ctx, oldName, newName, user, "connect", err)
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	source, err := dockerClient.VolumeInspect(ctx, oldName, client.VolumeInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			err = common.Classify(common.ErrNotFound, err)
		}
		s.logVolumeRenameErrorInternal(ctx, oldName, newName, user, "inspect", err)
		return nil, fmt.Errorf("inspect source volume: %w", err)
	}
	if isBackupTargetVolume(source.Volume.Labels) {
		return nil, hiddenTargetVolumeError()
	}
	if s.isInternalVolumeInternal(volumetypes.NewSummary(source.Volume)) {
		return nil, common.ErrVolumeRenameProtected
	}

	// Stop any read-only browse helper first; a helper mounting the volume would
	// otherwise fail the detached-source check with "in use".
	if stopErr := s.StopHelper(ctx, oldName); stopErr != nil {
		return nil, fmt.Errorf("stop volume workspace helper before rename: %w", stopErr)
	}

	migration, err := volumes.PlanRename(ctx, dockerClient, oldName, newName, s.toolsImageInternal())
	if err != nil {
		s.logVolumeRenameErrorInternal(ctx, oldName, newName, user, "plan", err)
		return nil, err
	}
	if applyErr := migration.Apply(ctx); applyErr != nil {
		if errdefs.IsInvalidArgument(applyErr) {
			applyErr = common.Classify(common.ErrBadRequest, applyErr)
		}
		s.logVolumeRenameErrorInternal(ctx, oldName, newName, user, "apply", applyErr)
		return nil, applyErr
	}

	if renameVolumeMetadataErr := s.renameVolumeMetadataInternal(ctx, oldName, newName); renameVolumeMetadataErr != nil {
		rollbackErr := migration.Rollback(ctx)
		combinedErr := errors.Join(fmt.Errorf("rename volume metadata: %w", renameVolumeMetadataErr), rollbackErr)
		s.logVolumeRenameErrorInternal(ctx, oldName, newName, user, "metadata", combinedErr)
		return nil, combinedErr
	}

	if committer, ok := migration.(volumetypes.Committer); ok {
		if commitErr := committer.Commit(ctx); commitErr != nil {
			if _, localOk := errors.AsType[*volumetypes.SourceCleanupError](commitErr); localOk {
				// The copy and metadata are committed; only removing the source
				// failed, so the rename itself succeeded.
				slog.WarnContext(ctx, "volume renamed but source volume could not be removed", "oldVolume", oldName, "newVolume", newName, "error", commitErr.Error())
			} else {
				metadataErr := s.renameVolumeMetadataInternal(ctx, newName, oldName)
				rollbackErr := migration.Rollback(ctx)
				combinedErr := errors.Join(commitErr, metadataErr, rollbackErr)
				s.logVolumeRenameErrorInternal(ctx, oldName, newName, user, "commit", combinedErr)
				return nil, combinedErr
			}
		}
	}

	s.removeHelperEntry(oldName)
	docker.InvalidateVolumeUsageCache(dockerClient)

	renamed, err := s.GetVolumeByName(ctx, newName)
	if err != nil {
		s.logVolumeRenameErrorInternal(ctx, oldName, newName, user, "inspect-renamed", err)
		return nil, fmt.Errorf("inspect renamed volume: %w", err)
	}

	metadata := database.JSON{"action": "rename", "oldName": oldName, "newName": newName}
	if logErr := s.eventService.LogVolumeEvent(ctx, event.EventTypeVolumeRename, newName, newName, user.ID, user.Username, "0", metadata); logErr != nil {
		slog.WarnContext(ctx, "could not log volume rename action", "oldVolume", oldName, "newVolume", newName, "error", logErr)
	}

	return renamed, nil
}

// Backup jobs are keyed by policy ID and re-read the policy row on every run,
// so renaming volume_name requires no job rescheduling.
func (s *VolumeService) renameVolumeMetadataInternal(ctx context.Context, oldName, newName string) error {
	if s.db == nil {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&VolumeBackup{}).Where("volume_name = ?", oldName).Update("volume_name", newName).Error; err != nil {
			return fmt.Errorf("rename volume backup history: %w", err)
		}
		if err := tx.Table("volume_restore_runs").Where("volume_name = ?", oldName).Update("volume_name", newName).Error; err != nil {
			return fmt.Errorf("rename volume restore history: %w", err)
		}
		if err := tx.Table("volume_restore_runs").Where("new_volume_name = ?", oldName).Update("new_volume_name", newName).Error; err != nil {
			return fmt.Errorf("rename restore destination history: %w", err)
		}
		if err := s.backup.RenamePolicies(tx, oldName, newName); err != nil {
			return fmt.Errorf("rename volume backup policies: %w", err)
		}
		return nil
	})
}

func (s *VolumeService) logVolumeRenameErrorInternal(ctx context.Context, oldName, newName string, user usertypes.Actor, step string, err error) {
	s.eventService.LogErrorEvent(ctx, event.EventTypeVolumeError, "volume", oldName, oldName, user.ID, user.Username, "0", err, database.JSON{
		"action":  "rename",
		"step":    step,
		"oldName": oldName,
		"newName": newName,
	})
}
