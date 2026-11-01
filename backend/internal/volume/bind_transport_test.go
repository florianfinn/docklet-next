// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func bindTestRouter(t *testing.T, svc *VolumeService) http.Handler {
	t.Helper()
	router := echo.New()
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
			ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &user.SystemUser)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("binds", "test"))
	New(svc, svc.dockerService, nil, nil, nil).RegisterRoutes(api, handlerutil.NewActivityAppContext(t.Context()), &config.Config{AgentMode: true})
	return router
}
func TestBindValidationAndRefusalsThroughHubDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"hub-local", "agent-direct", "agent-edge", "hub-direct", "hub-edge"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/info"):
					_, _ = w.Write([]byte(`{"DockerRootDir":"/var/lib/docker"}`))
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					_, _ = w.Write([]byte(`[]`))
				default:
					t.Errorf("unexpected Docker effect %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer engine.Close()
			ds := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeServiceTestDockerClientInternal(t, engine))
			svc := NewVolumeService(nil, ds, nil, nil, nil, nil, nil, nil, nil, nil)
			call := maintenanceReadTransport(t, mode, bindTestRouter(t, svc))
			for _, key := range []string{"bind:2F737276", "bind:gg", volume.BindKey("/a/../b"), volume.BindKey("/a/"), volume.BindKey("/"), "bind:ff", volume.BindKey("relative"), volume.BindKey("/a\x00b"), volume.BindKey("/" + strings.Repeat("a", 255))} {
				before := calls.Load()
				status, body := call("GET", "/api/environments/0/volumes/"+key+"/workspace", "", nil)
				require.Equal(t, 400, status, string(body))
				require.Contains(t, string(body), "volume.bind.invalid-key")
				require.Equal(t, before, calls.Load())
			}
			key := volume.BindKey("/etc/config")
			for _, route := range []struct{ method, suffix string }{{"GET", "workspace"}, {"POST", "backups"}} {
				if route.suffix == "backups" && (mode == "hub-direct" || mode == "hub-edge") {
					continue
				}
				status, body := call(route.method, "/api/environments/0/volumes/"+key+"/"+route.suffix, "", []byte(`{}`))
				require.Equal(t, 422, status, string(body))
				var problem struct{ Code, Reason string }
				require.NoError(t, json.Unmarshal(body, &problem))
				require.Equal(t, "volume.bind.path-denied", problem.Code)
				require.Equal(t, "system", problem.Reason)
			}
		})
	}
}
func TestBindUnsupportedBeforeAnyDocker(t *testing.T) {
	h := &VolumeHandler{}
	key := volume.BindKey("/srv/app")
	_, err := h.CreateVolume(t.Context(), &CreateVolumeInput{Body: volume.Create{Name: key}})
	require.EqualError(t, err, "volume.bind.unsupported")
	_, err = h.RemoveVolume(t.Context(), &RemoveVolumeInput{VolumeName: key})
	require.EqualError(t, err, "volume.bind.unsupported")
	for _, input := range []*RenameVolumeInput{{VolumeName: key, Body: volume.Rename{Name: "other"}}, {VolumeName: "app", Body: volume.Rename{Name: key}}} {
		_, err = h.RenameVolume(t.Context(), input)
		require.EqualError(t, err, "volume.bind.unsupported")
	}
	_, err = (&VolumeService{}).ExecuteVolumePrune(t.Context(), []volume.PruneCandidate{{Name: "app"}, {Name: key}})
	require.EqualError(t, err, "volume.bind.unsupported")
}
