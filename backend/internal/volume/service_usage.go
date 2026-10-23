// Source: docklet-next/issues/191
// Revision: 2efbe8bbe02f930dc19929d07d4e64f6226701e3
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original helper-aware volume consumer projection for issue #191.

package volume

import (
	"context"
	"slices"

	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
)

// Helpers are workspace infrastructure, never user-visible consumers. Keep real
// containers in every state, counting each container once per mounted volume.
func volumeConsumersInternal(containers []container.Summary) map[string][]string {
	consumers := make(map[string][]string)
	for _, c := range containers {
		if isVolumeHelperContainerInternal(c) {
			continue
		}
		for _, m := range c.Mounts {
			if m.Type == mount.TypeVolume && m.Name != "" && !slices.Contains(consumers[m.Name], c.ID) {
				consumers[m.Name] = append(consumers[m.Name], c.ID)
			}
		}
	}
	return consumers
}

// Mutation re-checks and detail reads bypass inventory coalescing and include
// stopped containers, which still prevent non-forced Docker volume removal.
func listVolumeConsumersInternal(ctx context.Context, cli *client.Client) (map[string][]string, error) {
	containers, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	return volumeConsumersInternal(containers.Items), nil
}

func volumeSummaryWithConsumersInternal(v volume.Volume, consumers []string) volumetypes.Volume {
	if v.UsageData != nil {
		usage := *v.UsageData
		usage.RefCount = int64(len(consumers))
		v.UsageData = &usage
	}
	summary := volumetypes.NewSummary(v)
	summary.Containers = append(summary.Containers, consumers...)
	summary.InUse = len(consumers) > 0
	return summary
}
