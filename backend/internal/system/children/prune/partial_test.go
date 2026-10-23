// Source: docklet-next/docs/design/storage-workspace.md
// Revision: ce3c4ea4af5b0bc1cdb2bb287d30281bbe20f0a8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original R1 regression coverage for issue #179.
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
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	volumeservice "github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSystemPruneKeepsPartialVolumeRemovals(t *testing.T) {
	volumes := []volume.Volume{{Name: "good", Driver: "local", CreatedAt: "2026-10-22T00:00:00Z"}, {Name: "bad", Driver: "local", CreatedAt: "2026-10-22T00:00:00Z"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/volumes"):
			_ = json.NewEncoder(w).Encode(volume.ListResponse{Volumes: volumes})
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/system/df"):
			_, _ = w.Write([]byte(`{"Volumes":[]}`))
		case strings.Contains(r.URL.Path, "/volumes/"):
			name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			if r.Method == "DELETE" {
				if name == "bad" {
					http.Error(w, `{"message":"removal failed"}`, 500)
				} else {
					w.WriteHeader(204)
				}
				return
			}
			for _, v := range volumes {
				if v.Name == name {
					_ = json.NewEncoder(w).Encode(v)
					return
				}
			}
			http.Error(w, `{"message":"missing"}`, 404)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.52"))
	require.NoError(t, err)
	defer cli.Close()
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&policies.VolumeBackupPolicy{}))
	dockerService := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli)
	svc := NewService(time.UTC, func(context.Context) (*client.Client, error) { return cli, nil }, nil, nil, volumeservice.NewVolumeService(&database.DB{DB: db}, dockerService, nil, nil, nil, nil, nil, nil, nil, nil), nil)
	result, started, err := svc.PruneAll(t.Context(), "0", system.PruneAllRequest{Volumes: &system.PruneVolumesOptions{Mode: system.PruneVolumeModeAll}})
	require.NoError(t, err)
	require.True(t, started)
	require.False(t, result.Success)
	require.Contains(t, strings.Join(result.Errors, " "), "Volume pruning failed")
	require.Equal(t, []string{"good"}, result.VolumesDeleted, "manual/scheduled execution must report successful removals even when another candidate fails")
}
