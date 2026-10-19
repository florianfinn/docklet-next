// Source: docklet-next/docs/design/runtime-control.md
// Revision: cdfb339023d76259b5de81589b8081255683560f7f129b911cab8db4b23a642a
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Original effective Compose-label consequence regression coverage.

package image_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/dashboard"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	"github.com/moby/moby/api/types/container"
	dockerimage "github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func TestComposeNameImageAndDashboardResponses(t *testing.T) {
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "usage.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.Table("projects").Create(map[string]any{"id": "example-project", "name": "alpha", "path": "/srv/projects/alpha", "status": "stopped", "compose_project_name": "beta_stack"}).Error)
	containers := []container.Summary{{ID: "example-container", Image: "registry.example.com/app:1", ImageID: "sha256:example", State: "running", Labels: map[string]string{"com.docker.compose.project": "beta_stack", "com.docker.compose.service": "web"}}}
	images := []dockerimage.Summary{{ID: "sha256:example", RepoTags: []string{"registry.example.com/app:1"}}}
	var unexpected atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			unexpected.Add(1)
			http.Error(w, "read only", 405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode(containers)
		case strings.HasSuffix(r.URL.Path, "/networks"):
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/images/json"):
			_ = json.NewEncoder(w).Encode(images)
		case strings.HasSuffix(r.URL.Path, "/volumes"):
			_, _ = w.Write([]byte(`{"Volumes":[],"Warnings":[]}`))
		default:
			unexpected.Add(1)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() { server.CloseClientConnections(); server.Close() })
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cli.Close()) })
	dockerSvc := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli)
	imageSvc := image.NewImageService(db, dockerSvc, nil, nil, nil, nil, nil, nil)
	assertUsage := func(t *testing.T, rows []imagetypes.Summary) {
		t.Helper()
		data, err := json.Marshal(rows)
		require.NoError(t, err)
		var wire []map[string]any
		require.NoError(t, json.Unmarshal(data, &wire))
		require.Len(t, wire, 1)
		uses, ok := wire[0]["usedBy"].([]any)
		require.True(t, ok)
		require.Len(t, uses, 1)
		usage, ok := uses[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "project", usage["type"])
		require.Equal(t, "example-project", usage["id"])
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	for _, state := range []string{"cold", "warm"} {
		t.Run(state, func(t *testing.T) {
			rows, _, err := imageSvc.ListImagesPaginated(ctx, pagination.QueryParams{Limit: -1})
			require.NoError(t, err)
			assertUsage(t, rows)
			// A fresh dashboard avoids its own snapshot cache while sharing the image
			// map, proving both consumers under cold/warm image-map cache state.
			dashboardSvc := dashboard.NewDashboardService(db, dockerSvc, nil, nil, imageSvc, nil, nil, nil, nil, nil)
			snapshot, err := dashboardSvc.GetSnapshot(ctx, dashboard.DashboardActionItemsOptions{DebugAllGood: true}, true)
			require.NoError(t, err)
			assertUsage(t, snapshot.Images.Data)
		})
	}
	require.Zero(t, unexpected.Load(), "synthetic inventory must use expected read endpoints only")
}
