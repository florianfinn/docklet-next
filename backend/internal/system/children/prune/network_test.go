// Source: review-203-r1/probe_prune_review_test.go
// Revision: 06300929ef8199f8b80f15b3260626ce3e0e6868545ab945df749a176096486b
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Adapted the #203 R1 probe into an asserting scheduled-prune regression; added active and built-in exclusions.

package prune

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	networkservice "github.com/florianfinn/docklet-next/backend/internal/network"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestScheduledPrunePreservesNetworkRemovalSemantics(t *testing.T) {
	nets := []network.Inspect{
		{Network: network.Network{ID: "compose1", Name: "stack_default", Driver: "bridge", Scope: "local", Labels: map[string]string{"com.docker.compose.project": "stack", "com.docker.compose.network": "default"}}},
		{Network: network.Network{ID: "stale1", Name: "stale", Driver: "bridge", Scope: "local"}},
		{Network: network.Network{ID: "plain1", Name: "plain", Driver: "bridge", Scope: "local"}},
	}
	nets = append(nets,
		network.Inspect{Network: network.Network{ID: "builtin", Name: "bridge", Driver: "bridge", Scope: "local"}},
		network.Inspect{Network: network.Network{ID: "active", Name: "active", Driver: "bridge", Scope: "local"}, Containers: map[string]network.EndpointResource{"running1": {Name: "running"}}},
	)
	stopped := container.InspectResponse{ID: "stopped1", State: &container.State{Running: false}, HostConfig: &container.HostConfig{NetworkMode: "stale"}, NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{"stale": {NetworkID: "stale1"}}}}
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/networks") && r.Method == "GET":
			var list []network.Network
			for _, n := range nets {
				list = append(list, n.Network)
			}
			_ = json.NewEncoder(w).Encode(list)
		case strings.HasSuffix(p, "/containers/json"):
			_, _ = w.Write([]byte(`[{"Id":"stopped1","State":"exited"}]`))
		case strings.Contains(p, "/containers/stopped1/json"):
			_ = json.NewEncoder(w).Encode(stopped)
		case strings.HasSuffix(p, "/system/df"):
			_, _ = w.Write([]byte(`{}`))
		case strings.Contains(p, "/networks/"):
			id := p[strings.LastIndex(p, "/")+1:]
			for _, n := range nets {
				if n.ID == id || n.Name == id {
					if r.Method == "DELETE" {
						deleted = append(deleted, n.Name)
						w.WriteHeader(204)
						return
					}
					_ = json.NewEncoder(w).Encode(n)
					return
				}
			}
			http.Error(w, `{"message":"missing"}`, 404)
		default:
			t.Errorf("unexpected Docker request: %s %s", r.Method, p)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.52"))
	require.NoError(t, err)
	defer cli.Close()
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&event.Event{}))
	dbs := &database.DB{DB: db}
	dockerService := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli)
	netSvc := networkservice.NewNetworkService(dbs, dockerService, event.NewEventService(dbs, nil, nil))
	svc := NewService(time.UTC, func(context.Context) (*client.Client, error) { return cli, nil }, nil, nil, nil, netSvc)
	result, started, err := svc.PruneScheduled(t.Context(), "0", system.PruneAllRequest{Networks: &system.PruneNetworksOptions{Mode: system.PruneNetworkModeUnused}})
	require.NoError(t, err)
	require.True(t, started)
	require.NotNil(t, result)
	require.True(t, result.Success, "%v", result.Errors)
	require.Empty(t, result.Errors)
	require.ElementsMatch(t, []string{"stack_default", "stale", "plain"}, deleted)
	require.ElementsMatch(t, []string{"compose1", "stale1", "plain1"}, result.NetworksDeleted)
}
