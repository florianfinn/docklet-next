// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backup

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBindCaptureUsesWitnessLayoutAndRootSource(t *testing.T) {
	for _, kind := range []string{"directory", "file", "unsupported"} {
		t.Run(kind, func(t *testing.T) {
			f := newRuntimePlan(t)
			require.NoError(t, f.service.deps.DB.AutoMigrate(&backuptarget.Target{}, &policies.VolumeBackupPolicy{}))
			source := "/srv/app.conf"
			key := volume.BindKey(source)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/info"):
					_, _ = w.Write([]byte(`{"DockerRootDir":"/var/lib/docker"}`))
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					require.NoError(t, json.NewEncoder(w).Encode([]container.Summary{{ID: "workload", Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: source, Destination: "/config"}}}}))
				default:
					f.serve(w, r)
				}
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
			require.NoError(t, err)
			defer cli.Close()
			f.service.deps.Docker.WithClient(cli)
			f.service.deps.Exec = func(context.Context, string, string, []string) (string, string, error) { return kind, "", nil }
			f.stdout = `{"id":"` + strings.Repeat("a", 64) + `","summary":{"total_bytes_processed":24}}`
			entry := &volume.Backup{ID: "capture", VolumeName: key, Destination: "archive"}
			err = f.service.createLocalBackupSnapshotInternal(rustic.WithOperation(f.targetContext("local"), key, "capture"), cli, entry)
			if kind == "unsupported" {
				require.EqualError(t, err, "volume.bind.unsupported-type")
				require.Len(t, f.creates, 1)
				return
			}
			require.NoError(t, err)
			require.Len(t, f.creates, 2)
			engine := f.creates[1]
			require.Equal(t, rustic.DefaultImage, engine.Image)
			require.Empty(t, engine.HostConfig.Binds)
			target := "/volume"
			if kind == "file" {
				target += "/app.conf"
			}
			require.Contains(t, engine.HostConfig.Mounts, mount.Mount{Type: mount.TypeBind, Source: source, Target: target, ReadOnly: true})
			require.Equal(t, "/volume", engine.Cmd[len(engine.Cmd)-1])
			require.Contains(t, strings.Join(engine.Cmd, " "), "--as-path /")
			require.Equal(t, key, engine.Labels[rustic.OperationVolumeLabel])
			require.EqualValues(t, 24, entry.Size)
		})
	}
}
