// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package workspace

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestBindWriteIdentityAndStagedOwnership(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		users                 []string
		inspectFail, statFail bool
		uid, gid              int
	}{{"agree-plus-empty", []string{"1000:1000", ""}, false, false, 1000, 1000}, {"skip-named-and-root", []string{"app", "root", "0", "1000:1000"}, false, false, 1000, 1000}, {"disagree", []string{"1000:1000", "1001:1001"}, false, false, 2000, 2000}, {"inspect-fails", []string{"1000:1000"}, true, false, 2000, 2000}, {"none", nil, false, false, 2000, 2000}, {"stat-fails", nil, false, true, 0, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			source := "/srv/app"
			var stagedUID, stagedGID int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					rows := []container.Summary{}
					for i := range tc.users {
						rows = append(rows, container.Summary{ID: strconv.Itoa(i), Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: source}}})
					}
					require.NoError(t, json.NewEncoder(w).Encode(rows))
				case strings.HasSuffix(r.URL.Path, "/json"):
					if tc.inspectFail {
						http.Error(w, `{"message":"inspect failed"}`, 500)
						return
					}
					parts := strings.Split(r.URL.Path, "/")
					index, err := strconv.Atoi(parts[len(parts)-2])
					require.NoError(t, err)
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Config": map[string]string{"User": tc.users[index]}}))
				case strings.HasSuffix(r.URL.Path, "/archive"):
					reader := tar.NewReader(r.Body)
					header, err := reader.Next()
					require.NoError(t, err)
					stagedUID, stagedGID = header.Uid, header.Gid
					_, _ = io.Copy(io.Discard, r.Body)
				default:
					t.Errorf("unexpected %s", r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			cli := newVolumeWorkspaceTestDockerClientInternal(t, server)
			svc := &Service{deps: Dependencies{Exec: func(_ context.Context, _, _ string, args []string) (string, string, error) {
				if args[0] == "stat" {
					if tc.statFail {
						return "", "", errors.New("stat failed")
					}
					return "2000:2000", "", nil
				}
				return "", "", nil
			}}}
			identity := svc.resolveVolumeWorkspaceWriteIdentityInternal(t.Context(), cli, "helper", volume.BindKey(source))
			require.Equal(t, tc.uid, identity.uid)
			require.Equal(t, tc.gid, identity.gid)
			_, err := svc.stageVolumeWorkspaceChangesInternal(t.Context(), cli, "helper", []volume.WorkspaceFileChange{{Operation: volume.FileOpUpdateFile, RelativePath: "data", UploadIndex: new(0)}}, map[int][]byte{0: []byte("data")}, identity)
			require.NoError(t, err)
			require.Equal(t, tc.uid, stagedUID)
			require.Equal(t, tc.gid, stagedGID)
		})
	}
}
