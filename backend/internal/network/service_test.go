// Source: docklet-next/docs/design/network-management.md
// Revision: 81b69b33c80a454465f54a1f7289c1bc5b5a3381
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package network

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	networktypes "github.com/florianfinn/docklet-next/types/network"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type networkDockerFixture struct {
	networks      []network.Inspect
	containers    []container.InspectResponse
	writes        []string
	created       network.CreateRequest
	connected     client.NetworkConnectOptions
	failList      bool
	vanishing     bool
	writeStatus   int
	writeMessage  string
	inspectStatus int
}

func (f *networkDockerFixture) service(t *testing.T) *NetworkService {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		if r.Method != "GET" && f.writeStatus != 0 {
			w.WriteHeader(f.writeStatus)
			message := f.writeMessage
			if message == "" {
				message = "synthetic Docker conflict"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
			return
		}
		switch {
		case r.Method == "GET" && strings.HasSuffix(path, "/networks"):
			var nets []network.Network
			for _, n := range f.networks {
				nets = append(nets, n.Network)
			}
			_ = json.NewEncoder(w).Encode(nets)
		case r.Method == "GET" && strings.HasSuffix(path, "/containers/json"):
			if f.failList {
				w.WriteHeader(500)
				_, _ = w.Write([]byte(`{"message":"list failed"}`))
				return
			}
			list := []container.Summary{}
			if f.inspectStatus != 0 {
				list = append(list, container.Summary{ID: "unreadable"})
			}
			if f.vanishing {
				list = append(list, container.Summary{ID: "gone"})
			}
			for _, c := range f.containers {
				if r.URL.Query().Get("all") == "1" || (c.State != nil && c.State.Running) {
					list = append(list, container.Summary{ID: c.ID})
				}
			}
			_ = json.NewEncoder(w).Encode(list)
		case r.Method == "GET" && strings.Contains(path, "/containers/"):
			parts := strings.Split(path, "/")
			id := parts[len(parts)-2]
			if id == "unreadable" {
				w.WriteHeader(f.inspectStatus)
				_, _ = w.Write([]byte(`{"message":"inspect failed"}`))
				return
			}
			for _, c := range f.containers {
				if c.ID == id || c.Name == "/"+id {
					_ = json.NewEncoder(w).Encode(c)
					return
				}
			}
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"no such container"}`))
		case r.Method == "GET" && strings.Contains(path, "/networks/"):
			id := path[strings.LastIndex(path, "/")+1:]
			for _, n := range f.networks {
				if n.ID == id || n.Name == id {
					_ = json.NewEncoder(w).Encode(n)
					return
				}
			}
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"no such network"}`))
		case r.Method == "POST" && strings.HasSuffix(path, "/networks/create"):
			require.NoError(t, json.NewDecoder(r.Body).Decode(&f.created))
			f.writes = append(f.writes, "create")
			_ = json.NewEncoder(w).Encode(network.CreateResponse{ID: "created"})
		case r.Method == "POST" && strings.HasSuffix(path, "/connect"):
			require.NoError(t, json.NewDecoder(r.Body).Decode(&f.connected))
			f.writes = append(f.writes, "connect")
			w.WriteHeader(200)
		case r.Method == "POST" && strings.HasSuffix(path, "/disconnect"):
			f.writes = append(f.writes, "disconnect")
			w.WriteHeader(200)
		case r.Method == "POST" && strings.HasSuffix(path, "/prune"):
			f.writes = append(f.writes, "prune")
			_ = json.NewEncoder(w).Encode(network.PruneReport{NetworksDeleted: []string{"removed"}})
		case r.Method == "DELETE" && strings.Contains(path, "/networks/"):
			f.writes = append(f.writes, "remove")
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected Docker request %s %s", r.Method, path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.54"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&event.Event{}))
	databaseService := &database.DB{DB: db}
	return NewNetworkService(databaseService, docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli), event.NewEventService(databaseService, nil, nil))
}

var actor = user.Actor{ID: "example-user", Username: "example-user"}

func TestRemoveNetworkProtection(t *testing.T) {
	for _, name := range []string{"bridge", "host", "none", "ingress"} {
		t.Run(name, func(t *testing.T) {
			n := inspectNetwork()
			n.Name = name
			f := &networkDockerFixture{networks: []network.Inspect{n}}
			requireCode(t, f.service(t).RemoveNetwork(t.Context(), n.ID, actor), "builtin-network")
			require.Empty(t, f.writes)
		})
	}
	for _, key := range []string{"com.docker.compose.project", "com.docker.compose.network"} {
		t.Run(key, func(t *testing.T) {
			n := inspectNetwork()
			n.Labels = map[string]string{key: "example"}
			f := &networkDockerFixture{networks: []network.Inspect{n}}
			requireCode(t, f.service(t).RemoveNetwork(t.Context(), n.ID, actor), "compose-managed")
			require.Empty(t, f.writes)
		})
	}
	t.Run("stopped only reference", func(t *testing.T) {
		f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}, containers: []container.InspectResponse{stoppedReference()}}
		requireCode(t, f.service(t).RemoveNetwork(t.Context(), "net1", actor), "in-use")
		require.Empty(t, f.writes)
	})
	t.Run("unused", func(t *testing.T) {
		f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}}
		require.NoError(t, f.service(t).RemoveNetwork(t.Context(), "net1", actor))
		require.Equal(t, []string{"remove"}, f.writes)
	})
	t.Run("reference snapshot failure", func(t *testing.T) {
		f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}, failList: true}
		require.Error(t, f.service(t).RemoveNetwork(t.Context(), "net1", actor))
		require.Empty(t, f.writes)
	})
	t.Run("missing", func(t *testing.T) {
		f := &networkDockerFixture{}
		requireCode(t, f.service(t).RemoveNetwork(t.Context(), "missing", actor), "not-found")
		require.Empty(t, f.writes)
	})
}

func TestConnectServiceIncludesStoppedReservations(t *testing.T) {
	f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}, containers: []container.InspectResponse{disconnectedContainer(), stoppedReference()}}
	svc := f.service(t)
	requireCode(t, svc.ConnectContainer(t.Context(), "net1", networktypes.ConnectContainerRequest{ContainerID: "container1", IPv4Address: "192.0.2.7"}, actor), "invalid-ip")
	require.Empty(t, f.writes)
	requireCode(t, svc.ConnectContainer(t.Context(), "net1", networktypes.ConnectContainerRequest{ContainerID: "stopped"}, actor), "already-connected")
	requireCode(t, svc.ConnectContainer(t.Context(), "missing", networktypes.ConnectContainerRequest{ContainerID: "container1"}, actor), "not-found")
	requireCode(t, svc.ConnectContainer(t.Context(), "net1", networktypes.ConnectContainerRequest{ContainerID: "missing"}, actor), "not-found")
	require.NoError(t, svc.ConnectContainer(t.Context(), "net1", networktypes.ConnectContainerRequest{ContainerID: "container1", IPv4Address: "192.0.2.8"}, actor))
	require.Equal(t, []string{"connect"}, f.writes)
}

func TestDisconnectServiceChecksMembership(t *testing.T) {
	f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}, containers: []container.InspectResponse{disconnectedContainer(), stoppedReference()}}
	svc := f.service(t)
	requireCode(t, svc.DisconnectContainer(t.Context(), "net1", networktypes.DisconnectContainerRequest{ContainerID: "container1"}, actor), "not-connected")
	require.NoError(t, svc.DisconnectContainer(t.Context(), "net1", networktypes.DisconnectContainerRequest{ContainerID: "stopped"}, actor))
	require.Equal(t, []string{"disconnect"}, f.writes)
}

func TestPruneNetworkAuditActor(t *testing.T) {
	f := &networkDockerFixture{}
	svc := f.service(t)
	_, err := svc.PruneNetworks(t.Context(), actor)
	require.NoError(t, err)
	var row event.Event
	require.NoError(t, svc.db.Where("resource_name = ?", "bulk_prune").First(&row).Error)
	require.Equal(t, actor.ID, *row.UserID)
	require.Equal(t, actor.Username, *row.Username)
}

func TestCreateNetworkValidationIsHostScoped(t *testing.T) {
	occupied := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}}
	otherHost := &networkDockerFixture{}
	occupiedService, otherService := occupied.service(t), otherHost.service(t)
	_, err := occupiedService.CreateNetwork(t.Context(), "new-example", options4(), actor)
	requireCode(t, err, "overlap")
	require.Empty(t, occupied.writes)
	_, err = otherService.CreateNetwork(t.Context(), "new-example", options4(), actor)
	require.NoError(t, err, "another host's overlapping pool must not block this daemon")
	require.Equal(t, []string{"create"}, otherHost.writes)
	_, err = occupiedService.CreateNetwork(t.Context(), "example", client.NetworkCreateOptions{}, actor)
	requireCode(t, err, "exists")
}
