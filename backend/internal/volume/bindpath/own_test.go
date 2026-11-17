// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6af01ffd644ceb458b56c80d51c78e3f58847754
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original projects-mount exemption and install-directory protection tests.

package bindpath

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
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

const ownID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type fakeDaemon struct {
	inspect string // JSON body; empty answers HTTP 500
	list    string
}

func (f fakeDaemon) client(t *testing.T) *client.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info"):
			_, _ = w.Write([]byte(`{"DockerRootDir":"/var/lib/docker"}`))
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_, _ = w.Write([]byte(cmp.Or(f.list, `[]`)))
		case f.inspect == "":
			http.Error(w, "daemon error", http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte(f.inspect))
		}
	}))
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func inspectJSON(id, role, mounts string) string {
	return `{"Id":"` + id + `","Name":"/docklet-agent","Config":{"Labels":{"io.docklet.system":"` + role + `"}},"Mounts":[` + mounts + `]}`
}

const standardMounts = `{"Type":"bind","Source":"/srv/docker","Destination":"/srv/docker"},` +
	`{"Type":"bind","Source":"/srv/docker/docklet-agent/data","Destination":"/app/data"},` +
	`{"Type":"bind","Source":"/var/run/docker.sock","Destination":"/var/run/docker.sock"}`

func selfID(id string, err error) func() (string, error) {
	return func() (string, error) { return id, err }
}

func runtimeFor(projectsDir string, state ...string) *Runtime {
	return &Runtime{Projects: func(context.Context) string { return projectsDir }, State: append([]string{"/var/run/docker.sock"}, state...)}
}

func TestOwnProjectsIdentifiesOnlyTheExecutingDockletContainer(t *testing.T) {
	rt := runtimeFor("/srv/docker")
	want := Own{ContainerID: ownID, Projects: "/srv/docker"}
	for name, tc := range map[string]struct {
		daemon fakeDaemon
		id     func() (string, error)
		want   Own
	}{
		"own agent":                {fakeDaemon{inspect: inspectJSON(ownID, "agent", standardMounts)}, selfID(ownID, nil), want},
		"own hub, short id":        {fakeDaemon{inspect: inspectJSON(ownID, "hub", standardMounts)}, selfID(ownID[:12], nil), want},
		"identity unavailable":     {fakeDaemon{inspect: inspectJSON(ownID, "agent", standardMounts)}, selfID("", errors.New("no id")), Own{}},
		"identity is a name":       {fakeDaemon{inspect: inspectJSON(ownID, "agent", standardMounts)}, selfID("docklet-agent", nil), Own{}},
		"inspect fails":            {fakeDaemon{}, selfID(ownID, nil), Own{}},
		"inspect returns another":  {fakeDaemon{inspect: inspectJSON(strings.Repeat("f", 64), "agent", standardMounts)}, selfID(ownID, nil), Own{}},
		"not a Docklet container":  {fakeDaemon{inspect: inspectJSON(ownID, "workload", standardMounts)}, selfID(ownID, nil), Own{}},
		"shared network namespace": {fakeDaemon{inspect: inspectJSON(ownID, "agent", standardMounts), list: `[{"Id":"sidecar","HostConfig":{"NetworkMode":"container:` + ownID[:12] + `"}}]`}, selfID(ownID, nil), Own{}},
		"no projects bind mount":   {fakeDaemon{inspect: inspectJSON(ownID, "agent", `{"Type":"volume","Source":"/var/lib/docker/volumes/p/_data","Destination":"/srv/docker"}`)}, selfID(ownID, nil), Own{}},
		"own mount nested inside":  {fakeDaemon{inspect: inspectJSON(ownID, "agent", standardMounts+`,{"Type":"bind","Source":"/srv/secret","Destination":"/srv/docker/secret"}`)}, selfID(ownID, nil), Own{}},
	} {
		require.Equal(t, tc.want, ownProjects(t.Context(), tc.daemon.client(t), rt, tc.id), name)
	}
}

func TestOwnProjectsNeverExemptsDockletState(t *testing.T) {
	id := selfID(ownID, nil)
	for projectsDir, state := range map[string][]string{
		"/app/data":                {"/srv/db"},
		"/app":                     {"/srv/db"},
		"/var/lib/docklet/backups": {"/var/lib/docklet/backups"},
		"/srv/state":               {"/srv/state/registry"},
		"/srv/state/registry":      {"/srv/state/registry"},
		"/srv/projects":            {"/srv/projects/state"},   // database directory decoded by the database layer
		"/app/projects":            {"/app/projects/backups"}, // relative backup directory resolved at startup
	} {
		mounts := `{"Type":"bind","Source":"/srv/private-state","Destination":"` + projectsDir + `"}`
		require.Equal(t, Own{}, ownProjects(t.Context(), fakeDaemon{inspect: inspectJSON(ownID, "hub", mounts)}.client(t), runtimeFor(projectsDir, state...), id), projectsDir)
	}
}

func TestOwnProjectsNeedsTheRegisteredRuntime(t *testing.T) {
	id := selfID(ownID, nil)
	daemon := fakeDaemon{inspect: inspectJSON(ownID, "agent", standardMounts)}
	require.Equal(t, Own{}, ownProjects(t.Context(), daemon.client(t), nil, id), "not configured at startup")
	require.Equal(t, Own{}, ownProjects(t.Context(), daemon.client(t), runtimeFor("/srv/docker", "relative/state"), id), "relative state path")
	require.Equal(t, Own{}, ownProjects(t.Context(), daemon.client(t), runtimeFor(""), id), "default /app/data/projects is inside the data root")
	require.Equal(t, Own{ContainerID: ownID, Projects: "/srv/docker"}, ownProjects(t.Context(), daemon.client(t), runtimeFor("/srv/docker:/host/docker"), id), "container side of a mapping")
}

func TestCheckStaysStrictWhenProjectsDirectoryIsTheDataMount(t *testing.T) {
	// F-R1-2 end to end: the data mount configured as projects directory never exempts its contents.
	Configure(*runtimeFor("/app/data"))
	t.Cleanup(func() { runtime.Store(nil) })
	previous := CurrentContainerID
	CurrentContainerID = selfID(ownID, nil)
	t.Cleanup(func() { CurrentContainerID = previous })
	list := `[{"Id":"` + ownID + `","Labels":{"io.docklet.system":"hub"},"Mounts":[{"Type":"bind","Source":"/srv/private-state","Destination":"/app/data"}]},` +
		`{"Id":"app","Labels":{},"Mounts":[{"Type":"bind","Source":"/srv/private-state/credentials","Destination":"/data"}]}]`
	cli := fakeDaemon{inspect: inspectJSON(ownID, "hub", `{"Type":"bind","Source":"/srv/private-state","Destination":"/app/data"}`), list: list}.client(t)
	_, err := Check(t.Context(), cli, nil, volume.BindKey("/srv/private-state/credentials"))
	require.Equal(t, "docklet", reason(t, err))
}
