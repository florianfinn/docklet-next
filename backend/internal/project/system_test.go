// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original fake Docker project-list regression test; hub role; pending updates and status counts skip Docklet projects.

package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestListProjectsHidesAgentFakeDocker(t *testing.T) {
	testListProjectsHidesSystemRole(t, "agent")
}

// B6: the hub's own Compose project, e.g. inside its own projects directory, is not a stack.
func TestListProjectsHidesHubFakeDocker(t *testing.T) {
	testListProjectsHidesSystemRole(t, "hub")
}

func testListProjectsHidesSystemRole(t *testing.T, role string) {
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
		{ID: "agent", Labels: map[string]string{"com.docker.compose.project": "system", "com.docker.compose.service": role, "io.docklet.system": role}, State: container.StateRunning},
		{ID: "user", Labels: map[string]string{"com.docker.compose.project": "docklet-agent", "com.docker.compose.service": "web"}, State: container.StateRunning},
	})
	service := NewProjectService(db, settings, nil, nil, &docker.DockerClientService{Client: newTestDockerClient(t, server)}, nil, nil, config.Load(), nil, nil)
	rows, page, err := service.ListProjects(t.Context(), pagination.QueryParams{Limit: 1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "user", rows[0].ID)
	require.EqualValues(t, 1, page.TotalItems)
}

// Docklet's own projects never appear as pending-update rows or in the pending-update count.
func TestPendingUpdatesSkipDockletProjectsFakeDocker(t *testing.T) {
	for _, role := range []string{"hub", "agent", "workload"} {
		for _, tracked := range []bool{false, true} {
			name := role + "/untracked"
			if tracked {
				name = role + "/tracked"
			}
			t.Run(name, func(t *testing.T) {
				db := setupProjectTestDB(t)
				settings, err := newSettingsServiceForTest(t, t.Context(), db)
				require.NoError(t, err)
				dir := t.TempDir()
				require.NoError(t, settings.SetStringSetting(t.Context(), "projectsDirectory", dir))
				if tracked {
					require.NoError(t, os.MkdirAll(filepath.Join(dir, "docklet"), 0o755))
					require.NoError(t, os.WriteFile(filepath.Join(dir, "docklet", "compose.yaml"), []byte("name: docklet\nservices:\n  hub:\n    image: example:1.0.0\n"), 0o600))
					require.NoError(t, db.Create(&Project{ID: "docklet", Name: "docklet", Path: filepath.Join(dir, "docklet"), Status: ProjectStatusRunning, ImageRefsJSON: `["example:1.0.0"]`}).Error)
				}
				require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{
					ID: "sha256:hub", Repository: "docker.io/library/example", Tag: "1.0.0", Platform: new("native-v1:classic:linux/amd64"),
					CurrentDigest: new("sha256:" + strings.Repeat("a", 64)), LatestDigest: new("sha256:" + strings.Repeat("b", 64)),
					HasUpdate: true, UpdateType: imageupdate.UpdateTypeDigest, CurrentVersion: "1.0.0", CheckTime: time.Now().UTC(),
				}).Error)
				rows := []container.Summary{{ID: "hub", Names: []string{"/docklet-hub"}, Image: "example:1.0.0", ImageID: "sha256:hub", State: container.StateRunning,
					Labels: map[string]string{"io.docklet.system": role, "com.docker.compose.project": "docklet", "com.docker.compose.service": "hub"}},
					// An unlabelled sibling service with its own update belongs to the same hidden project.
					{ID: "sidecar", Names: []string{"/docklet-sidecar"}, Image: "example:1.0.0", ImageID: "sha256:hub", State: container.StateRunning,
						Labels: map[string]string{"com.docker.compose.project": "docklet", "com.docker.compose.service": "sidecar"}}}
				images := image.NewImageService(db, nil, nil, nil, nil, nil, nil, nil)
				service := NewProjectService(db, settings, nil, images, &docker.DockerClientService{Client: newTestDockerClient(t, newProjectRuntimeDockerServer(t, rows))}, nil, nil, config.Load(), nil, nil)
				pending, _, err := service.ListProjects(t.Context(), pagination.QueryParams{Limit: 100, Filters: map[string]string{"updates": "has_update"}})
				require.NoError(t, err)
				count, err := service.CountProjectsWithPendingUpdates(t.Context(), rows)
				require.NoError(t, err)
				if role == "workload" {
					require.Len(t, pending, 1, "an ordinary stack with an update stays listed")
					require.Equal(t, 1, count)
					return
				}
				require.Empty(t, pending)
				require.Zero(t, count)
			})
		}
	}
}

// Status counts skip Docklet's own stacks like the stack list does.
func TestProjectStatusCountsSkipDockletProjectsFakeDocker(t *testing.T) {
	for _, role := range []string{"hub", "agent", "workload"} {
		t.Run(role, func(t *testing.T) {
			db := setupProjectTestDB(t)
			settings, err := newSettingsServiceForTest(t, t.Context(), db)
			require.NoError(t, err)
			directory := t.TempDir()
			require.NoError(t, settings.SetStringSetting(t.Context(), "projectsDirectory", directory))
			require.NoError(t, db.Create(&[]Project{
				{ID: "docklet", Name: "docklet", Path: filepath.Join(directory, "docklet"), Status: ProjectStatusRunning},
				{ID: "user", Name: "user", Path: filepath.Join(directory, "user"), Status: ProjectStatusRunning},
			}).Error)
			server := newProjectRuntimeDockerServer(t, []container.Summary{
				{ID: "docklet", Labels: map[string]string{"com.docker.compose.project": "docklet", "com.docker.compose.service": "hub", "io.docklet.system": role}, State: container.StateRunning},
				{ID: "user", Labels: map[string]string{"com.docker.compose.project": "user", "com.docker.compose.service": "web"}, State: container.StateRunning},
			})
			service := NewProjectService(db, settings, nil, nil, &docker.DockerClientService{Client: newTestDockerClient(t, server)}, nil, nil, config.Load(), nil, nil)
			counts, err := service.GetProjectStatusCounts(t.Context())
			require.NoError(t, err)
			want := 1
			if role == "workload" {
				want = 2
			}
			require.Equal(t, want, counts.TotalProjects)
			require.Equal(t, want, counts.RunningProjects)
		})
	}
}

// F-R9-1: a Docklet install directory stored by an earlier scan is removed on the next sync, so its
// private .env can no longer block ordinary stacks; its files stay in place.
func TestSyncRemovesStoredDockletInstallDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file modes do not restrict root")
	}
	db := setupProjectTestDB(t)
	settings, err := newSettingsServiceForTest(t, t.Context(), db)
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, settings.SetStringSetting(t.Context(), "projectsDirectory", dir))
	install := filepath.Join(dir, "docklet")
	require.NoError(t, os.MkdirAll(install, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(install, "compose.yaml"), []byte("name: docklet\nservices:\n  hub:\n    image: example:1.0.0\n    labels:\n      io.docklet.system: hub\n"), 0o644))
	env := filepath.Join(install, ".env")
	require.NoError(t, os.WriteFile(env, []byte("APP_URL=https://docklet.example.com\n"), 0))
	t.Cleanup(func() { _ = os.Chmod(env, 0o600) })
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "user"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "user", "compose.yaml"), []byte("name: user\nservices:\n  web:\n    image: example:1.0.0\n"), 0o644))
	require.NoError(t, db.Create(&[]Project{
		{ID: "docklet", Name: "docklet", Path: install, Status: ProjectStatusRunning},
		{ID: "user", Name: "user", Path: filepath.Join(dir, "user"), Status: ProjectStatusStopped},
	}).Error)
	server := newProjectRuntimeDockerServer(t, nil)
	service := NewProjectService(db, settings, nil, nil, &docker.DockerClientService{Client: newTestDockerClient(t, server)}, nil, nil, config.Load(), nil, nil)
	_, _, err = service.ResolveRuntimeProjectIdentity(t.Context(), "user", "")
	require.Error(t, err, "precondition: the stored, unreadable install row blocks")

	require.NoError(t, service.SyncProjectsFromFileSystem(t.Context()))
	var remaining []string
	require.NoError(t, db.Model(&Project{}).Order("id").Pluck("id", &remaining).Error)
	require.Equal(t, []string{"user"}, remaining)
	require.FileExists(t, filepath.Join(install, "compose.yaml"), "files stay in place")
	ref, ok, err := service.ResolveRuntimeProjectIdentity(t.Context(), "user", "")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "user", ref.Name)
}
