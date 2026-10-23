// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 17796cbc61cdb65fd8ef7236ba301a2fe0286008
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original baseline guard tests using the real root-confined ACFS producer.

package workspace

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/acfs"
	"go.getarcane.app/acfs/types"
)

func TestVolumeUpdateBaselineContentGuard(t *testing.T) {
	for _, tc := range []struct {
		name, actual, baseline string
		supplied, conflict     bool
	}{
		{"P5 replaced upload", "uploaded A", "old A", true, true},
		{"P6 renamed identity", "old B", "old A", true, true},
		{"matching", "old A", "old A", true, false},
		{"empty matches", "", "", true, false},
		{"empty differs", "x", "", true, true},
		{"longer", "old A extra", "old A", true, true},
		{"shorter", "old", "old A", true, true},
		{"optional absent", "other", "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, staging := t.TempDir(), t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte(tc.actual), 0600))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				archive := tar.NewReader(r.Body)
				for {
					header, err := archive.Next()
					if err == io.EOF {
						break
					}
					require.NoError(t, err)
					data, err := io.ReadAll(archive)
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(filepath.Join(staging, filepath.Base(header.Name)), data, 0600))
				}
			}))
			defer server.Close()
			cli := newVolumeWorkspaceTestDockerClientInternal(t, server)
			applies, rollbacks := 0, 0
			service := &Service{deps: Dependencies{Exec: func(ctx context.Context, _ string, _ string, cmd []string) (string, string, error) {
				switch cmd[1] {
				case "read":
					limit, err := strconv.ParseInt(cmd[len(cmd)-1], 10, 64)
					require.NoError(t, err)
					reader, size, err := acfs.OpenRead(ctx, root, cmd[5], limit)
					if err != nil {
						return "", "", err
					}
					defer reader.Close()
					var out bytes.Buffer
					require.NoError(t, acfs.WriteStreamHeader(&out, uint64(size)))
					_, err = io.Copy(&out, reader)
					return out.String(), "", err
				case "apply":
					applies++
					data, err := os.ReadFile(filepath.Join(staging, cmd[len(cmd)-1]))
					require.NoError(t, err)
					var manifest types.ApplyManifest
					require.NoError(t, json.Unmarshal(data, &manifest))
					count, err := acfs.Apply(ctx, root, staging, manifest)
					if err != nil {
						return "", "", err
					}
					out, err := json.Marshal(types.ApplyResponse{Version: types.ProtocolVersion, Applied: count})
					return string(out), "", err
				default:
					return "", "", nil
				}
			}}}
			upload, baseline := 0, 1
			change := volume.WorkspaceFileChange{Operation: volume.FileOpUpdateFile, RelativePath: "a.txt", UploadIndex: &upload}
			if tc.supplied {
				change.BaselineIndex = &baseline
			}
			staged, err := service.stageVolumeWorkspaceChangesInternal(t.Context(), cli, "helper", []volume.WorkspaceFileChange{change}, map[int][]byte{0: []byte("my edit"), 1: []byte(tc.baseline)}, volumeWorkspaceWriteIdentityInternal{})
			require.NoError(t, err)
			err = service.applyVolumeWorkspaceChangesInternal(t.Context(), cli, "helper", "data", []volume.WorkspaceFileChange{change}, staged, volumeWorkspaceWriteIdentityInternal{}, func() error { rollbacks++; return nil })
			data, readErr := os.ReadFile(filepath.Join(root, "a.txt"))
			require.NoError(t, readErr)
			if tc.conflict {
				require.ErrorIs(t, err, common.ErrVolumeWorkspaceConflict, "stale baseline must reject a current-revision update")
				require.Equal(t, tc.actual, string(data), "conflict must preserve producer content")
				require.Zero(t, applies)
				require.Zero(t, rollbacks, "a failed guard before any apply must leave external content untouched")
				problem := volumeWorkspaceHTTPErrorInternal(err).(*huma.ErrorModel)
				require.Equal(t, 409, problem.Status)
				require.Equal(t, "Conflict", problem.Title)
				require.NotEmpty(t, problem.Detail)
			} else {
				require.NoError(t, err)
				require.Equal(t, "my edit", string(data))
				require.Equal(t, 1, applies)
				require.Zero(t, rollbacks)
			}
		})
	}
}

func TestVolumeBaselineAfterOrderedRename(t *testing.T) {
	for _, baseline := range []string{"old B", "old A"} {
		t.Run(baseline, func(t *testing.T) {
			root, staging := t.TempDir(), t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "b.txt"), []byte("old B"), 0600))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				archive := tar.NewReader(r.Body)
				for {
					header, err := archive.Next()
					if err == io.EOF {
						break
					}
					require.NoError(t, err)
					data, err := io.ReadAll(archive)
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(filepath.Join(staging, filepath.Base(header.Name)), data, 0600))
				}
			}))
			defer server.Close()
			cli := newVolumeWorkspaceTestDockerClientInternal(t, server)
			applies, rollbacks := 0, 0
			service := &Service{deps: Dependencies{Exec: func(ctx context.Context, _ string, _ string, cmd []string) (string, string, error) {
				if cmd[0] != "acfs" {
					return "", "", nil
				}
				if cmd[1] == "read" {
					// The earlier rename must have materialised the target before comparison.
					require.Equal(t, 1, applies)
					require.Equal(t, "/a.txt", cmd[5])
					data, err := os.ReadFile(filepath.Join(root, "a.txt"))
					require.NoError(t, err)
					var out bytes.Buffer
					require.NoError(t, acfs.WriteStreamHeader(&out, uint64(len(data))))
					out.Write(data)
					return out.String(), "", nil
				}
				data, err := os.ReadFile(filepath.Join(staging, cmd[len(cmd)-1]))
				require.NoError(t, err)
				var manifest types.ApplyManifest
				require.NoError(t, json.Unmarshal(data, &manifest))
				count, err := acfs.Apply(ctx, root, staging, manifest)
				require.NoError(t, err)
				applies++
				response, err := json.Marshal(types.ApplyResponse{Version: types.ProtocolVersion, Applied: count})
				return string(response), "", err
			}}}
			upload, original := 0, 1
			changes := []volume.WorkspaceFileChange{{Operation: volume.FileOpRename, RelativePath: "b.txt", NewName: "a.txt"}, {Operation: volume.FileOpUpdateFile, RelativePath: "a.txt", UploadIndex: &upload, BaselineIndex: &original}}
			staged, err := service.stageVolumeWorkspaceChangesInternal(t.Context(), cli, "helper", changes, map[int][]byte{0: []byte("my edit"), 1: []byte(baseline)}, volumeWorkspaceWriteIdentityInternal{})
			require.NoError(t, err)
			err = service.applyVolumeWorkspaceChangesInternal(t.Context(), cli, "helper", "data", changes, staged, volumeWorkspaceWriteIdentityInternal{}, func() error {
				rollbacks++
				return os.Rename(filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt"))
			})
			if baseline == "old B" {
				require.NoError(t, err)
				require.Equal(t, 2, applies)
				require.Zero(t, rollbacks)
				data, err := os.ReadFile(filepath.Join(root, "a.txt"))
				require.NoError(t, err)
				require.Equal(t, "my edit", string(data))
			} else {
				require.ErrorIs(t, err, common.ErrVolumeWorkspaceConflict)
				require.Equal(t, 1, applies)
				require.Equal(t, 1, rollbacks)
				data, err := os.ReadFile(filepath.Join(root, "b.txt"))
				require.NoError(t, err)
				require.Equal(t, "old B", string(data))
				_, err = os.Stat(filepath.Join(root, "a.txt"))
				require.True(t, os.IsNotExist(err))
			}
		})
	}
}
