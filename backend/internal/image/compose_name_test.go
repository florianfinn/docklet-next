// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Original effective Compose-label consequence regression coverage.

package image

import (
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/moby/moby/api/types/container"
	dockerimage "github.com/moby/moby/api/types/image"
	"github.com/stretchr/testify/require"
)

func TestComposeNameImageUsageColdAndWarm(t *testing.T) {
	for _, order := range [][]string{{"alias", "owner"}, {"owner", "alias"}} {
		t.Run(order[0], func(t *testing.T) {
			db, err := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "images.db")), database.MigrationOptions{})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			for _, kind := range order {
				id, name, observed := "a-alias", "Beta_Stack", any(nil)
				if kind == "owner" {
					id, name, observed = "z-owner", "alpha", "beta_stack"
				}
				require.NoError(t, db.Table("projects").Create(map[string]any{"id": id, "name": name, "path": "/srv/projects/" + kind, "status": "stopped", "compose_project_name": observed}).Error)
			}
			for _, entry := range []struct {
				id, name string
				metadata any
			}{
				{"legacy-nil", "__Legacy Custom!", nil}, {"legacy-empty", "Other Custom!", ""},
			} {
				require.NoError(t, db.Table("projects").Create(map[string]any{"id": entry.id, "name": entry.name, "path": "/srv/projects/" + entry.id, "status": "stopped", "compose_project_name": entry.metadata}).Error)
			}
			svc := NewImageService(db, nil, nil, nil, nil, nil, nil, nil)
			containers := []container.Summary{
				{ID: "current", ImageID: "sha256:example", Labels: map[string]string{"com.docker.compose.project": "beta_stack"}},
				{ID: "legacy", Labels: map[string]string{"com.docker.compose.project": "legacycustom"}},
				{ID: "empty", Labels: map[string]string{"com.docker.compose.project": "othercustom"}},
				{ID: "stale", Labels: map[string]string{"com.docker.compose.project": "alpha"}},
			}
			for _, state := range []string{"cold", "warm"} {
				ids := svc.BuildProjectIDMap(t.Context(), containers)
				require.Equal(t, "z-owner", ids["beta_stack"], state)
				require.Equal(t, "legacy-nil", ids["legacycustom"], state)
				require.Equal(t, "legacy-empty", ids["othercustom"], state)
				require.NotContains(t, ids, "alpha", state)
				usage := BuildVolumeUsageMap(containers, ids)
				dto := MapDockerImagesToDTOs([]dockerimage.Summary{{ID: "sha256:example", RepoTags: []string{"registry.example.com/app:1"}}}, containers, usage, nil, nil)
				require.Len(t, dto, 1)
				require.Len(t, dto[0].UsedBy, 1)
				require.Equal(t, "project", dto[0].UsedBy[0].Type)
				require.Equal(t, "z-owner", dto[0].UsedBy[0].ID, "image-list/dashboard shared DTO path: "+state)
			}
			// A new snapshot observes removed metadata; the warm instance retains its
			// existing bounded TTL behavior, not fresh runtime ownership authority.
			require.NoError(t, db.Table("projects").Where("id = ?", "z-owner").Update("compose_project_name", nil).Error)
			fresh := NewImageService(db, nil, nil, nil, nil, nil, nil, nil)
			ids := fresh.BuildProjectIDMap(t.Context(), containers)
			require.Equal(t, "z-owner", ids["alpha"])
			require.Equal(t, "a-alias", ids["beta_stack"])
		})
	}
}
