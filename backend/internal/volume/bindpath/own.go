// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6af01ffd644ceb458b56c80d51c78e3f58847754
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original projects-mount exemption and install-directory protection for rule R2.

package bindpath

import (
	"context"
	"path"
	"strings"
	"sync"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

const workingDirLabel = "com.docker.compose.project.working_dir"

// Own names the executing Docklet container and its projects directory (container path).
// Its projects mount protects the mount root and its ancestors; descendants are workload data.
type Own struct{ ContainerID, Projects string }

// OwnLookup resolves Own at most once, and only when a Docklet mount overlaps a source.
type OwnLookup func() Own

// Strict never exempts a mount.
func Strict() Own { return Own{} }

// OwnProjects returns a lazy lookup of the executing container.
func OwnProjects(ctx context.Context, cli *client.Client, db *database.DB) OwnLookup {
	return sync.OnceValue(func() Own { return ownProjects(ctx, cli, db) })
}

// Any lookup failure yields the zero value, which keeps every Docklet mount fully protected.
func ownProjects(ctx context.Context, cli *client.Client, db *database.DB) Own {
	if cli == nil {
		return Own{}
	}
	inspect, err := libarcane.InspectCurrentArcaneContainer(ctx, cli)
	if err != nil || inspect == nil || inspect.ID == "" || inspect.Config == nil {
		return Own{}
	}
	if role := inspect.Config.Labels["io.docklet.system"]; role != "hub" && role != "agent" {
		return Own{}
	}
	configured, _, _ := utils.LookupEnvOrFile("PROJECTS_DIRECTORY")
	if strings.TrimSpace(configured) == "" && db != nil {
		var setting struct{ Value string }
		if db.WithContext(ctx).Table("settings").Where("key = ?", "projectsDirectory").Limit(1).Find(&setting).Error != nil {
			return Own{}
		}
		configured = setting.Value
	}
	return Own{ContainerID: inspect.ID, Projects: path.Clean(projects.ResolveConfiguredContainerDirectory(configured, "/app/data/projects"))}
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
