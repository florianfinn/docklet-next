// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package project

import (
	"context"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/access"
	dto "github.com/florianfinn/docklet-next/types/project"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestLocalResourceMembershipReadsRuntimeProducer(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "project-uuid", "example", identityServices)
	configureProjectRuntimeDocker(t, []container.Summary{{ID: "canonical", Names: []string{"/example-app-1"}, Image: "registry.example.com/app:1", State: container.StateRunning, Labels: map[string]string{"com.docker.compose.project": "example", "com.docker.compose.service": "app", "com.docker.compose.config-hash": "hash"}}})
	p := access.Principal{ID: "viewer", Assignments: []userDTO.AppAssignment{{EnvironmentID: "0", ProjectID: row.ID, Level: "restricted"}}}
	ctx := access.WithController(t.Context(), &access.Controller{Runtime: func(ctx context.Context, env, id string) ([]access.Container, error) {
		details, err := svc.GetProjectDetails(ctx, id, dto.DetailsOptions{IncludeRuntimeServices: true})
		if err != nil {
			return nil, err
		}
		out := []access.Container{}
		for _, v := range details.RuntimeServices {
			out = append(out, access.Container{ID: v.ContainerID, Names: []string{v.ContainerName}})
		}
		return out, nil
	}})
	for _, id := range []string{"canonical", "example-app-1"} {
		require.Equal(t, access.Allow, access.Decide(ctx, p, "get-container", "0", access.Params{ContainerID: id}))
	}
	require.Equal(t, access.NotFound, access.Decide(ctx, p, "get-container", "0", access.Params{ContainerID: "standalone"}))
	// UUID grants survive a rename but never transfer to another same-name row.
	row.Name = "renamed"
	require.NoError(t, svc.db.Save(row).Error)
	require.Equal(t, access.Allow, access.Decide(ctx, p, "get-project", "0", access.Params{ProjectID: row.ID}))
	require.Equal(t, access.NotFound, access.Decide(ctx, p, "get-project", "0", access.Params{ProjectID: "same-name-recreated-uuid"}))
	// AUTO-1: a producer that hides a Docker read error yields an empty view and denies.
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:1")
	require.Equal(t, access.NotFound, access.Decide(ctx, p, "get-container", "0", access.Params{ContainerID: "canonical"}))
	require.NoError(t, svc.db.Delete(row).Error)
	require.Equal(t, access.Unavailable, access.Decide(ctx, p, "get-container", "0", access.Params{ContainerID: "canonical"}))
}
