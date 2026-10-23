// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 17796cbc61cdb65fd8ef7236ba301a2fe0286008
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original real producer baseline coverage through HTTP and the Edge tunnel.

package volume

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	workspacechild "github.com/florianfinn/docklet-next/backend/internal/volume/children/workspace"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/acfs"
	"go.getarcane.app/acfs/types"
)

// The same workspace handler/service is registered on hub and both agent modes.
// Only Docker is replaced: tree scans, multipart parsing, content comparison,
// root-confined ACFS apply and (for Edge) the real command tunnel remain active.
func TestVolumeBaselineGuardThroughLocalHTTPAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, tc := range []struct {
			name, actual, baseline string
			conflict               bool
		}{
			{"P5", "uploaded A", "old A", true}, {"P6", "old B", "old A", true}, {"matching", "old A", "old A", false},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				root, staging := t.TempDir(), t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte(tc.actual), 0600))
				copies, applies := 0, 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch {
					case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/volumes/data"):
						_, _ = io.WriteString(w, `{"Name":"data","Driver":"local"}`)
					case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/containers/json"):
						_, _ = io.WriteString(w, `[]`)
					case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/containers/helper/exec"):
						var request struct{ Cmd []string }
						require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
						require.Equal(t, "walk", request.Cmd[1])
						_, _ = io.WriteString(w, `{"Id":"walk"}`)
					case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/exec/walk/start"):
						_, _ = io.Copy(io.Discard, r.Body)
						var output bytes.Buffer
						count := 0
						err := acfs.Walk(t.Context(), root, "/", func(entry types.Entry) error {
							count++
							return json.NewEncoder(&output).Encode(types.WalkRecord{Version: types.ProtocolVersion, Entry: &entry})
						})
						require.NoError(t, err)
						require.NoError(t, json.NewEncoder(&output).Encode(types.WalkRecord{Version: types.ProtocolVersion, End: true, Count: count}))
						writeDockerExecAttachResponseInternal(t, w, output.String())
					case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/exec/walk/json"):
						_, _ = io.WriteString(w, `{"Running":false,"ExitCode":0}`)
					case r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/containers/helper/archive"):
						copies++
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
					default:
						t.Errorf("unexpected Docker request %s %s", r.Method, r.URL.Path)
						w.WriteHeader(500)
					}
				}))
				defer server.Close()
				cli := newVolumeWorkspaceTestDockerClientInternal(t, server)
				svc := workspacechild.NewService(workspacechild.Dependencies{
					Docker: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli), Locks: &utils.KeyedMutex{},
					AcquireHelper: func(context.Context, string) (string, func(), error) { return "helper", func() {}, nil },
					RequireACFS:   func(context.Context, string, string) error { return nil },
					Exec: func(ctx context.Context, _ string, _ string, cmd []string) (string, string, error) {
						if cmd[0] == "stat" {
							return "0:0", "", nil
						}
						if cmd[0] == "sh" {
							if strings.Contains(cmd[2], "tar -cf") {
								return "present\x00", "", nil
							}
							return "", "", nil
						}
						if cmd[0] == "mkdir" || cmd[0] == "tar" {
							return "", "", nil
						}
						if cmd[0] == "acfs" && cmd[1] == "read" {
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
						}
						if cmd[0] == "acfs" && cmd[1] == "apply" {
							applies++
							data, err := os.ReadFile(filepath.Join(staging, cmd[len(cmd)-1]))
							require.NoError(t, err)
							var manifest types.ApplyManifest
							require.NoError(t, json.Unmarshal(data, &manifest))
							count, err := acfs.Apply(ctx, root, staging, manifest)
							if err != nil {
								return "", "", err
							}
							return fmt.Sprintf(`{"version":%d,"applied":%d}`, types.ProtocolVersion, count), "", nil
						}
						return "", "", fmt.Errorf("unexpected command %q", cmd)
					},
				})
				tree, err := svc.GetVolumeWorkspace(t.Context(), "data")
				require.NoError(t, err)
				router := echo.New()
				router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
					return func(c *echo.Context) error {
						grants := authz.NewPermissionSet()
						grants.AddEnv("0", authz.PermVolumesRead, authz.PermVolumesUpload)
						ctx := context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, grants)
						ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &user.Actor{ID: "example-user", Username: "example-user"})
						c.SetRequest(c.Request().WithContext(ctx))
						return next(c)
					}
				})
				api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("workspace", "test"))
				workspacechild.RegisterRoutes(api, workspacechild.NewHandler(svc, nil, t.Context()))
				var body bytes.Buffer
				form := multipart.NewWriter(&body)
				upload, baseline := 0, 1
				manifest, err := json.Marshal(volumetypes.WorkspaceUpdateManifest{FileTreeRevision: tree.FileTreeRevision, FileChanges: []volumetypes.WorkspaceFileChange{{Operation: volumetypes.FileOpUpdateFile, RelativePath: "a.txt", UploadIndex: &upload, BaselineIndex: &baseline}}})
				require.NoError(t, err)
				require.NoError(t, form.WriteField("manifest", string(manifest)))
				for _, text := range []string{"my edit", tc.baseline} {
					part, err := form.CreateFormFile("files", "a.txt")
					require.NoError(t, err)
					_, err = io.WriteString(part, text)
					require.NoError(t, err)
				}
				require.NoError(t, form.Close())
				call := pruneTransport(t, mode, router, form.FormDataContentType())
				status, response := call("PUT", "/api/environments/0/volumes/data/workspace", "", body.Bytes())
				data, err := os.ReadFile(filepath.Join(root, "a.txt"))
				require.NoError(t, err)
				if tc.conflict {
					require.Equal(t, 409, status, string(response))
					require.Equal(t, tc.actual, string(data))
					require.Zero(t, applies)
					var problem huma.ErrorModel
					require.NoError(t, json.Unmarshal(response, &problem))
					require.Equal(t, "Conflict", problem.Title)
					require.Contains(t, problem.Detail, "file content changed")
				} else {
					require.Equal(t, 200, status, string(response))
					require.Equal(t, "my edit", string(data))
					require.Equal(t, 1, applies)
				}
				require.Greater(t, copies, 0, "the whole-tree revision passed and uploads were staged")
			})
		}
	}
}
