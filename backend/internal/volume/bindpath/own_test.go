// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6af01ffd644ceb458b56c80d51c78e3f58847754
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original projects-mount exemption and install-directory protection tests.

package bindpath

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/types/backup"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
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

func fixedOwn(own Own) OwnLookup { return func() Own { return own } }

func reason(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	var p *backup.Problem
	require.ErrorAs(t, err, &p)
	return p.Reason
}

func TestOwnProjectsMountProtectsOnlyRootAndAncestors(t *testing.T) {
	agent := dockletContainer("agent-id", "agent", "/srv/docker/docklet-agent",
		bind("/srv/docker", "/srv/docker"), bind("/srv/docker/docklet-agent/data", "/app/data"))
	workload := container.Summary{ID: "app", Mounts: []container.MountPoint{bind("/srv/docker/app/data", "/data")}}
	rows := []container.Summary{agent, workload}
	own := fixedOwn(Own{ContainerID: "agent-id", Projects: "/srv/docker"})
	for source, want := range map[string]string{
		"/srv/docker/app/data":                  "", // B1: stack data below the projects root
		"/srv/docker/app":                       "",
		"/srv/docker":                           "docklet", // B1: the root itself
		"/srv":                                  "docklet", // B1: an ancestor
		"/srv/docker/docklet-agent/data":        "docklet", // other own mount: data
		"/srv/docker/docklet-agent/data/x":      "docklet",
		"/srv/docker/docklet-agent":             "docklet", // B2: install directory (compose working dir)
		"/srv/docker/docklet-agent/compose.yml": "docklet",
	} {
		require.Equal(t, want, reason(t, Rules(source, "", rows, nil, true, own)), source)
	}
}

func TestOtherDockletContainersStayStrict(t *testing.T) {
	// B3: the exemption applies to the executing container's own projects mount only.
	other := dockletContainer("other-agent", "agent", "", bind("/srv/docker", "/srv/docker"))
	require.Equal(t, "docklet", reason(t, Rules("/srv/docker/app/data", "", []container.Summary{other}, nil, true,
		fixedOwn(Own{ContainerID: "agent-id", Projects: "/srv/docker"}))))
	// A different destination on the own container is not the projects mount.
	self := dockletContainer("agent-id", "agent", "", bind("/srv/docker", "/srv/elsewhere"))
	require.Equal(t, "docklet", reason(t, Rules("/srv/docker/app/data", "", []container.Summary{self}, nil, true,
		fixedOwn(Own{ContainerID: "agent-id", Projects: "/srv/docker"}))))
}

func TestOwnProjectsMountAloneProtectsAncestors(t *testing.T) {
	only := dockletContainer("agent-id", "agent", "", bind("/srv/docker", "/srv/docker"))
	own := fixedOwn(Own{ContainerID: "agent-id", Projects: "/srv/docker"})
	for source, want := range map[string]string{"/srv": "docklet", "/srv/docker": "docklet", "/srv/docker/app": ""} {
		require.Equal(t, want, reason(t, Rules(source, "", []container.Summary{only}, nil, true, own)), source)
	}
}

func TestOwnLookupFailureKeepsStrictProtection(t *testing.T) {
	// B5: zero Own (lookup failed, not in a container) refuses as before.
	agent := dockletContainer("agent-id", "agent", "", bind("/srv/docker", "/srv/docker"))
	require.Equal(t, "docklet", reason(t, Rules("/srv/docker/app/data", "", []container.Summary{agent}, nil, true, Strict)))
}

func TestOwnLookupRunsOnlyForOverlappingDockletMounts(t *testing.T) {
	calls := 0
	lookup := func() Own { calls++; return Own{} }
	workload := container.Summary{ID: "app", Mounts: []container.MountPoint{bind("/srv/app", "/data")}}
	hub := dockletContainer("hub", "hub", "", bind("/srv/docklet/data", "/app/data"))
	require.NoError(t, Rules("/srv/app", "", []container.Summary{workload, hub}, nil, true, lookup))
	require.Zero(t, calls)
}

func TestOwnProjectsIdentifiesTheExecutingDockletContainer(t *testing.T) {
	t.Setenv("PROJECTS_DIRECTORY", "/srv/docker")
	for role, want := range map[string]Own{
		"agent":    {ContainerID: "own-id", Projects: "/srv/docker"},
		"hub":      {ContainerID: "own-id", Projects: "/srv/docker"},
		"workload": {},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(r.URL.Path, "/containers/json") {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`{"Id":"own-id","Config":{"Labels":{"io.docklet.system":"` + role + `"}}}`))
		}))
		cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
		require.NoError(t, err)
		require.Equal(t, want, OwnProjects(t.Context(), cli, nil)(), role)
		require.NoError(t, cli.Close())
		server.Close()
	}
}
