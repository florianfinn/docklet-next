// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6af01ffd644ceb458b56c80d51c78e3f58847754
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original tests for Docklet install-directory and state-mount protection.

package bindpath

import (
	"testing"

	"github.com/florianfinn/docklet-next/types/backup"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

func dockletContainer(id, role, workingDir string, mounts ...container.MountPoint) container.Summary {
	labels := map[string]string{"io.docklet.system": role}
	if workingDir != "" {
		labels[workingDirLabel] = workingDir
	}
	return container.Summary{ID: id, Labels: labels, Mounts: mounts}
}

func bind(source, destination string) container.MountPoint {
	return container.MountPoint{Type: mount.TypeBind, Source: source, Destination: destination}
}

func reason(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	var p *backup.Problem
	require.ErrorAs(t, err, &p)
	return p.Reason
}

func TestDockletMountsProtectStateInstallDirectoryAndMountRoots(t *testing.T) {
	// README and agent installer layout: Docklet lives inside the projects directory it mounts.
	for _, self := range []container.Summary{
		dockletContainer("hub", "hub", "/srv/docker/docklet",
			bind("/srv/docker", "/srv/docker"), bind("/srv/docker/docklet/data", "/app/data"),
			bind("/srv/docker/docklet/backups", "/var/lib/docklet/backups"), bind("/var/run/docker.sock", "/var/run/docker.sock")),
		dockletContainer("agent", "agent", "/srv/docker/docklet-agent",
			bind("/srv/docker", "/srv/docker"), bind("/srv/docker/docklet-agent/data", "/app/data")),
	} {
		install := self.Labels[workingDirLabel]
		for source, want := range map[string]string{
			"/srv/docker/app/data":          "", // stack data below the projects mount
			"/srv/docker/app":               "",
			"/srv/docker":                   "docklet", // the mount root
			"/srv":                          "docklet", // an ancestor
			install:                         "docklet", // install directory with .env
			install + "/compose.yaml":       "docklet",
			install + "/data/docklet.db":    "docklet", // state mount
			"/srv/docker/docklet/backups/x": "docklet",
			"/var/run/docker.sock":          "system", // R1 already refuses the socket
		} {
			if self.ID == "agent" && source == "/srv/docker/docklet/backups/x" {
				want = "" // the hub's backups are not the agent's state
			}
			require.Equal(t, want, reason(t, Rules(source, "", []container.Summary{self}, nil, true)), "%s %s", self.ID, source)
		}
	}
}

func TestDockletStateMountsProtectTheirSubtreeWhereverTheyLive(t *testing.T) {
	// Without a Compose working directory label the state mount alone still protects its subtree.
	hub := dockletContainer("hub", "hub", "", bind("/srv/state", "/app/data"), bind("/srv/archives", "/var/lib/docklet/backups"))
	for _, source := range []string{"/srv/state/docklet.db", "/srv/state/.docker/config.json", "/srv/archives/archive.tar"} {
		require.Equal(t, "docklet", reason(t, Rules(source, "", []container.Summary{hub}, nil, true)), source)
	}
	// A projects mount at a state destination is state too.
	nested := dockletContainer("hub", "hub", "", bind("/srv/docker", "/app/data/projects"))
	require.Equal(t, "docklet", reason(t, Rules("/srv/docker/app/data", "", []container.Summary{nested}, nil, true)))
}

func TestDockletProjectsMountAloneProtectsItsRootAndAncestors(t *testing.T) {
	only := dockletContainer("agent", "agent", "", bind("/srv/docker", "/srv/docker"))
	for source, want := range map[string]string{"/srv": "docklet", "/srv/docker": "docklet", "/srv/docker/app": ""} {
		require.Equal(t, want, reason(t, Rules(source, "", []container.Summary{only}, nil, true)), source)
	}
}
