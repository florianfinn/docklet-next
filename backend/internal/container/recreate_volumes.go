// Source: getarcaneapp/arcane/backend/internal/container/service.go (retainedVolumeMountPoints)
// Revision: a7812907535f79639abad968763508df60ee157b
// Retrieved: 2026-11-27
// License: BSD-3-Clause
// Deviations: Docklet #667: one helper for every standalone recreate path; works on copies of the inspected and requested host configs.

package container

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"go.getarcane.app/docker"
)

// recreateVolumeHostConfig returns a copy of requested that names every volume
// the old container mounted at a destination the request leaves to Docker, so
// the replacement reuses anonymous and image-declared volumes instead of
// receiving fresh empty ones. A destination the request retargets (named
// volume, bind, tmpfs) or removes keeps the requested behaviour.
func recreateVolumeHostConfig(ctx context.Context, old container.InspectResponse, requested *container.HostConfig) (*container.HostConfig, error) {
	if requested == nil {
		return nil, nil
	}
	host := *requested
	var original container.HostConfig
	if old.HostConfig != nil {
		original = *old.HostConfig
	}
	binds, mounts, err := docker.PreserveVolumeMounts(ctx, host.Binds, host.Mounts, retainedVolumeMountPoints(original, host, old.Mounts))
	if err != nil {
		return nil, fmt.Errorf("preserve volumes of container %s: %w", strings.TrimPrefix(old.Name, "/"), err)
	}
	host.Binds, host.Mounts = binds, mounts
	return &host, nil
}

// retainedVolumeMountPoints drops mount points whose destination the updated
// config targets with a non-anonymous spec, or that the original config
// targeted and the updated one no longer does.
func retainedVolumeMountPoints(original, updated container.HostConfig, mountPoints []container.MountPoint) []container.MountPoint {
	specAt := func(hc container.HostConfig, destination string) (found, anonymous bool) {
		if i := slices.IndexFunc(hc.Mounts, func(m mount.Mount) bool { return path.Clean(m.Target) == destination }); i >= 0 {
			return true, hc.Mounts[i].Type == mount.TypeVolume && hc.Mounts[i].Source == ""
		}
		for _, bind := range hc.Binds {
			parts := strings.SplitN(bind, ":", 3)
			if path.Clean(parts[min(1, len(parts)-1)]) == destination {
				return true, len(parts) == 1
			}
		}
		return false, false
	}
	return slices.DeleteFunc(slices.Clone(mountPoints), func(mp container.MountPoint) bool {
		destination := path.Clean(mp.Destination)
		inOriginal, _ := specAt(original, destination)
		inUpdated, anonymous := specAt(updated, destination)
		if inUpdated {
			return !anonymous
		}
		return inOriginal
	})
}
