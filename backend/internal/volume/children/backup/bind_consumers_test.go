// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backup

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBindConsumersOverlapAndSelfRefusal(t *testing.T) {
	row := func(id, source string, labels map[string]string) container.Summary {
		return container.Summary{ID: id, Names: []string{"/" + id}, State: container.StateRunning, Labels: labels, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: source}, {Type: mount.TypeBind, Source: source}}}
	}
	rows := []container.Summary{row("equal", "/srv/app", nil), row("parent", "/srv", nil), row("child", "/srv/app/data", nil), row("sibling", "/srv/app2", nil), row("helper", "/srv/app", volumehelper.Labels())}
	started := []string{}
	svc := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.True(t, strings.HasSuffix(r.URL.Path, "/containers/json"))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(rows))
	}), &started)
	consumers, err := svc.listConsumers(t.Context(), volume.BindKey("/srv/app"), true)
	require.NoError(t, err)
	ids := []string{}
	for _, c := range consumers {
		ids = append(ids, c.ID)
	}
	require.Equal(t, []string{"equal", "parent", "child"}, ids)
	for _, labels := range []map[string]string{{"com.getarcaneapp.arcane": "true"}, {"io.docklet.system": "agent", "com.getarcaneapp.arcane.hidden": "true"}, {"io.docklet.system": "hub"}} {
		rows = append(rows[:5:5], row("self", "/srv/app/secrets", labels))
		_, err = svc.listConsumers(t.Context(), volume.BindKey("/srv/app"), true)
		require.EqualError(t, err, "volume.restore.self-mounted", "%v", labels)
	}
}
func TestProtectedBindBackupsRemainDownloadable(t *testing.T) {
	f := newRuntimePlan(t)
	entry := &volume.Backup{ID: "retained", VolumeName: volume.BindKey("/etc/config"), Destination: "archive", Format: volume.BackupFormatRustic, LocalSnapshotID: strings.Repeat("a", 64), Size: 6}
	f.service.deps.Store.Run = func(context.Context, string) (*volume.Backup, error) { return entry, nil }
	reader, _, err := f.service.DownloadBackup(f.targetContext("local"), entry.ID, nil)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	for _, created := range f.creates {
		for _, m := range created.HostConfig.Mounts {
			require.NotEqual(t, "/etc/config", m.Source, "by-ID recovery never mounts the protected source")
		}
	}
}

func TestBindConsumersAllowStackDataBelowADockletProjectsMount(t *testing.T) {
	agent := container.Summary{ID: "agent", Names: []string{"/agent"}, State: container.StateRunning,
		Labels: map[string]string{"io.docklet.system": "agent", "com.docker.compose.project.working_dir": "/srv/docker/docklet-agent"},
		Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: "/srv/docker", Destination: "/srv/docker"}, {Type: mount.TypeBind, Source: "/srv/docker/docklet-agent/data", Destination: "/app/data"}}}
	app := container.Summary{ID: "app", Names: []string{"/app"}, State: container.StateRunning,
		Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: "/srv/docker/app/data", Destination: "/data"}}}
	started := []string{}
	svc := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode([]container.Summary{agent, app}))
	}), &started)
	// A stack bind below the agent's projects mount is restorable; its workload consumer is stopped.
	consumers, err := svc.listConsumers(t.Context(), volume.BindKey("/srv/docker/app/data"), true)
	require.NoError(t, err)
	require.Len(t, consumers, 1)
	require.Equal(t, "app", consumers[0].ID)
	// The agent's state mount and install directory still refuse a restore into them.
	for _, source := range []string{"/srv/docker/docklet-agent/data", "/srv/docker/docklet-agent/data/docklet.db"} {
		_, err = svc.listConsumers(t.Context(), volume.BindKey(source), true)
		require.EqualError(t, err, "volume.restore.self-mounted", source)
	}
}
