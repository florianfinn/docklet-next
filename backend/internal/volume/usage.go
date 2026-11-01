// Source: getarcaneapp/arcane/backend/internal/volume/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; split inventory reads; Explicit candidate prune execution; M12 backup foundation; M12 persisted volume lifecycle and legacy restore cut.; M12 target hiding and non-waiting destination admission; remove retired centralized volume wrappers; M16 bind mounts and backup completion.

package volume

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
	"log/slog"
)

func (s *VolumeService) GetVolumeUsage(ctx context.Context, name string) (bool, []string, error) {
	slog.DebugContext(ctx, "volume service: get volume usage", "volume", name)
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	vol, err := dockerClient.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		return false, nil, fmt.Errorf("volume not found: %w", err)
	}

	if isBackupTargetVolume(vol.Volume.Labels) {
		return false, nil, hiddenTargetVolumeError()
	}
	consumers, err := listVolumeConsumersInternal(ctx, dockerClient)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get containers using volume: %w", err)
	}

	containerIDs := consumers[vol.Volume.Name]
	return len(containerIDs) > 0, containerIDs, nil
}

// VolumeSizeData holds size information for a volume.
type VolumeSizeData struct {
	Size     int64
	RefCount int64
}

// GetVolumeSizes returns disk usage data for all volumes.
// This is a slow operation as it calls Docker's DiskUsage API.
func (s *VolumeService) GetVolumeSizes(ctx context.Context) (map[string]VolumeSizeData, error) {
	slog.DebugContext(ctx, "volume service: get volume sizes")
	localSettings := s.settingsService.GetSettingsConfig()
	apiCtx, cancel := context.WithTimeout(ctx, timeouts.GetDuration(localSettings.DockerAPITimeout.AsInt(), timeouts.DefaultDockerAPI))
	defer cancel()

	dockerClient, err := s.dockerService.GetClient(apiCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	usageVolumes, err := docker.GetVolumeUsageData(apiCtx, dockerClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume usage data: %w", err)
	}

	consumers, err := listVolumeConsumersInternal(apiCtx, dockerClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get containers using volumes: %w", err)
	}
	result := make(map[string]VolumeSizeData, len(usageVolumes))
	for _, v := range usageVolumes {
		if isBackupTargetVolume(v.Labels) {
			continue
		}
		if v.UsageData != nil {
			result[v.Name] = VolumeSizeData{
				Size:     v.UsageData.Size,
				RefCount: int64(len(consumers[v.Name])),
			}
		}
	}

	return result, nil
}
