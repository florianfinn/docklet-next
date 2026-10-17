// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original fake-Docker read-path integration tests.

package docker_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/network"
	"github.com/florianfinn/docklet-next/backend/internal/port"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReadInventoryFakeDocker(t *testing.T) {
	containers := []map[string]any{
		{"Id": "app", "Names": []string{"/app"}, "Image": "example:1.0.0", "ImageID": "sha256:user", "State": "running", "Mounts": []map[string]any{{"Type": "volume", "Name": "user-data", "Destination": "/data"}}, "NetworkSettings": map[string]any{"Networks": map[string]any{"user-net": map[string]any{"NetworkID": "user-net"}}}, "Ports": []map[string]any{{"PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"}}},
		{"Id": "agent", "Names": []string{"/agent"}, "Image": "example-agent:1.0.0", "ImageID": "sha256:agent", "State": "running", "Labels": map[string]string{"io.docklet.system": "agent"}, "Mounts": []map[string]any{{"Type": "volume", "Name": "agent-data"}}, "NetworkSettings": map[string]any{"Networks": map[string]any{"agent-net": map[string]any{"NetworkID": "agent-net"}}}, "Ports": []map[string]any{{"PrivatePort": 3552, "PublicPort": 3552, "Type": "tcp"}}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var value any
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			value = containers
		case strings.HasSuffix(r.URL.Path, "/images/json"):
			value = []map[string]any{{"Id": "sha256:user", "RepoTags": []string{"example:1.0.0"}, "Size": 1048576}, {"Id": "sha256:agent", "RepoTags": []string{"example-agent:1.0.0"}}}
		case strings.HasSuffix(r.URL.Path, "/volumes"):
			value = map[string]any{"Volumes": []map[string]any{{"Name": "user-data", "Driver": "local", "Labels": map[string]string{}}, {"Name": "agent-data", "Driver": "local", "Labels": map[string]string{}}}}
		case strings.HasSuffix(r.URL.Path, "/networks"):
			value = []map[string]any{{"Id": "user-net", "Name": "user-net", "Driver": "bridge"}, {"Id": "agent-net", "Name": "agent-net", "Driver": "bridge"}, {"Id": "bridge", "Name": "bridge", "Driver": "bridge"}, {"Id": "host", "Name": "host", "Driver": "host"}, {"Id": "none", "Name": "none", "Driver": "null"}}
		case strings.HasSuffix(r.URL.Path, "/networks/user-net"):
			value = map[string]any{"Id": "user-net", "Name": "user-net", "Driver": "bridge", "Containers": map[string]any{"app": map[string]any{"Name": "app", "IPv4Address": "192.0.2.2/24"}, "agent": map[string]any{"Name": "agent"}}}
		case strings.Contains(r.URL.Path, "/networks/"):
			name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			driver := name
			if name == "none" {
				driver = "null"
			}
			value = map[string]any{"Id": name, "Name": name, "Driver": driver}
		default:
			http.NotFound(w, r)
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(value))
	}))
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	rawDB, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, rawDB.AutoMigrate(&settings.SettingVariable{}))
	settingsSvc, err := settings.NewSettingsService(t.Context(), &database.DB{DB: rawDB})
	require.NoError(t, err)
	svc := docker.NewDockerClientService(t.Context(), nil, nil, settingsSvc).WithClient(cli)
	params := pagination.QueryParams{Limit: 100}
	t.Run("images", func(t *testing.T) {
		rows, page, err := image.NewImageService(nil, svc, nil, nil, nil, nil, settingsSvc, nil).ListImagesPaginated(t.Context(), params)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.EqualValues(t, 1, page.TotalItems)
		require.Equal(t, "sha256:user", rows[0].ID)
		require.True(t, rows[0].InUse)
		require.Equal(t, "app", rows[0].UsedBy[0].ID)
	})
	t.Run("volumes", func(t *testing.T) {
		rows, page, counts, err := volume.NewVolumeService(nil, svc, nil, nil, settingsSvc, nil, nil, nil, nil, nil).ListVolumesPaginated(t.Context(), params, false)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.EqualValues(t, 1, page.TotalItems)
		require.Equal(t, 1, counts.Total)
		require.Equal(t, "user-data", rows[0].Name)
		require.Equal(t, []string{"app"}, rows[0].Containers)
	})
	t.Run("networks", func(t *testing.T) {
		service := network.NewNetworkService(nil, svc, nil)
		rows, page, counts, err := service.ListNetworksPaginated(t.Context(), params)
		require.NoError(t, err)
		require.Len(t, rows, 4)
		require.EqualValues(t, 4, page.TotalItems)
		require.Equal(t, 4, counts.Total)
		drivers := map[string]string{}
		for _, row := range rows {
			drivers[row.Name] = row.Driver
		}
		require.Equal(t, map[string]string{"user-net": "bridge", "bridge": "bridge", "host": "host", "none": "null"}, drivers)
		graph, err := service.GetNetworkTopology(t.Context())
		require.NoError(t, err)
		require.Len(t, graph.Nodes, 5)
		require.Len(t, graph.Edges, 1)
		require.Equal(t, "app", graph.Edges[0].Target)
	})
	t.Run("ports", func(t *testing.T) {
		rows, page, err := port.NewPortService(svc).ListPortsPaginated(t.Context(), params)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.EqualValues(t, 1, page.TotalItems)
		require.Equal(t, "app", rows[0].ContainerID)
		require.Equal(t, 8080, rows[0].HostPort)
	})
}
