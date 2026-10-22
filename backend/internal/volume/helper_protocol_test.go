// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 60d2471b62b12e1af64c54060a50102a2e39d1c1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original real helper protocol guard tests for issue #179.
package volume

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	volumeworkspace "github.com/florianfinn/docklet-next/backend/internal/volume/children/workspace"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/api/types/volume"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/acfs/types"
)

func TestWorkspaceRejectsIncompatibleHelperBeforeFileAccess(t *testing.T) {
	for _, protocol := range []struct {
		name, output string
		exit         int
	}{
		{"missing", "", 127}, {"older", `{"protocol":1}`, 0}, {"invalid", "invalid version", 0},
	} {
		for _, operation := range []string{"list", "read", "download", "update"} {
			t.Run(protocol.name+"/"+operation, func(t *testing.T) {
				commands := [][]string{}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case strings.Contains(r.URL.Path, "/volumes/"):
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(volume.Volume{Name: "workspace-volume", Driver: "local"})
					case strings.Contains(r.URL.Path, "/images/"):
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(map[string]string{"Id": "helper-image"})
					case strings.HasSuffix(r.URL.Path, "/containers/create"):
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(map[string]string{"Id": "helper"})
					case strings.HasSuffix(r.URL.Path, "/containers/helper/json"):
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(map[string]any{"Id": "helper", "State": map[string]bool{"Running": true}})
					case strings.HasSuffix(r.URL.Path, "/containers/helper/start"), r.Method == "DELETE":
						w.WriteHeader(204)
					case strings.HasSuffix(r.URL.Path, "/containers/helper/exec"):
						var request struct{ Cmd []string }
						require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
						commands = append(commands, request.Cmd)
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(map[string]string{"Id": "version"})
					case strings.HasSuffix(r.URL.Path, "/exec/version/start"):
						_, _ = io.Copy(io.Discard, r.Body)
						writeDockerExecAttachResponseInternal(t, w, protocol.output)
					case strings.HasSuffix(r.URL.Path, "/exec/version/json"):
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(map[string]any{"Running": false, "ExitCode": protocol.exit})
					default:
						t.Errorf("file access before compatibility guard: %s %s", r.Method, r.URL.Path)
						w.WriteHeader(500)
					}
				}))
				t.Cleanup(server.Close)
				svc := NewVolumeService(nil, docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeWorkspaceTestDockerClientInternal(t, server)), nil, nil, nil, nil, nil, nil, nil, nil)
				svc.helperByVolume["workspace-volume"] = &volumeHelper{id: "helper", lastUsedAt: time.Now()}
				var err error
				switch operation {
				case "list":
					_, err = svc.workspace.GetVolumeWorkspace(t.Context(), "workspace-volume")
				case "read":
					_, err = svc.workspace.GetVolumeWorkspaceFile(t.Context(), "workspace-volume", "file.txt")
				case "download":
					_, _, err = svc.workspace.DownloadVolumeWorkspaceFile(t.Context(), "workspace-volume", "file.txt")
				case "update":
					_, err = svc.workspace.UpdateVolumeWorkspace(t.Context(), "workspace-volume", volumetypes.WorkspaceUpdateManifest{FileTreeRevision: "revision", FileChanges: []volumetypes.WorkspaceFileChange{{Operation: volumetypes.FileOpDelete, RelativePath: "file.txt"}}}, nil, user.Actor{})
				}
				var incompatible *volumetypes.HelperIncompatibleError
				require.ErrorAs(t, err, &incompatible)
				require.Equal(t, "volume.helper-incompatible", incompatible.Code)
				require.Equal(t, 503, incompatible.GetStatus())
				require.Equal(t, [][]string{{"acfs", "version"}}, commands, "no workspace command may precede compatibility")
				if operation == "list" {
					router := echo.New()
					router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
						return func(c *echo.Context) error {
							grants := authz.NewPermissionSet()
							grants.AddEnv("0", authz.PermVolumesRead)
							c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, grants)))
							return next(c)
						}
					})
					api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("workspace", "test"))
					volumeworkspace.RegisterRoutes(api, volumeworkspace.NewHandler(svc.workspace, nil, t.Context()))
					for _, transport := range []string{"direct", "edge"} {
						call := pruneTransport(t, transport, router)
						status, body := call("GET", "/api/environments/0/volumes/workspace-volume/workspace", "", nil)
						require.Equal(t, 503, status, string(body))
						var problem volumetypes.HelperIncompatibleError
						require.NoError(t, json.Unmarshal(body, &problem))
						require.Equal(t, "volume.helper-incompatible", problem.Code)
					}
				}

			})
		}
	}
}

func TestHelperCachesCompatibleProtocol(t *testing.T) {
	svc := &VolumeService{helperByVolume: map[string]*volumeHelper{"data": {id: "helper", protocol: types.ProtocolVersion}}}
	require.NoError(t, svc.requireVolumeHelperACFSInternal(context.Background(), "data", "helper"))
}
