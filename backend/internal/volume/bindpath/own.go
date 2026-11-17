// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6af01ffd644ceb458b56c80d51c78e3f58847754
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original Docklet install-directory and state-mount protection for rule R2.

package bindpath

import (
	"path"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

const workingDirLabel = "com.docker.compose.project.working_dir"

// stateDestinations are the fixed container paths of Docklet's own data, hub backups and socket.
var stateDestinations = []string{"/app/data", "/var/lib/docklet", "/var/run/docker.sock"}

// Protects reports whether a Docklet container c guards source. Its install directory (Compose
// working directory) and its state mounts are protected with everything below them. Any other
// bind mount, such as the projects directory, protects only its root and ancestors, so stack
// data below it stays usable.
func Protects(c container.Summary, source string) bool {
	source = path.Clean(source)
	if dir := strings.TrimSpace(c.Labels[workingDirLabel]); dir != "" && Overlap(dir, source) {
		return true
	}
	for _, m := range c.Mounts {
		if m.Type != mount.TypeBind || !Overlap(m.Source, source) {
			continue
		}
		root := path.Clean(m.Source)
		if source == root || source == "/" || strings.HasPrefix(root, source+"/") {
			return true
		}
		for _, state := range stateDestinations {
			if Overlap(path.Clean(m.Destination), state) {
				return true
			}
		}
	}
	return false
}
