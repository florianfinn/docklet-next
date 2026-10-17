// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host inventory read implementation.

package listing

import (
	"github.com/moby/moby/api/types/container"
	"go.getarcane.app/docker"
	"slices"
)

// A labeled agent excludes its Compose project, including file-discovered records.
func SystemProjectNames(containers []container.Summary) []string {
	names := []string{}
	for _, c := range containers {
		if c.Labels["io.docklet.system"] == "agent" {
			if name := docker.ComposeProjectLabel(c.Labels); name != "" && !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	return names
}
