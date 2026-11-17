// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6af01ffd644ceb458b56c80d51c78e3f58847754
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original projects-mount exemption and install-directory protection for rule R2.

package bindpath

import (
	"cmp"
	"context"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	kit "go.getarcane.app/kit/pkg"
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

// OwnProjects returns a lazy lookup of the executing container.
func OwnProjects(ctx context.Context, cli *client.Client, db *database.DB) OwnLookup {
	return sync.OnceValue(func() Own { return ownProjects(ctx, cli, db, CurrentContainerID) })
}

// CurrentContainerID reads this process's container ID; tests replace it.
var CurrentContainerID = cgroup.CurrentContainerID

var hexID = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

// Any doubt yields the zero value, which keeps every Docklet mount fully protected: the identity
// is this process's own container ID (no label or name fallback), no other container shares its
// network namespace, and the projects mount touches neither Docklet's state nor another own mount.
func ownProjects(ctx context.Context, cli *client.Client, db *database.DB, id func() (string, error)) Own {
	if cli == nil || id == nil {
		return Own{}
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
	configured, ok := projectsSetting(ctx, db)
	if !ok {
		return Own{}
	}
	projectsDir := path.Clean(projects.ResolveConfiguredContainerDirectory(configured, "/app/data/projects"))
	for _, operational := range operationalPaths() {
		if Overlap(projectsDir, operational) {
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

// projectsSetting mirrors the settings service: a non-empty environment override (file
// variants, quotes trimmed) wins over the stored setting.
func projectsSetting(ctx context.Context, db *database.DB) (string, bool) {
	if value := envValue("PROJECTS_DIRECTORY"); value != "" {
		return value, true
	}
	if db == nil {
		return "", true
	}
	var setting struct{ Value string }
	if db.WithContext(ctx).Table("settings").Where("key = ?", "projectsDirectory").Limit(1).Find(&setting).Error != nil {
		return "", false
	}
	return setting.Value, true
}

func envValue(name string) string {
	value, _, _ := utils.LookupEnvOrFile(name)
	return strings.TrimSpace(kit.TrimQuotes(strings.TrimSpace(value)))
}

// operationalPaths are Docklet's own state: data root, database directory, hub backups,
// registry login data and the Docker socket.
func operationalPaths() []string {
	paths := []string{dataDirectory, "/var/run/docker.sock", cmp.Or(envValue("DOCKLET_BACKUP_DIR"), "/var/lib/docklet/backups")}
	if dir := envValue("DOCKER_CONFIG"); dir != "" {
		paths = append(paths, dir)
	}
	database := strings.TrimPrefix(strings.SplitN(cmp.Or(envValue("DATABASE_URL"), "file:data/docklet.db"), "?", 2)[0], "file:")
	if !strings.Contains(database, "://") {
		if abs, err := filepath.Abs(database); err == nil {
			paths = append(paths, path.Dir(abs))
		}
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.HasPrefix(p, "/") {
			out = append(out, path.Clean(p))
		}
	}
	return out
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
