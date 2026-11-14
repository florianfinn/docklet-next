// Source: docklet-next/issues/394
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original collection route regressions for issue #394.

package volume

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/volume"
	"github.com/stretchr/testify/require"
)

// Volumes may carry the names of the collection operations; inspect must reach each of them on the
// hub-local handler and through the real Edge command mapping.
func TestVolumesNamedLikeCollectionOperationsInspect(t *testing.T) {
	names := []string{"binds", "counts", "sizes", "prune"}
	raw := map[string]volume.Volume{}
	for _, name := range names {
		v := pruneVolume(name)
		v.Mountpoint = "/var/lib/docker/volumes/" + name + "/_data"
		v.Labels = map[string]string{"example.org/name": name}
		raw[name] = v
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		for _, name := range names {
			if strings.HasSuffix(r.URL.Path, "/volumes/"+name) {
				_ = json.NewEncoder(w).Encode(raw[name])
				return
			}
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]container.Summary{})
		case strings.HasSuffix(r.URL.Path, "/volumes"):
			list := volume.ListResponse{}
			for _, name := range names {
				list.Volumes = append(list.Volumes, raw[name])
			}
			_ = json.NewEncoder(w).Encode(list)
		case strings.HasSuffix(r.URL.Path, "/system/df"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Volumes": []volume.Volume{}})
		default:
			t.Errorf("unexpected Docker request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(server.Close)
	svc := &VolumeService{dockerService: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeServiceTestDockerClientInternal(t, server)), settingsService: usageSettings(t)}
	for _, transport := range []string{"direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			call := pruneTransport(t, transport, collectionRouter(t, svc))
			for _, name := range names {
				status, body := call("GET", "/api/environments/0/volumes/"+name, "", nil)
				require.Equal(t, 200, status, string(body))
				var detail base.ApiResponse[volumetypes.Volume]
				require.NoError(t, json.Unmarshal(body, &detail), name)
				require.Equal(t, name, detail.Data.Name)
				require.Equal(t, raw[name].Mountpoint, detail.Data.Mountpoint)
				require.Equal(t, raw[name].Labels, detail.Data.Labels)
			}
			status, body := call("GET", "/api/environments/0/volumes/_collection/counts", "", nil)
			require.Equal(t, 200, status, string(body))
			var counts base.ApiResponse[VolumeUsageCountsData]
			require.NoError(t, json.Unmarshal(body, &counts))
			require.Equal(t, VolumeUsageCountsData{Total: len(names), Unused: len(names)}, counts.Data)
		})
	}
}

func collectionRouter(t *testing.T, svc *VolumeService) http.Handler {
	t.Helper()
	router := echo.New()
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			grants := authz.NewPermissionSet()
			grants.AddEnv("0", authz.PermVolumesRead, authz.PermVolumesList, authz.PermVolumesPrune)
			ctx := context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, grants)
			ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &user.Actor{ID: "example-user", Username: "example-user"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("storage", "test"))
	registerBindRoutes(api, svc)
	RegisterVolumes(api, nil, svc, nil, nil, handlerutil.NewActivityAppContext(t.Context()))
	return router
}

// The hub proxy validates bind keys in collection bodies before forwarding; the stand-in agent accepts
// everything, so only the proxy can refuse.
func TestHubProxyValidatesCollectionBodies(t *testing.T) {
	key := volumetypes.BindKey("/srv/app/")
	for _, mode := range []string{"hub-direct", "hub-edge"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			agent := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) })
			call := maintenanceReadTransport(t, mode, agent)
			for _, row := range []struct{ path, body string }{
				{"/volumes/_collection/prune", fmt.Sprintf(`{"candidates":[{"name":%q,"createdAt":"2026-11-01T00:00:00Z"}]}`, key)},
				{"/volumes/_collection/binds/size", fmt.Sprintf(`{"key":%q}`, key)},
			} {
				status, body := call("POST", "/api/environments/0"+row.path, "", []byte(row.body))
				require.Equal(t, 400, status, row.path+": "+string(body))
				require.Contains(t, string(body), "volume.bind.invalid-key")
			}
			require.Zero(t, calls.Load())
		})
	}
}
