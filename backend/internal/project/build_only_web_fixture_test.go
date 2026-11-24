// Source: docklet-next/issues/600
// Revision: 393e645a79236449c35cb8e1a4827c83bbf45da0
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original Go-to-web fixture of a build-only, never-created project from the hub's list and detail producers.

package project

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	dto "github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
)

// The web fixture for #600 carries the hub's own list and get-project bytes for a stopped project with a build-only,
// never-created service next to an ordinary stopped project. UPDATE_WEB_FIXTURES=1 regenerates it.
func TestBuildOnlyProjectWebFixture(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	runtimeIdentityProject(t, svc, root, "project-build", "builder", "services:\n  app:\n    build: ./app\n  cache:\n    image: registry.example.com/cache:7\n")
	runtimeIdentityProject(t, svc, root, "project-plain", "plain", "services:\n  web:\n    image: registry.example.com/web:1.2.3\n")
	server := newProjectRuntimeDockerServer(t, nil)
	svc.dockerService = &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	svc.initChildren(nil, nil)

	rows, _, err := svc.ListProjects(t.Context(), pagination.QueryParams{Limit: -1, Sort: "name"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	var build dto.Details
	for _, row := range rows {
		if row.ID == "project-build" {
			build = row
		}
	}
	// Producer facts the web relies on: the build-only service has an empty image and is not created.
	found := false
	for _, s := range build.RuntimeServices {
		if s.Name == "app" {
			found = true
			require.Empty(t, s.Image)
			require.Equal(t, dto.ServiceNotCreated, s.Status)
			require.Empty(t, s.ContainerID)
		}
	}
	require.True(t, found, "build-only service must be listed")
	detail, err := svc.GetProjectDetails(t.Context(), "project-build", dto.DetailsOptions{})
	require.NoError(t, err)
	for i := range rows {
		rows[i].CreatedAt, rows[i].UpdatedAt, rows[i].Path = "", "", "/srv/projects/"+rows[i].Name
	}
	detail.CreatedAt, detail.UpdatedAt, detail.Path = "", "", "/srv/projects/"+detail.Name
	// A struct keeps the key order fixed; encoding/json/v2 does not order map keys.
	writeBuildOnlyWebFixture(t, struct {
		Projects []dto.Details `json:"projects"`
		Detail   dto.Details   `json:"detail"`
	}{rows, detail})
}

func writeBuildOnlyWebFixture(t *testing.T, value any) {
	t.Helper()
	// The hub's API options (backend/api/json_serializer.go).
	body, err := json.Marshal(value, jsonv1.FormatDurationAsNano(true), jsontext.WithIndent("  "))
	require.NoError(t, err)
	body = append(body, '\n')
	path := "../../../web/src/data/http/testdata/build-only-project.json"
	if os.Getenv("UPDATE_WEB_FIXTURES") == "1" {
		require.NoError(t, os.WriteFile(path, body, 0644))
	}
	expected, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(expected), string(body), "web fixture differs from producer bytes; regenerate with UPDATE_WEB_FIXTURES=1")
}
