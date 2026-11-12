// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original Stop All successful-subset intent integration test.

package system

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/runtimeintent"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHB5StopAllPersistsOnlySuccessfulMembers(t *testing.T) {
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "stop.db")), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	setting, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	defer setting.Stop(context.Background())
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": "success", "Names": []string{"/success"}, "State": "running"}, map[string]any{"Id": "failed", "Names": []string{"/failed"}, "State": "running"}})
		case strings.HasSuffix(r.URL.Path, "/json"):
			id := "success"
			if strings.Contains(r.URL.Path, "/failed/") {
				id = "failed"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": "/" + id, "Config": map[string]any{"Labels": map[string]string{}}, "State": map[string]any{"Status": "running"}})
		case strings.HasSuffix(r.URL.Path, "/stop"):
			if strings.Contains(r.URL.Path, "/failed/") {
				http.Error(w, "synthetic failure", 500)
			} else {
				w.WriteHeader(204)
			}
		default:
			w.WriteHeader(200)
		}
	}))
	defer daemon.Close()
	cli, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.41"), client.WithHTTPClient(daemon.Client()))
	require.NoError(t, err)
	defer cli.Close()
	cfg := &config.Config{Environment: "test"}
	dockerService := docker.NewDockerClientService(t.Context(), db, cfg, setting).WithClient(cli)
	events := event.NewEventService(db, cfg, daemon.Client())
	projects := project.NewProjectService(db, setting, events, nil, dockerService, nil, nil, cfg, nil, nil)
	containers := container.NewContainerService(events, dockerService, nil, setting, projects)
	service := &SystemService{db: db, dockerService: dockerService, containerService: containers, activityService: activity.NewActivityService(db, setting), settingsService: setting}
	result, err := service.StopAllContainers(t.Context(), "0")
	require.NoError(t, err)
	require.False(t, result.Success)
	var rows []runtimeintent.Row
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "success", rows[0].ContainerID)
}
