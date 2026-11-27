// Source: docklet-next/issues/678
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original bounded prune inventory helper.

package prune

import (
	"context"
	"github.com/moby/moby/client"
	"time"
)

// pruneDiskUsage retains all references for guards, computing writable sizes
// only for stopped containers. One bounded disk snapshot serves the whole plan.
func pruneDiskUsage(ctx context.Context, cli *client.Client, buildCache bool) (client.DiskUsageResult, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	usage, err := cli.DiskUsage(ctx, client.DiskUsageOptions{Images: true, Volumes: true, BuildCache: buildCache, Verbose: true})
	if err != nil {
		return usage, err
	}
	all, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return usage, err
	}
	stopped, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Size: true, Filters: make(client.Filters).Add("status", "created", "removing", "exited", "dead")})
	if err != nil {
		return usage, err
	}
	sizes := map[string]int64{}
	for _, c := range stopped.Items {
		sizes[c.ID] = c.SizeRw
	}
	for i := range all.Items {
		all.Items[i].SizeRw = sizes[all.Items[i].ID]
	}
	usage.Containers.Items = all.Items
	return usage, nil
}
