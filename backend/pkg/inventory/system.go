// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original inventory read projection.

package inventory

import (
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/volume"
)

const SystemLabel = "io.docklet.system"

// Agent-only resources are hidden; resources shared with workloads stay visible.
func agentOnly(containers []container.Summary, uses func(container.Summary) bool) bool {
	agent := false
	for _, c := range containers {
		if !uses(c) {
			continue
		}
		if c.Labels[SystemLabel] != "agent" {
			return false
		}
		agent = true
	}
	return agent
}

func Images(items []image.Summary, containers []container.Summary) []image.Summary {
	out := make([]image.Summary, 0, len(items))
	for _, item := range items {
		if item.Labels[SystemLabel] == "agent" || agentOnly(containers, func(c container.Summary) bool { return c.ImageID == item.ID }) {
			continue
		}
		out = append(out, item)
	}
	return out
}
func Volumes(items []volume.Volume, containers []container.Summary) []volume.Volume {
	out := make([]volume.Volume, 0, len(items))
	for _, item := range items {
		if item.Labels[SystemLabel] == "agent" || agentOnly(containers, func(c container.Summary) bool {
			for _, m := range c.Mounts {
				if m.Type == "volume" && m.Name == item.Name {
					return true
				}
			}
			return false
		}) {
			continue
		}
		out = append(out, item)
	}
	return out
}
func Networks(items []network.Summary, containers []container.Summary) []network.Summary {
	out := make([]network.Summary, 0, len(items))
	for _, item := range items {
		builtIn := item.Name == "bridge" || item.Name == "host" || item.Name == "none"
		if item.Labels[SystemLabel] == "agent" || !builtIn && agentOnly(containers, func(c container.Summary) bool {
			if c.NetworkSettings == nil {
				return false
			}
			for name, endpoint := range c.NetworkSettings.Networks {
				if name == item.Name || endpoint != nil && endpoint.NetworkID == item.ID {
					return true
				}
			}
			return false
		}) {
			continue
		}
		out = append(out, item)
	}
	return out
}
