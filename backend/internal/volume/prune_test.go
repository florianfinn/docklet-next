// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 60d2471b62b12e1af64c54060a50102a2e39d1c1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original stateless prune contract implementation for issue #179.
package volume

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/volume"
	"github.com/stretchr/testify/require"
)

const pruneCreated = "2026-10-22T00:00:00Z"

type pruneDockerFixture struct {
	volumes       []volume.Volume
	containers    []container.Summary
	removed       []string
	removeErrors  map[string]int
	inspectErrors map[string]int
	usageError    bool
}

func (f *pruneDockerFixture) service(t *testing.T) *VolumeService {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/volumes"):
			_ = json.NewEncoder(w).Encode(volume.ListResponse{Volumes: f.volumes})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/containers/json"):
			if f.usageError {
				http.Error(w, `{"message":"usage unavailable"}`, 500)
				return
			}
			// A broken implementation that omits All must see only running containers.
			list := []container.Summary{}
			for _, c := range f.containers {
				if r.URL.Query().Get("all") == "1" || c.State == container.StateRunning {
					list = append(list, c)
				}
			}
			_ = json.NewEncoder(w).Encode(list)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/system/df"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Volumes": f.volumes})
		case strings.Contains(r.URL.Path, "/volumes/"):
			name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			if r.Method == "DELETE" {
				require.NotEqual(t, "1", r.URL.Query().Get("force"), "prune must never force removal")
				if status := f.removeErrors[name]; status != 0 {
					w.WriteHeader(status)
					_, _ = io.WriteString(w, `{"message":"synthetic removal failure"}`)
					return
				}
				f.removed = append(f.removed, name)
				w.WriteHeader(204)
				return
			}
			if status := f.inspectErrors[name]; status != 0 {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"message":"synthetic inspect failure"}`)
				return
			}
			for _, v := range f.volumes {
				if v.Name == name {
					_ = json.NewEncoder(w).Encode(v)
					return
				}
			}
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"message":"no such volume"}`)
		default:
			t.Errorf("unexpected Docker request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(server.Close)
	return &VolumeService{dockerService: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeServiceTestDockerClientInternal(t, server)), helperByVolume: map[string]*volumeHelper{}}
}

func pruneVolume(name string) volume.Volume {
	return volume.Volume{Name: name, Driver: "local", CreatedAt: pruneCreated, UsageData: &volume.UsageData{Size: 12}}
}
func pruneConsumer(name string, state container.ContainerState) container.Summary {
	return container.Summary{State: state, Mounts: []container.MountPoint{{Type: mount.TypeVolume, Name: name}}}
}

func TestPrunePlanSelection(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			f := &pruneDockerFixture{}
			svc := f.service(t)
			router := pruneRouter(t, svc)
			call := pruneTransport(t, transport, router)
			get := func(all bool) volumetypes.PrunePlan {
				query := "all=false"
				if all {
					query = "all=true"
				}
				status, body := call("GET", "/api/environments/0/volumes/prune", query, nil)
				require.Equal(t, 200, status, string(body))
				var response base.ApiResponse[volumetypes.PrunePlan]
				require.NoError(t, json.Unmarshal(body, &response))
				require.True(t, response.Success)
				return response.Data
			}
			require.Empty(t, get(true).Candidates)
			named, anonymous, running, stopped := pruneVolume("named"), pruneVolume("anonymous"), pruneVolume("running"), pruneVolume("stopped")
			anonymous.Labels = map[string]string{anonymousVolumeLabel: ""}
			protected := pruneVolume("internal")
			protected.Labels = map[string]string{libarcane.InternalResourceLabel: "true"}
			remote := pruneVolume("remote")
			remote.Driver = "plugin"
			bind := pruneVolume("bind")
			bind.Options = map[string]string{"type": "none", "device": "/example"}
			f.volumes = []volume.Volume{named, anonymous, running, stopped, protected, remote, bind}
			f.containers = []container.Summary{pruneConsumer("running", container.StateRunning), pruneConsumer("stopped", container.StateExited)}
			plan := get(false)
			require.Equal(t, []volumetypes.PrunePlanCandidate{{Name: "anonymous", Driver: "local", CreatedAt: pruneCreated, Reason: "unused-anonymous", SizeBytes: &anonymous.UsageData.Size}}, plan.Candidates)
			plan = get(true)
			require.Len(t, plan.Candidates, 2)
			require.Equal(t, "named", plan.Candidates[1].Name)
			require.Equal(t, "unused", plan.Candidates[1].Reason)
			require.Empty(t, f.removed)
		})
	}
}

func TestExecutePruneRechecksCandidateIdentityAndUsage(t *testing.T) {
	for _, state := range []container.ContainerState{container.StateRunning, container.StateExited} {
		t.Run(string(state), func(t *testing.T) {
			f := &pruneDockerFixture{volumes: []volume.Volume{pruneVolume("ok"), pruneVolume("used"), pruneVolume("changed"), pruneVolume("failure"), pruneVolume("untouched"), pruneVolume("gone")}, removeErrors: map[string]int{"failure": 500}}
			svc := f.service(t)
			plan, err := svc.PlanVolumePrune(t.Context(), true)
			require.NoError(t, err)
			candidates := []volumetypes.PruneCandidate{}
			for _, c := range plan.Candidates {
				if c.Name != "untouched" {
					candidates = append(candidates, volumetypes.PruneCandidate{Name: c.Name, CreatedAt: c.CreatedAt})
				}
			}
			candidates = append(candidates, volumetypes.PruneCandidate{Name: "ok", CreatedAt: pruneCreated})
			// These changes occur after the real plan producer returned.
			f.containers = []container.Summary{pruneConsumer("used", state)}
			f.volumes[2].CreatedAt = "2026-10-22T01:00:00Z"
			f.volumes = f.volumes[:len(f.volumes)-1] // The planned gone volume was deleted meanwhile.
			result, err := svc.ExecuteVolumePrune(t.Context(), candidates)
			require.NoError(t, err)
			require.Equal(t, []string{"ok"}, result.Removed)
			require.ElementsMatch(t, []volumetypes.PruneSkipped{{Name: "used", Reason: "in-use"}, {Name: "changed", Reason: "changed"}, {Name: "gone", Reason: "not-found"}}, result.Skipped)
			require.Equal(t, []volumetypes.PruneFailed{{Name: "failure", Code: "volume.remove-failed"}}, result.Failed)
			require.EqualValues(t, 12, result.SpaceReclaimed)
			require.Equal(t, []string{"ok"}, f.removed, "only selected and re-checked candidates may be removed")
		})
	}
}

func TestExecutePruneAllEmptyAndDockerFailures(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result, err := (&VolumeService{}).ExecuteVolumePrune(t.Context(), nil)
		require.NoError(t, err)
		require.Equal(t, &volumetypes.PruneResult{Removed: []string{}, Skipped: []volumetypes.PruneSkipped{}, Failed: []volumetypes.PruneFailed{}}, result)
	})
	for _, transport := range []string{"direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			f := &pruneDockerFixture{volumes: []volume.Volume{pruneVolume("one"), pruneVolume("two"), pruneVolume("unselected")}}
			call := pruneTransport(t, transport, pruneRouter(t, f.service(t)))
			status, body := call("POST", "/api/environments/0/volumes/prune", "", nil)
			require.Equal(t, 400, status, string(body))
			request, err := json.Marshal(volumetypes.PruneRequest{Candidates: []volumetypes.PruneCandidate{{Name: "one", CreatedAt: pruneCreated}, {Name: "two", CreatedAt: pruneCreated}}})
			require.NoError(t, err)
			status, body = call("POST", "/api/environments/0/volumes/prune", "", request)
			require.Equal(t, 200, status, string(body))
			var response base.ApiResponse[volumetypes.PruneResult]
			require.NoError(t, json.Unmarshal(body, &response))
			require.Equal(t, []string{"one", "two"}, response.Data.Removed)
			require.Empty(t, response.Data.Skipped)
			require.Empty(t, response.Data.Failed)
			require.EqualValues(t, 24, response.Data.SpaceReclaimed)
			require.Equal(t, response.Data.Removed, f.removed)
		})
	}
	for _, tc := range []struct {
		name         string
		status       int
		code, reason string
	}{
		{"inspect", 500, "volume.inspect-failed", ""}, {"remove-conflict", 409, "", "in-use"}, {"remove-gone", 404, "", "not-found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &pruneDockerFixture{volumes: []volume.Volume{pruneVolume("one")}}
			if tc.name == "inspect" {
				f.inspectErrors = map[string]int{"one": tc.status}
			} else {
				f.removeErrors = map[string]int{"one": tc.status}
			}
			result, err := f.service(t).ExecuteVolumePrune(t.Context(), []volumetypes.PruneCandidate{{Name: "one", CreatedAt: pruneCreated}})
			require.NoError(t, err)
			if tc.code != "" {
				require.Equal(t, tc.code, result.Failed[0].Code)
			} else {
				require.Equal(t, tc.reason, result.Skipped[0].Reason)
			}
			require.Empty(t, result.Removed)
		})
	}
}

func TestExecutePruneFailsClosedForProtectedAndUnavailableUsage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*pruneDockerFixture)
		code      string
	}{
		{"internal", func(f *pruneDockerFixture) {
			f.volumes[0].Labels = map[string]string{libarcane.InternalResourceLabel: "true"}
		}, "volume.protected"},
		{"driver", func(f *pruneDockerFixture) { f.volumes[0].Driver = "plugin" }, "volume.protected"},
		{"options", func(f *pruneDockerFixture) { f.volumes[0].Options = map[string]string{"type": "nfs"} }, "volume.protected"},
		{"usage", func(f *pruneDockerFixture) { f.usageError = true }, "volume.usage-failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &pruneDockerFixture{volumes: []volume.Volume{pruneVolume("one")}}
			tc.configure(f)
			result, err := f.service(t).ExecuteVolumePrune(t.Context(), []volumetypes.PruneCandidate{{Name: "one", CreatedAt: pruneCreated}})
			require.NoError(t, err)
			require.Equal(t, []volumetypes.PruneFailed{{Name: "one", Code: tc.code}}, result.Failed)
			require.Empty(t, f.removed)
		})
	}
}

func pruneRouter(t *testing.T, svc *VolumeService) http.Handler {
	t.Helper()
	router := echo.New()
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			grants := authz.NewPermissionSet()
			grants.AddEnv("0", authz.PermVolumesPrune, authz.PermVolumesRename)
			ctx := context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, grants)
			ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &user.Actor{ID: "example-user", Username: "example-user"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("storage", "test"))
	RegisterVolumes(api, nil, svc, nil, nil, handlerutil.NewActivityAppContext(t.Context()))
	return router
}

type pruneCall func(method, path, query string, body []byte) (int, []byte)

// This fixture exercises the real command client, WebSocket tunnel, command
// validation and local Huma handler, with no fabricated producer JSON.
func pruneTransport(t *testing.T, mode string, handler http.Handler) pruneCall {
	t.Helper()
	if mode == "direct" {
		return func(method, path, query string, body []byte) (int, []byte) {
			if query != "" {
				path += "?" + query
			}
			req := httptest.NewRequest(method, path, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			return response.Code, response.Body.Bytes()
		}
	}
	registry := edge.NewTunnelRegistry()
	connected := make(chan struct{})
	var once sync.Once
	server := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return "example-storage", nil }, func(_ context.Context, _ string, online bool) {
		if online {
			once.Do(func() { close(connected) })
		}
	})
	router := echo.New()
	router.GET("/api/tunnel/connect", server.HandleConnect)
	manager := httptest.NewServer(router)
	t.Cleanup(manager.Close)
	client := edge.NewTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: uuid.New().String()}, handler)
	life, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); client.StartWithErrorChan(life, nil) }()
	t.Cleanup(func() {
		cancel()
		registry.Unregister("example-storage")
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("tunnel cleanup failed")
		}
	})
	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("tunnel registration barrier failed")
	}
	tunnel, ok := registry.Get("example-storage").Get()
	require.True(t, ok)
	return func(method, path, query string, body []byte) (int, []byte) {
		status, _, response, err := edge.ProxyRequest(t.Context(), tunnel, method, path, query, map[string]string{"Content-Type": "application/json"}, body)
		require.NoError(t, err)
		return status, response
	}
}

func TestRenameVolumeThroughActualEdgeTunnel(t *testing.T) {
	call := pruneTransport(t, "edge", pruneRouter(t, &VolumeService{}))
	status, body := call("POST", "/api/environments/0/volumes/data/rename", "", []byte(`{"name":"data"}`))
	require.Equal(t, 400, status, string(body))
	var problem huma.ErrorModel
	require.NoError(t, json.Unmarshal(body, &problem))
	require.Contains(t, problem.Detail, "source and target volume names must be non-empty and different")
}
