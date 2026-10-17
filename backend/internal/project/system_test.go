// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original fake Docker project-list regression test.

package project

import (
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestListProjectsHidesAgentFakeDocker(t *testing.T) {
	db := setupProjectTestDB(t)
	settings, err := newSettingsServiceForTest(t, t.Context(), db)
	require.NoError(t, err)
	directory := t.TempDir()
	require.NoError(t, settings.SetStringSetting(t.Context(), "projectsDirectory", directory))
	records := []Project{
		{ID: "system", Name: "System", Path: filepath.Join(directory, "system"), Status: ProjectStatusRunning},
		{ID: "user", Name: "docklet-agent", Path: filepath.Join(directory, "user"), Status: ProjectStatusRunning},
	}
	require.NoError(t, db.Create(&records).Error)
	server := newProjectRuntimeDockerServer(t, []container.Summary{
		{ID: "agent", Labels: map[string]string{"com.docker.compose.project": "system", "com.docker.compose.service": "agent", "io.docklet.system": "agent"}, State: container.StateRunning},
		{ID: "user", Labels: map[string]string{"com.docker.compose.project": "docklet-agent", "com.docker.compose.service": "web"}, State: container.StateRunning},
	})
	service := NewProjectService(db, settings, nil, nil, &docker.DockerClientService{Client: newTestDockerClient(t, server)}, nil, nil, config.Load(), nil, nil)
	rows, page, err := service.ListProjects(t.Context(), pagination.QueryParams{Limit: 1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "user", rows[0].ID)
	require.EqualValues(t, 1, page.TotalItems)
}
