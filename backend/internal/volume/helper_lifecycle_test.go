// Source: docklet-next/issues/191
// Revision: 2efbe8bbe02f930dc19929d07d4e64f6226701e3
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original helper lifecycle regressions for issue #191.

package volume

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type helperLifecycleFixture struct {
	containers   []container.Summary
	volumes      map[string]volume.Volume
	removed      []string
	stopped      int
	stopStatus   int
	copies       int
	imageStarted chan struct{}
	imageProceed chan struct{}
	svc          *VolumeService
}

func (f *helperLifecycleFixture) service(t *testing.T) *VolumeService {
	t.Helper()
	f.volumes = map[string]volume.Volume{"data": pruneVolume("data")}
	f.containers = append([]container.Summary{usageHelper("data")}, f.containers...)
	holders := 0
	var imageBlocked atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			require.Equal(t, "1", r.URL.Query().Get("all"))
			_ = json.NewEncoder(w).Encode(f.containers)
		case r.Method == "DELETE" && strings.HasSuffix(r.URL.Path, "/containers/workspace-helper"):
			unlock, acquired := f.svc.workspaceLocks.TryLock("data")
			if acquired {
				unlock()
			}
			assert.False(t, acquired, "helper removal must hold the per-volume workspace lock")
			if f.stopStatus != 0 {
				w.WriteHeader(f.stopStatus)
				_, _ = io.WriteString(w, `{"message":"helper removal failed"}`)
				return
			}
			f.stopped++
			f.containers = slices.DeleteFunc(f.containers, func(c container.Summary) bool { return c.ID == "workspace-helper" })
			w.WriteHeader(204)
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/containers/create"):
			var request struct {
				Cmd        []string
				HostConfig container.HostConfig
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			if f.imageStarted != nil && slices.Equal(request.Cmd, []string{"sleep", "infinity"}) && slices.Equal(request.HostConfig.Binds, []string{"data:/volume"}) {
				if _, exists := f.volumes["data"]; !exists {
					f.volumes["data"] = pruneVolume("data")
				}
				f.containers = append(f.containers, usageHelper("data"))
				_ = json.NewEncoder(w).Encode(map[string]string{"Id": "workspace-helper"})
				return
			}
			holders++
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": fmt.Sprintf("holder-%d", holders)})
		case r.Method == "DELETE" && strings.Contains(r.URL.Path, "/containers/holder-"):
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/containers/workspace-helper/start"):
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/archive"):
			if r.Method == "GET" {
				stat, err := json.Marshal(container.PathStat{Name: "volume", Size: 12})
				require.NoError(t, err)
				w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(stat))
				_, _ = io.WriteString(w, "archive-data")
			} else {
				data, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.Equal(t, "archive-data", string(data))
				f.copies++
				w.WriteHeader(200)
			}
		case strings.Contains(r.URL.Path, "/images/"):
			if f.imageStarted != nil && imageBlocked.CompareAndSwap(false, true) {
				close(f.imageStarted)
				<-f.imageProceed
			}
			_, _ = io.WriteString(w, `{}`)
		case strings.HasSuffix(r.URL.Path, "/system/df"):
			_, _ = io.WriteString(w, `{"Volumes":[]}`)
		case strings.HasSuffix(r.URL.Path, "/volumes/create"):
			var options client.VolumeCreateOptions
			require.NoError(t, json.NewDecoder(r.Body).Decode(&options))
			v := pruneVolume(options.Name)
			f.volumes[options.Name] = v
			_ = json.NewEncoder(w).Encode(v)
		case strings.HasSuffix(r.URL.Path, "/volumes"):
			items := []volume.Volume{}
			for _, v := range f.volumes {
				items = append(items, v)
			}
			_ = json.NewEncoder(w).Encode(volume.ListResponse{Volumes: items})
		case strings.Contains(r.URL.Path, "/volumes/"):
			name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			v, ok := f.volumes[name]
			if !ok {
				http.Error(w, `{"message":"not found"}`, 404)
				return
			}
			if r.Method == "DELETE" {
				for _, c := range f.containers {
					for _, m := range c.Mounts {
						if m.Name == name {
							http.Error(w, `{"message":"volume is in use"}`, 409)
							return
						}
					}
				}
				assert.NotEqual(t, "1", r.URL.Query().Get("force"))
				f.removed = append(f.removed, name)
				delete(f.volumes, name)
				w.WriteHeader(204)
				return
			}
			_ = json.NewEncoder(w).Encode(v)
		default:
			t.Errorf("unexpected Docker request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(server.Close)
	gdb, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&event.Event{}))
	f.svc = &VolumeService{
		dockerService:   docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeServiceTestDockerClientInternal(t, server)),
		settingsService: usageSettings(t),
		eventService:    event.NewEventService(&database.DB{DB: gdb}, nil, nil),
		helperByVolume:  map[string]*volumeHelper{"data": {id: "workspace-helper"}},
	}
	return f.svc
}

func TestPruneStopsIdleHelperAndKeepsRealConsumers(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		for _, real := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/real=%t", transport, real), func(t *testing.T) {
				f := &helperLifecycleFixture{}
				if real {
					consumer := pruneConsumer("data", container.StateExited)
					consumer.ID = "real-consumer"
					f.containers = []container.Summary{consumer}
				}
				call := pruneTransport(t, transport, pruneRouter(t, f.service(t)))
				status, body := call("GET", "/api/environments/0/volumes/prune", "all=true", nil)
				require.Equal(t, 200, status, string(body))
				var plan base.ApiResponse[volumetypes.PrunePlan]
				require.NoError(t, json.Unmarshal(body, &plan))
				if real {
					require.Empty(t, plan.Data.Candidates)
				} else {
					require.Len(t, plan.Data.Candidates, 1)
				}
				require.Zero(t, f.stopped, "planning is read-only")
				status, body = call("POST", "/api/environments/0/volumes/prune", "", []byte(`{"candidates":[{"name":"data","createdAt":"`+pruneCreated+`"}]}`))
				require.Equal(t, 200, status, string(body))
				var result base.ApiResponse[volumetypes.PruneResult]
				require.NoError(t, json.Unmarshal(body, &result))
				if real {
					require.Equal(t, []volumetypes.PruneSkipped{{Name: "data", Reason: "in-use"}}, result.Data.Skipped)
					require.Empty(t, f.removed)
				} else {
					require.Equal(t, []string{"data"}, result.Data.Removed, "helper-only volume must be removed after stopping its helper")
					require.Equal(t, []string{"data"}, f.removed)
				}
				require.Equal(t, 1, f.stopped)
				require.Empty(t, result.Data.Failed)
			})
		}
	}
}

func TestRenameStopsIdleHelperAndKeepsRealConsumers(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		for _, real := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/real=%t", transport, real), func(t *testing.T) {
				f := &helperLifecycleFixture{}
				if real {
					consumer := pruneConsumer("data", container.StateExited)
					consumer.ID = "real-consumer"
					f.containers = []container.Summary{consumer}
				}
				call := pruneTransport(t, transport, pruneRouter(t, f.service(t)))
				status, body := call("POST", "/api/environments/0/volumes/data/rename", "", []byte(`{"name":"renamed"}`))
				if real {
					require.Equal(t, 409, status, string(body))
					require.Empty(t, f.removed)
					require.Zero(t, f.copies)
				} else {
					require.Equal(t, 200, status, string(body))
					var result base.ApiResponse[volumetypes.Volume]
					require.NoError(t, json.Unmarshal(body, &result))
					require.Equal(t, "renamed", result.Data.Name)
					require.False(t, result.Data.InUse)
					require.Empty(t, result.Data.Containers)
					require.Equal(t, []string{"data"}, f.removed)
					require.Equal(t, 1, f.copies)
				}
				require.Equal(t, 1, f.stopped)
			})
		}
	}
}

func TestHelperRemovalFailureRetainsVolumeAndCanRetry(t *testing.T) {
	for _, operation := range []string{"prune", "rename"} {
		t.Run(operation, func(t *testing.T) {
			f := &helperLifecycleFixture{stopStatus: 500}
			svc := f.service(t)
			run := func() error {
				if operation == "rename" {
					_, err := svc.RenameVolume(t.Context(), "data", "renamed", user.Actor{})
					return err
				}
				result, err := svc.ExecuteVolumePrune(t.Context(), []volumetypes.PruneCandidate{{Name: "data", CreatedAt: pruneCreated}})
				require.NoError(t, err)
				if len(result.Failed) > 0 {
					require.Equal(t, []volumetypes.PruneFailed{{Name: "data", Code: "volume.usage-failed"}}, result.Failed)
					return fmt.Errorf("prune candidate failed")
				}
				return nil
			}
			require.Error(t, run())
			require.Empty(t, f.removed)
			require.Zero(t, f.copies)
			require.NotNil(t, svc.helperByVolume["data"], "retain failed helper cleanup for retry")
			f.stopStatus = 0
			require.NoError(t, run())
			require.Equal(t, 1, f.stopped)
			require.Equal(t, []string{"data"}, f.removed)
		})
	}
}
