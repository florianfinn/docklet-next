// Source: docklet-next/issues/287
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package restore

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/volume/bindpath"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBindScratchRootThroughLifecycle(t *testing.T) {
	for _, kind := range []string{"valid", "write-fails", "extra", "directory", "symlink", "missing"} {
		t.Run(kind, func(t *testing.T) {
			l, run := lifecycleFixture(t)
			run.VolumeName = volume.BindKey("/srv/app.conf")
			require.NoError(t, l.DB.Save(run).Error)
			scratch := t.TempDir()
			live := filepath.Join(t.TempDir(), "app.conf")
			require.NoError(t, os.WriteFile(live, []byte("original"), 0640))
			if kind != "missing" {
				if kind == "directory" {
					require.NoError(t, os.Mkdir(filepath.Join(scratch, "app.conf"), 0700))
				} else if kind == "symlink" {
					require.NoError(t, os.Symlink(live, filepath.Join(scratch, "app.conf")))
				} else {
					require.NoError(t, os.WriteFile(filepath.Join(scratch, "app.conf"), []byte("restored"), 0600))
				}
			}
			if kind == "extra" {
				require.NoError(t, os.WriteFile(filepath.Join(scratch, ".extra"), nil, 0600))
			}
			var creates []struct {
				container.Config
				HostConfig container.HostConfig
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.Contains(r.URL.Path, "/images/"):
					_, _ = w.Write([]byte(`{"Id":"image"}`))
				case strings.HasSuffix(r.URL.Path, "/info"):
					_, _ = w.Write([]byte(`{"DockerRootDir":"/var/lib/docker"}`))
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					_, _ = w.Write([]byte(`[{"Id":"app","Mounts":[{"Type":"bind","Source":"/srv/app.conf","Destination":"/app.conf"}]}]`))
				case strings.HasSuffix(r.URL.Path, "/volumes/create"):
					_, _ = w.Write([]byte(`{"Name":"scratch"}`))
				case strings.HasSuffix(r.URL.Path, "/containers/create"):
					var c struct {
						container.Config
						HostConfig container.HostConfig
					}
					require.NoError(t, json.NewDecoder(r.Body).Decode(&c))
					creates = append(creates, c)
					_, _ = w.Write([]byte(`{"Id":"helper"}`))
				case strings.HasSuffix(r.URL.Path, "/start") || r.Method == "DELETE":
					w.WriteHeader(204)
				case strings.HasSuffix(r.URL.Path, "/wait"):
					_, _ = w.Write([]byte(`{"StatusCode":0}`))
				case strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json"):
					w.WriteHeader(404)
					_, _ = w.Write([]byte(`{"message":"removed"}`))
				case strings.HasSuffix(r.URL.Path, "/logs"):
					w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
				default:
					t.Errorf("unexpected Docker %s", r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
			require.NoError(t, err)
			defer cli.Close()
			engine := backup.NewEngine(t.Context(), nil, nil)
			writes := 0
			s := NewService(Dependencies{Docker: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli), Engine: engine, HelperImage: func(context.Context, *client.Client) (string, error) { return "tools", nil }, Exec: func(ctx context.Context, _, _ string, args []string) (string, string, error) {
				var stored Run
				require.NoError(t, l.DB.First(&stored, "id = ?", run.ID).Error)
				require.Equal(t, "safety", stored.SafetyBackupID)
				switch args[2] {
				case bindScratchCheck:
					require.Equal(t, "preparing", stored.Phase)
				case bindpath.RestoreFileScript:
					require.Equal(t, "writing", stored.Phase)
					writes++
					require.Equal(t, []string{"/scratch/app.conf", "/volume/app.conf"}, args[4:])
					if kind == "write-fails" {
						return "", "copy failed", errors.New("exit status 1")
					}
					out, e := exec.CommandContext(ctx, "/bin/sh", "-c", args[2], "sh", filepath.Join(scratch, "app.conf"), live).CombinedOutput()
					return string(out), "", e
				default:
					require.Contains(t, args[2], "/source", "only the bind witness runs besides the scratch check and the write")
					return "file", "", nil
				}
				script := strings.ReplaceAll(args[2], "/scratch", scratch)
				out, e := exec.CommandContext(ctx, "/bin/sh", "-c", script, "sh", "app.conf").CombinedOutput()
				return string(out), "", e
			}})
			run.Prepared = &preparedRestore{bindKind: "file", repository: backup.Repository{ID: "repository"}, snapshot: strings.Repeat("a", 64)}
			s.BindEffects(&l.Effects)
			before, err := os.Stat(live)
			require.NoError(t, err)
			err = l.Execute(t.Context(), run, StartRequest{VolumeName: run.VolumeName, Kind: "full"})
			content, readErr := os.ReadFile(live)
			require.NoError(t, readErr)
			if kind == "valid" {
				require.NoError(t, err)
				require.Equal(t, 1, writes)
				require.Equal(t, "restored", string(content))
				after, statErr := os.Stat(live)
				require.NoError(t, statErr)
				require.True(t, os.SameFile(before, after), "the mounted file keeps its inode")
				require.Len(t, creates, 4, "scratch restore, scratch check, bind witness and writer")
			} else if kind == "write-fails" {
				// The writer's own failure settles the run as failed after writing began.
				require.Error(t, err)
				require.Equal(t, 1, writes)
				require.Equal(t, "original", string(content))
				require.Len(t, creates, 4)
			} else {
				var p *dto.Problem
				require.ErrorAs(t, err, &p)
				require.Equal(t, 422, p.Status)
				require.Zero(t, writes)
				require.False(t, run.PendingRestart)
				require.Equal(t, "original", string(content))
				require.Len(t, creates, 2)
			}
			require.Contains(t, creates[0].HostConfig.Mounts, mount.Mount{Type: mount.TypeVolume, Source: "docklet-restore-" + run.ID, Target: "/scratch"})
			require.Equal(t, "/scratch", creates[0].Cmd[len(creates[0].Cmd)-1])
		})
	}
}
