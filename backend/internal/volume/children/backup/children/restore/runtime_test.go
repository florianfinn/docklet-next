// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original Docker archive bridge, scratch command and ENOSPC effect proofs.

package restore

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func TestUploadRuntimeUsesScratchArchiveBridgeBeforeTargetEffects(t *testing.T) {
	for _, fail := range []string{"", "docker-enospc", "tar-enospc", "missing-target"} {
		t.Run(fail, func(t *testing.T) {
			l, run := lifecycleFixture(t)
			run.Kind = "upload"
			dir := t.TempDir()
			input := filepath.Join(dir, "input")
			require.NoError(t, os.WriteFile(input, archive(t, member{name: "file\nname", kind: tar.TypeReg, body: "payload"}), 0600))
			run.InputPath = input
			var commands [][]string
			var creates []container.HostConfig
			var labels []map[string]string
			var copied []string
			removed := []string{}
			targetEffects := 0
			targetCreated := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/volumes/create"):
					var body struct {
						Name   string
						Labels map[string]string
					}
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					if body.Name == "app-data" {
						require.Equal(t, "missing-target", fail)
						var stored Run
						require.NoError(t, l.DB.First(&stored, "id = ?", run.ID).Error)
						require.True(t, stored.CreatedVolume)
						targetCreated = true
						require.NoError(t, json.NewEncoder(w).Encode(body))
						return
					}
					require.Equal(t, "docklet-restore-current", body.Name)
					require.Equal(t, "true", body.Labels[libarcane.InternalResourceLabel])
					require.Equal(t, run.ID, body.Labels[RestoreLabel])
					require.NoError(t, json.NewEncoder(w).Encode(body))
				case strings.HasSuffix(r.URL.Path, "/containers/create"):
					var body struct {
						HostConfig container.HostConfig
						Labels     map[string]string
					}
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					creates = append(creates, body.HostConfig)
					labels = append(labels, body.Labels)
					id := "scratch-helper"
					if len(creates) > 1 {
						id = "target-writer"
						targetEffects++
					}
					require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"Id": id}))
				case strings.HasSuffix(r.URL.Path, "/start"):
					w.WriteHeader(204)
				case strings.HasSuffix(r.URL.Path, "/containers/scratch-helper/archive"):
					require.Equal(t, "/scratch/data", r.URL.Query().Get("path"))
					tr := tar.NewReader(r.Body)
					for {
						h, err := tr.Next()
						if err == io.EOF {
							break
						}
						require.NoError(t, err)
						copied = append(copied, h.Name)
						_, err = io.Copy(io.Discard, tr)
						require.NoError(t, err)
					}
					if fail == "docker-enospc" {
						w.WriteHeader(500)
						_, _ = w.Write([]byte(`{"message":"no space left on device"}`))
					} else {
						w.WriteHeader(200)
					}
				case strings.HasSuffix(r.URL.Path, "/volumes/app-data") && r.Method == http.MethodGet:
					if fail == "missing-target" && !targetCreated {
						w.WriteHeader(404)
						_, _ = w.Write([]byte(`{"message":"not found"}`))
						return
					}
					_, _ = w.Write([]byte(`{"Name":"app-data"}`))
				case r.Method == http.MethodDelete:
					removed = append(removed, r.URL.Path)
					w.WriteHeader(204)
				case strings.HasSuffix(r.URL.Path, "/json"):
					w.WriteHeader(404)
					_, _ = w.Write([]byte(`{"message":"not found"}`))
				default:
					t.Errorf("unexpected Docker request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
			require.NoError(t, err)
			defer cli.Close()
			s := NewService(Dependencies{Docker: docker.NewDockerClientService(t.Context(), l.DB, nil, nil).WithClient(cli), HelperImage: func(context.Context, *client.Client) (string, error) { return volumehelper.DefaultToolsImage, nil }, Exec: func(_ context.Context, id, _ string, args []string) (string, string, error) {
				commands = append(commands, args)
				if fail == "tar-enospc" && args[0] == "tar" {
					return "", "", errors.New("tar: no space left on device")
				}
				if id == "target-writer" {
					targetEffects++
				}
				return "", "", nil
			}})
			s.SetLifecycle(l)
			l.Effects.Prepare = s.PrepareRestore
			l.Effects.Write = s.WriteRestore
			// This test uses real preparation/write plans; consumer and safety effects
			// remain isolated and are forbidden after an upload preparation failure.
			l.Effects.Consumers = func(context.Context, string) ([]volume.StoppedContainer, error) {
				if fail == "docker-enospc" || fail == "tar-enospc" {
					t.Error("consumers stopped after scratch failure")
				}
				return nil, nil
			}
			l.Effects.Safety = func(context.Context, string, user.Actor) (string, error) {
				if fail == "missing-target" {
					t.Error("missing upload target must not get a safety snapshot")
				}
				if fail == "docker-enospc" || fail == "tar-enospc" {
					t.Error("safety attempted after scratch failure")
				}
				return "safety", nil
			}
			err = l.Execute(t.Context(), run, StartRequest{VolumeName: "app-data", Kind: "upload", InputPath: input})
			require.Equal(t, []string{"file\nname"}, copied)
			require.NotEmpty(t, removed)
			require.NotEmpty(t, creates)
			require.Empty(t, creates[0].Binds)
			require.Len(t, creates[0].Mounts, 1)
			require.Equal(t, "/scratch", creates[0].Mounts[0].Target)
			require.Equal(t, "docklet-restore-current", creates[0].Mounts[0].Source)
			if fail == "docker-enospc" || fail == "tar-enospc" {
				require.Error(t, err)
				require.Zero(t, targetEffects)
				require.Len(t, creates, 1)
				require.Equal(t, "preparing", run.Phase)
				return
			}
			require.NoError(t, err)
			if fail == "missing-target" {
				require.True(t, targetCreated)
				require.True(t, run.CreatedVolume)
				require.Empty(t, run.SafetyBackupID)
			}
			require.Len(t, creates, 2)
			require.Empty(t, creates[1].Binds)
			require.True(t, creates[1].Mounts[0].ReadOnly)
			require.Equal(t, "app-data", creates[1].Mounts[1].Source)
			for _, set := range labels {
				require.Equal(t, "true", set[libarcane.InternalResourceLabel])
				require.Equal(t, "current", set[RestoreLabel])
			}
			require.Equal(t, []string{"tar", "-C", "/scratch/data", "-cf", "/scratch/.docklet-copy.tar", "."}, commands[1])
			wipe := commands[len(commands)-1][2]
			require.Contains(t, wipe, "/volume/.[!.]* /volume/..?*")
			require.Contains(t, wipe, "tar -C /volume -xpf /scratch/.docklet-copy.tar")
			require.NotContains(t, wipe, "cp ")
			require.NotContains(t, wipe, "find ")
		})
	}
}
