// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6af01ffd644ceb458b56c80d51c78e3f58847754
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original projects-mount exemption and install-directory protection for rule R2.

package bindpath

import (
	"context"
	"path"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"go.getarcane.app/sys/cgroup"
)

const (
	workingDirLabel = "com.docker.compose.project.working_dir"
	dataDirectory   = "/app/data"
)

// Own names the executing Docklet container and its projects directory (container path).
// Its projects mount protects the mount root and its ancestors; descendants are workload data.
type Own struct{ ContainerID, Projects string }

// OwnLookup resolves Own at most once, and only when a Docklet mount overlaps a source.
type OwnLookup func() Own

// Strict never exempts a mount.
func Strict() Own { return Own{} }

// Runtime carries values resolved by their owners at hub or agent startup. Without it, or with
// a relative or empty state path, no mount is exempt.
type Runtime struct {
	Projects func(context.Context) string // effective projects setting, as the project service reads it
	State    []string                     // Docklet's own state: data root, database dir, backups, Docker config, socket
}

var runtime atomic.Pointer[Runtime]

// Configure registers the runtime once startup has resolved its configuration.
func Configure(r Runtime) { runtime.Store(&r) }

// OwnProjects returns a lazy lookup of the executing container.
func OwnProjects(ctx context.Context, cli *client.Client) OwnLookup {
	return sync.OnceValue(func() Own { return ownProjects(ctx, cli, runtime.Load(), CurrentContainerID) })
}

// CurrentContainerID reads this process's container ID; tests replace it.
var CurrentContainerID = cgroup.CurrentContainerID

var hexID = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

// Any doubt yields the zero value, which keeps every Docklet mount fully protected: the identity
// is this process's own container ID (no label or name fallback), no other container shares its
// network namespace, and the projects mount touches neither Docklet's state nor another own mount.
func ownProjects(ctx context.Context, cli *client.Client, rt *Runtime, id func() (string, error)) Own {
	if cli == nil || rt == nil || rt.Projects == nil || id == nil {
		return Own{}
	}
	for _, state := range rt.State {
		if !path.IsAbs(state) {
			return Own{}
		}
	}
	self, err := id()
	if self = strings.TrimSpace(self); err != nil || !hexID.MatchString(self) {
		return Own{}
	}
	inspected, err := cli.ContainerInspect(ctx, self, client.ContainerInspectOptions{})
	c := inspected.Container
	if err != nil || !strings.HasPrefix(c.ID, self) || c.Config == nil {
		return Own{}
	}
	if role := c.Config.Labels["io.docklet.system"]; role != "hub" && role != "agent" {
		return Own{}
	}
	rows, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return Own{}
	}
	for _, row := range rows.Items {
		owner, shared := strings.CutPrefix(row.HostConfig.NetworkMode, "container:")
		if shared && owner != "" && (strings.HasPrefix(c.ID, owner) || "/"+owner == c.Name) {
			return Own{}
		}
	}
	projectsDir := path.Clean(projects.ResolveConfiguredContainerDirectory(rt.Projects(ctx), "/app/data/projects"))
	if !path.IsAbs(projectsDir) {
		return Own{}
	}
	for _, state := range append([]string{dataDirectory}, rt.State...) {
		if Overlap(projectsDir, state) {
			return Own{}
		}
	}
	found := false
	for _, m := range c.Mounts {
		destination := path.Clean(m.Destination)
		if m.Type == mount.TypeBind && destination == projectsDir {
			found = true
		} else if Overlap(destination, projectsDir) {
			return Own{}
		}
	}
	if !found {
		return Own{}
	}
	return Own{ContainerID: c.ID, Projects: projectsDir}
}

// Protects reports whether a Docklet container c guards source: any of its bind mounts or its
// Compose working directory overlaps source, except below the own projects mount.
func Protects(c container.Summary, source string, lookup OwnLookup) bool {
	source = path.Clean(source)
	for _, m := range c.Mounts {
		if m.Type != mount.TypeBind || !Overlap(m.Source, source) {
			continue
		}
		if own := lookup(); own.ContainerID != "" && c.ID == own.ContainerID && path.Clean(m.Destination) == own.Projects {
			root := path.Clean(m.Source)
			if source == root || source == "/" || strings.HasPrefix(root, source+"/") {
				return true
			}
			continue
		}
		return true
	}
	dir := strings.TrimSpace(c.Labels[workingDirLabel])
	return dir != "" && Overlap(dir, source)
}
