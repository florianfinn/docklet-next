// Source: docklet-next/issues/191
// Revision: 2efbe8bbe02f930dc19929d07d4e64f6226701e3
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original volume usage route and snapshot regressions for issue #191.

package volume

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/types/base"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/volume"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func usageHelper(name string) container.Summary {
	c := pruneConsumer(name, container.StateRunning)
	c.ID = "workspace-helper"
	c.Labels = map[string]string{libarcane.InternalResourceLabel: "true", volumehelper.ContainerLabel: "true"}
	return c
}

func usageSettings(t *testing.T) *settings.SettingsService {
	t.Helper()
	gdb, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&settings.SettingVariable{}))
	svc, err := settings.NewSettingsService(t.Context(), &database.DB{DB: gdb})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, svc.Stop(context.Background())) })
	return svc
}

func TestHelperExcludedFromVolumeReadRoutes(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		for _, real := range []bool{false, true} {
			name := "helper-only"
			if real {
				name = "real-and-helper"
			}
			t.Run(transport+"/"+name, func(t *testing.T) {
				items := []container.Summary{usageHelper("data")}
				wantIDs := []string{}
				if real {
					consumer := pruneConsumer("data", container.StateExited)
					consumer.ID = "real-consumer"
					consumer.Mounts = append(consumer.Mounts, consumer.Mounts[0])
					items = append(items, consumer)
					wantIDs = append(wantIDs, consumer.ID)
				}
				raw := pruneVolume("data")
				raw.UsageData = &volume.UsageData{Size: 512, RefCount: int64(len(items))}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch {
					case strings.HasSuffix(r.URL.Path, "/containers/json"):
						require.Equal(t, "1", r.URL.Query().Get("all"))
						_ = json.NewEncoder(w).Encode(items)
					case strings.HasSuffix(r.URL.Path, "/volumes/data"):
						_ = json.NewEncoder(w).Encode(raw)
					case strings.HasSuffix(r.URL.Path, "/volumes"):
						_ = json.NewEncoder(w).Encode(volume.ListResponse{Volumes: []volume.Volume{raw}})
					case strings.HasSuffix(r.URL.Path, "/system/df"):
						_ = json.NewEncoder(w).Encode(map[string]any{"Volumes": []volume.Volume{raw}})
					default:
						t.Errorf("unexpected Docker request: %s %s", r.Method, r.URL.Path)
						w.WriteHeader(500)
					}
				}))
				t.Cleanup(server.Close)
				svc := &VolumeService{dockerService: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeServiceTestDockerClientInternal(t, server)), settingsService: usageSettings(t)}
				call := pruneTransport(t, transport, pruneRouter(t, svc))
				get := func(path, query string, dest any) {
					status, body := call("GET", "/api/environments/0/volumes"+path, query, nil)
					require.Equal(t, 200, status, string(body))
					require.NoError(t, json.Unmarshal(body, dest))
				}
				var sizes base.ApiResponse[[]VolumeSizeInfo]
				get("/sizes", "", &sizes)
				require.Equal(t, []VolumeSizeInfo{{Name: "data", Size: 512, RefCount: int64(len(wantIDs))}}, sizes.Data)
				var usage base.ApiResponse[VolumeUsageResponse]
				get("/data/usage", "", &usage)
				require.Equal(t, real, usage.Data.InUse)
				require.Equal(t, wantIDs, append([]string{}, usage.Data.Containers...))
				require.Len(t, usage.Data.Containers, len(wantIDs))
				var detail base.ApiResponse[volumetypes.Volume]
				get("/data", "", &detail)
				require.Equal(t, real, detail.Data.InUse)
				require.Equal(t, wantIDs, detail.Data.Containers)
				require.EqualValues(t, len(wantIDs), detail.Data.UsageData.RefCount)
				var list base.PaginatedWithCounts[volumetypes.Volume, VolumeUsageCountsData]
				get("", "sort=size", &list)
				require.Len(t, list.Data, 1)
				require.Equal(t, detail.Data, list.Data[0])
				wantCounts := VolumeUsageCountsData{Total: 1, Unused: 1}
				if real {
					wantCounts.Inuse, wantCounts.Unused = 1, 0
				}
				require.Equal(t, wantCounts, list.Counts)
				var counts base.ApiResponse[VolumeUsageCountsData]
				get("/counts", "", &counts)
				require.Equal(t, wantCounts, counts.Data)
				snapshot := svc.CountUsageFromSnapshot([]volume.Volume{raw}, items)
				require.Equal(t, volumetypes.UsageCounts{Total: 1, Inuse: wantCounts.Inuse, Unused: wantCounts.Unused}, snapshot)
			})
		}
	}
}

func TestVolumeConsumerSnapshotKeepsRealInternalContainersAndLegacyHelpers(t *testing.T) {
	helper := usageHelper("data")
	delete(helper.Labels, volumehelper.ContainerLabel)
	helper.Command = "sleep infinity"
	helper.Mounts[0].Destination = "/volume"
	real := pruneConsumer("data", container.StateCreated)
	real.ID = "internal-consumer"
	real.Labels = map[string]string{libarcane.InternalResourceLabel: "true"}
	real.Mounts[0].Destination = "/volume"
	require.Equal(t, map[string][]string{"data": {real.ID}}, volumeConsumersInternal([]container.Summary{helper, real}))
	raw := pruneVolume("data")
	raw.UsageData = &volume.UsageData{Size: 42, RefCount: 2}
	summary := volumeSummaryWithConsumersInternal(raw, nil)
	require.False(t, summary.InUse)
	require.EqualValues(t, 0, summary.UsageData.RefCount)
	require.EqualValues(t, 42, summary.UsageData.Size)
	require.EqualValues(t, 2, raw.UsageData.RefCount, "cached Docker usage must remain immutable")
}
