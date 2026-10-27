// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Head-only scheduled image prune planning for issue #229.

package prune

import (
	"context"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"slices"
)

// Disk usage includes intermediate parents on classic graphdriver hosts; use
// the same head-image producer as manual prune before freezing scheduled IDs.
func restrictPruneHeadImages(ctx context.Context, docker *client.Client, usage *client.DiskUsageResult) error {
	heads, err := docker.ImageList(ctx, client.ImageListOptions{All: false})
	if err != nil {
		return imageDTO.Failure("image.prune-failed", 502)
	}
	allowed := map[string]bool{}
	for _, head := range heads.Items {
		allowed[head.ID] = true
	}
	usage.Images.Items = slices.DeleteFunc(usage.Images.Items, func(item image.Summary) bool { return !allowed[item.ID] })
	return nil
}
