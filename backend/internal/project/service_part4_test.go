// Source: getarcaneapp/arcane/backend/internal/project/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture.

package project

import (
	"net/netip"

	"os"
	"path/filepath"

	"testing"
	"time"

	"github.com/docker/compose/v5/pkg/api"

	imagetypes "github.com/florianfinn/docklet-next/types/image"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"

	"github.com/moby/moby/api/types/container"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.getarcane.app/updater/labels"

	"github.com/florianfinn/docklet-next/backend/internal/config"

	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"

	"github.com/florianfinn/docklet-next/backend/internal/project/children/listing"

	"github.com/florianfinn/docklet-next/backend/pkg/pagination"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/iconcatalog"
)

func TestProjectService_UpdateProject_PreservesExistingOverrideName(t *testing.T) {
	ctx := t.Context()
	svc, project, projectPath := newProjectServiceForOverrideTest(t, "override-preserve")

	// An existing docker-compose.override.yml must keep its name on edit rather
	// than being rewritten to the default compose.override.yaml.
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "docker-compose.override.yml"), []byte("services:\n  app:\n    image: alpine:3\n"), 0o600))

	override := "services:\n  app:\n    image: busybox:latest\n"
	_, err := svc.UpdateProject(ctx, project.ID, nil, nil, nil, new(override), usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})
	require.NoError(t, err)

	overrideBytes, readErr := os.ReadFile(filepath.Join(projectPath, "docker-compose.override.yml"))
	require.NoError(t, readErr)
	assert.Equal(t, override, string(overrideBytes))
	assert.NoFileExists(t, filepath.Join(projectPath, "compose.override.yaml"))
}

func TestProjectService_UpdateProject_DeletesOverrideOnBlank(t *testing.T) {
	ctx := t.Context()
	svc, project, projectPath := newProjectServiceForOverrideTest(t, "override-delete")

	overridePath := filepath.Join(projectPath, "compose.override.yaml")
	require.NoError(t, os.WriteFile(overridePath, []byte("services:\n  app:\n    image: busybox:latest\n"), 0o600))

	// A non-nil blank override deletes the file so the deploy stops merging it.
	_, err := svc.UpdateProject(ctx, project.ID, nil, nil, nil, new(""), usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})
	require.NoError(t, err)

	assert.NoFileExists(t, overridePath)
}

func TestProjectService_UpdateProject_MergedValidationFailureLeavesDiskUnchanged(t *testing.T) {
	ctx := t.Context()
	baseCompose := "services:\n  app:\n    image: nginx:alpine\n"
	svc, project, projectPath := newProjectServiceForOverrideTest(t, "override-invalid")

	// A new base compose plus a malformed override must fail validation, and the
	// backup/restore must leave both the base and the override untouched on disk.
	newCompose := "services:\n  app:\n    image: nginx:1.27\n"
	badOverride := "services:\n  app:\n    image: \"unterminated\n"
	_, err := svc.UpdateProject(ctx, project.ID, nil, new(newCompose), nil, new(badOverride), usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})
	require.Error(t, err)

	baseBytes, readErr := os.ReadFile(filepath.Join(projectPath, "compose.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, baseCompose, string(baseBytes))
	assert.NoFileExists(t, filepath.Join(projectPath, "compose.override.yaml"))
}

func TestProjectService_UpdateProject_UsesProvidedEnvContentDuringComposeValidation(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	dirName := "env-updated"
	projectPath := filepath.Join(projectsDir, dirName)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))

	project := &Project{
		ID:      "proj-new-env-file",
		Name:    "env-updated",
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	compose := `services:
  app:
    image: nginx:alpine
    env_file:
      - .env
`
	env := "FOO=updated\n"

	updated, err := svc.UpdateProject(ctx, project.ID, nil, new(compose), new(env), nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.NoError(t, err)
	require.NotNil(t, updated)

	envBytes, readErr := os.ReadFile(filepath.Join(projectPath, ".env"))
	require.NoError(t, readErr)
	assert.Equal(t, env, string(envBytes))
}

func TestProjectService_UpdateProject_ReturnsEnvParseErrorDuringComposeValidation(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	dirName := "env-invalid"
	projectPath := filepath.Join(projectsDir, dirName)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))

	project := &Project{
		ID:      "proj-invalid-env-file",
		Name:    "env-invalid",
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	compose := `services:
  app:
    image: nginx:alpine
    environment:
      - REQUIRED=${REQUIRED}
`
	env := "BROKEN=${UNTERMINATED\n"

	updated, err := svc.UpdateProject(ctx, project.ID, nil, new(compose), new(env), nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.Error(t, err)
	assert.Nil(t, updated)
	assert.Contains(t, err.Error(), "invalid compose file: parse provided env content")
	assert.Contains(t, err.Error(), "parse env")
}

func TestProjectService_GetProjectDetails_ReturnsEffectiveEnvContent(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	dirName := "details-override"
	projectPath := filepath.Join(projectsDir, dirName)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte("services:\n  app:\n    image: nginx:alpine\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, ".env"), []byte("BASE=git\nTOKEN=secret\n"), 0o600))

	project := &Project{
		ID:      "proj-details-override",
		Name:    "details-override",
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	details, err := svc.GetProjectDetails(ctx, project.ID, projecttypes.AllDetails())
	require.NoError(t, err)
	assert.Equal(t, "BASE=git\nTOKEN=secret\n", details.EnvContent)
}

func TestBuildProjectUpdateInfoSummary(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name        string
		imageRefs   []string
		updates     map[string]*imagetypes.UpdateInfo
		wantStatus  string
		wantCount   int
		wantChecked int
		wantErrors  int
		wantUpdates int
		wantError   string
	}{
		{
			name:        "unknown when no checks exist",
			imageRefs:   []string{"nginx:latest"},
			updates:     nil,
			wantStatus:  "unknown",
			wantCount:   1,
			wantChecked: 0,
			wantErrors:  0,
			wantUpdates: 0,
		},
		{
			name:      "has update when any image has update",
			imageRefs: []string{"nginx:latest", "redis:7"},
			updates: map[string]*imagetypes.UpdateInfo{
				"nginx:latest": {HasUpdate: true, CheckTime: now},
				"redis:7":      {HasUpdate: false, CheckTime: now.Add(-time.Minute)},
			},
			wantStatus:  "has_update",
			wantCount:   2,
			wantChecked: 2,
			wantErrors:  0,
			wantUpdates: 1,
		},
		{
			name:      "error when no updates but a check failed",
			imageRefs: []string{"nginx:latest"},
			updates: map[string]*imagetypes.UpdateInfo{
				"nginx:latest": {HasUpdate: false, CheckTime: now, Error: "rate limited"},
			},
			wantStatus:  "error",
			wantCount:   1,
			wantChecked: 1,
			wantErrors:  1,
			wantUpdates: 0,
			wantError:   "rate limited",
		},
		{
			name:      "up to date when all images checked without updates",
			imageRefs: []string{"nginx:latest", "redis:7"},
			updates: map[string]*imagetypes.UpdateInfo{
				"nginx:latest": {HasUpdate: false, CheckTime: now},
				"redis:7":      {HasUpdate: false, CheckTime: now.Add(-time.Minute)},
			},
			wantStatus:  "up_to_date",
			wantCount:   2,
			wantChecked: 2,
			wantErrors:  0,
			wantUpdates: 0,
		},
		{
			name:      "not pulled when an image is missing locally and nothing else needs attention",
			imageRefs: []string{"nginx:latest", "redis:7"},
			updates: map[string]*imagetypes.UpdateInfo{
				"nginx:latest": {HasUpdate: false, UpdateType: imageupdate.UpdateTypeNotPulled, CheckTime: now},
				"redis:7":      {HasUpdate: false, CheckTime: now.Add(-time.Minute)},
			},
			wantStatus:  "not_pulled",
			wantCount:   2,
			wantChecked: 2,
			wantErrors:  0,
			wantUpdates: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := buildUpdateInfoSummary(tt.imageRefs, tt.updates)
			require.NotNil(t, summary)
			assert.Equal(t, tt.wantStatus, summary.Status)
			assert.Equal(t, tt.wantCount, summary.ImageCount)
			assert.Equal(t, tt.wantChecked, summary.CheckedImageCount)
			assert.Equal(t, tt.wantErrors, summary.ErrorCount)
			assert.Equal(t, tt.wantUpdates, summary.ImagesWithUpdates)
			assert.Equal(t, tt.wantUpdates > 0, summary.HasUpdate)
			assert.Equal(t, tt.imageRefs, summary.ImageRefs)
			if tt.wantError != "" {
				require.NotNil(t, summary.ErrorMessage)
				assert.Equal(t, tt.wantError, *summary.ErrorMessage)
			} else {
				assert.Nil(t, summary.ErrorMessage)
			}
			if tt.wantUpdates > 0 {
				assert.Equal(t, []string{tt.imageRefs[0]}, summary.UpdatedImageRefs)
			} else {
				assert.Empty(t, summary.UpdatedImageRefs)
			}
		})
	}
}

func TestProjectService_GetProjectDetails_IncludesUpdateInfo(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	imageService := image.NewImageService(db, nil, nil, nil, nil, nil, nil, nil)
	svc := NewProjectService(db, settingsService, nil, imageService, nil, nil, nil, config.Load(), nil, nil)

	projectPath := createComposeProjectDir(t, projectsDir, "updates-demo")
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte("services:\n  app:\n    image: nginx:latest\n"), 0o644))

	projectRecord := &Project{
		ID:      "proj-update-info",
		Name:    "updates-demo",
		DirName: new("updates-demo"),
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(projectRecord).Error)

	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("native-v1:classic:linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:update-demo",
		Repository:     "docker.io/library/nginx",
		Tag:            "latest",
		HasUpdate:      true,
		UpdateType:     "digest",
		CurrentVersion: "latest",
		CheckTime:      time.Now().UTC(),
	}).Error)

	details, err := svc.GetProjectDetails(ctx, projectRecord.ID, projecttypes.AllDetails())
	require.NoError(t, err)
	require.NotNil(t, details.UpdateInfo)
	assert.Equal(t, "has_update", details.UpdateInfo.Status)
	assert.True(t, details.UpdateInfo.HasUpdate)
	assert.Equal(t, 1, details.UpdateInfo.ImageCount)
	assert.Equal(t, 1, details.UpdateInfo.CheckedImageCount)
	assert.Equal(t, 1, details.UpdateInfo.ImagesWithUpdates)
	assert.Equal(t, []string{"nginx:latest"}, details.UpdateInfo.ImageRefs)
	assert.Equal(t, []string{"nginx:latest"}, details.UpdateInfo.UpdatedImageRefs)
}

func TestProjectService_GetProjectDetails_RefreshesRuntimeStatusWithoutRuntimeServices(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	projectPath := createComposeProjectDir(t, projectsDir, "projectA")
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte("services:\n  server:\n    image: nginx:alpine\n  worker:\n    image: busybox:latest\n"), 0o644))

	server := newProjectRuntimeDockerServer(t, []container.Summary{
		{
			ID:     "server-container",
			Names:  []string{"/projecta-server-1"},
			Image:  "nginx:alpine",
			State:  container.StateRunning,
			Status: "Up 30 seconds (healthy)",
			Ports: []container.PortSummary{
				{
					IP:          netip.MustParseAddr("0.0.0.0"),
					PrivatePort: 80,
					PublicPort:  8080,
					Type:        "tcp",
				},
			},
			Labels: map[string]string{
				api.ProjectLabel:    "projecta",
				api.ServiceLabel:    "server",
				api.ConfigHashLabel: "server-hash",
				api.WorkingDirLabel: "/host/path/projects/projectA",
			},
		},
		{
			ID:     "worker-container",
			Names:  []string{"/projecta-worker-1"},
			Image:  "busybox:latest",
			State:  container.StateRunning,
			Status: "Up 30 seconds",
			Labels: map[string]string{
				api.ProjectLabel:    "projecta",
				api.ServiceLabel:    "worker",
				api.ConfigHashLabel: "worker-hash",
				api.WorkingDirLabel: "/host/path/projects/projectA",
			},
		},
	})
	t.Setenv("DOCKER_HOST", dockerHostFromProjectRuntimeServerURL(t, server.URL))

	projectRecord := &Project{
		ID:           "proj-runtime-refresh",
		Name:         "projectA",
		DirName:      new("projectA"),
		Path:         projectPath,
		Status:       ProjectStatusStopped,
		ServiceCount: 2,
		RunningCount: 0,
	}
	require.NoError(t, db.Create(projectRecord).Error)

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)

	details, err := svc.GetProjectDetails(ctx, projectRecord.ID, projecttypes.DetailsOptions{})
	require.NoError(t, err)
	assert.Equal(t, string(ProjectStatusRunning), details.Status)
	assert.Equal(t, 2, details.ServiceCount)
	assert.Equal(t, 2, details.RunningCount)
	assert.Empty(t, details.RuntimeServices)
}

func TestProjectService_GetProjectDetails_PopulatesRuntimeServicesFromComposePs(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	projectPath := createComposeProjectDir(t, projectsDir, "projectA")
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte("services:\n  server:\n    image: nginx:alpine\n"), 0o644))

	server := newProjectRuntimeDockerServer(t, []container.Summary{
		{
			ID:     "server-container",
			Names:  []string{"/projecta-server-1"},
			Image:  "nginx:alpine",
			State:  container.StateRunning,
			Status: "Up 30 seconds",
			Labels: map[string]string{
				api.ProjectLabel:    "projecta",
				api.ServiceLabel:    "server",
				api.ConfigHashLabel: "server-hash",
				api.WorkingDirLabel: "/host/path/projects/projectA",
			},
		},
	})
	t.Setenv("DOCKER_HOST", dockerHostFromProjectRuntimeServerURL(t, server.URL))

	projectRecord := &Project{
		ID:           "proj-runtime-services",
		Name:         "projectA",
		DirName:      new("projectA"),
		Path:         projectPath,
		Status:       ProjectStatusStopped,
		ServiceCount: 1,
		RunningCount: 0,
	}
	require.NoError(t, db.Create(projectRecord).Error)

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)

	details, err := svc.GetProjectDetails(ctx, projectRecord.ID, projecttypes.DetailsOptions{IncludeRuntimeServices: true})
	require.NoError(t, err)
	require.Len(t, details.RuntimeServices, 1)
	assert.Equal(t, string(ProjectStatusRunning), details.Status)
	assert.Equal(t, "server", details.RuntimeServices[0].Name)
	assert.Equal(t, "running", details.RuntimeServices[0].Status)
	assert.Equal(t, "server-container", details.RuntimeServices[0].ContainerID)
	assert.Equal(t, "projecta-server-1", details.RuntimeServices[0].ContainerName)
}

func TestListingPage_FiltersByLabel(t *testing.T) {
	items := []projecttypes.Details{
		{ID: "tagged", RuntimeServices: []projecttypes.RuntimeService{
			{Name: "web", ContainerLabels: map[string]string{"heal": "true"}},
			{Name: "db", ContainerLabels: map[string]string{"tags": "a,b"}},
		}},
		{ID: "other", RuntimeServices: []projecttypes.RuntimeService{{Name: "web", ContainerLabels: map[string]string{"heal": "false"}}}},
		{ID: "down"},
	}
	tests := []struct {
		name   string
		filter string
		want   []string
	}{
		{name: "key only", filter: "heal", want: []string{"tagged", "other"}},
		{name: "key and value", filter: "heal=true", want: []string{"tagged"}},
		{name: "missing key", filter: "missing", want: nil},
		{name: "value containing comma", filter: "tags=a,b", want: []string{"tagged"}},
		{name: "blank is a no-op", filter: " ", want: []string{"tagged", "other", "down"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := listing.Page(items, pagination.QueryParams{Filters: map[string]string{"label": tt.filter}, Limit: -1}, func(string) bool { return true })
			var got []string
			for _, item := range result.Items {
				got = append(got, item.ID)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestProjectService_ListProjects_FiltersByUpdateStatus(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	imageService := image.NewImageService(db, nil, nil, nil, nil, nil, nil, nil)
	svc := NewProjectService(db, settingsService, nil, imageService, nil, nil, nil, config.Load(), nil, nil)

	updatedPath := createComposeProjectDir(t, projectsDir, "updated-demo")
	require.NoError(t, os.WriteFile(filepath.Join(updatedPath, "compose.yaml"), []byte("services:\n  app:\n    image: nginx:latest\n"), 0o644))
	upToDatePath := createComposeProjectDir(t, projectsDir, "current-demo")
	require.NoError(t, os.WriteFile(filepath.Join(upToDatePath, "compose.yaml"), []byte("services:\n  app:\n    image: redis:7\n"), 0o644))
	errorPath := createComposeProjectDir(t, projectsDir, "error-demo")
	require.NoError(t, os.WriteFile(filepath.Join(errorPath, "compose.yaml"), []byte("services:\n  app:\n    image: busybox:latest\n"), 0o644))
	unknownPath := createComposeProjectDir(t, projectsDir, "unknown-demo")
	require.NoError(t, os.WriteFile(filepath.Join(unknownPath, "compose.yaml"), []byte("services:\n  app:\n    image: alpine:latest\n"), 0o644))

	require.NoError(t, db.Create(&Project{
		ID:      "project-updated",
		Name:    "updated-demo",
		DirName: new("updated-demo"),
		Path:    updatedPath,
		Status:  ProjectStatusStopped,
	}).Error)
	require.NoError(t, db.Create(&Project{
		ID:      "project-current",
		Name:    "current-demo",
		DirName: new("current-demo"),
		Path:    upToDatePath,
		Status:  ProjectStatusStopped,
	}).Error)
	require.NoError(t, db.Create(&Project{
		ID:      "project-error",
		Name:    "error-demo",
		DirName: new("error-demo"),
		Path:    errorPath,
		Status:  ProjectStatusStopped,
	}).Error)
	require.NoError(t, db.Create(&Project{
		ID:      "project-unknown",
		Name:    "unknown-demo",
		DirName: new("unknown-demo"),
		Path:    unknownPath,
		Status:  ProjectStatusStopped,
	}).Error)

	now := time.Now().UTC()
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("native-v1:classic:linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:updated-image",
		Repository:     "docker.io/library/nginx",
		Tag:            "latest",
		HasUpdate:      true,
		UpdateType:     "digest",
		CurrentVersion: "latest",
		CheckTime:      now,
	}).Error)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{
		ID:             "sha256:current-image",
		Repository:     "docker.io/library/redis",
		Tag:            "7",
		HasUpdate:      false,
		UpdateType:     "tag",
		CurrentVersion: "7",
		CheckTime:      now.Add(-time.Minute),
	}).Error)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{
		ID:             "sha256:error-image",
		Repository:     "docker.io/library/busybox",
		Tag:            "latest",
		HasUpdate:      false,
		UpdateType:     "error",
		CurrentVersion: "latest",
		CheckTime:      now.Add(-2 * time.Minute),
		LastError:      new("registry timeout"),
	}).Error)

	tests := []struct {
		name     string
		filter   string
		expected []string
	}{
		{name: "has update", filter: "has_update", expected: []string{"updated-demo"}},
		{name: "up to date", filter: "up_to_date", expected: []string{"current-demo"}},
		{name: "error", filter: "error", expected: []string{"error-demo"}},
		{name: "unknown", filter: "unknown", expected: []string{"unknown-demo"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, page, listProjectsErr := svc.ListProjects(ctx, pagination.QueryParams{
				Filters: map[string]string{
					"updates": tt.filter,
				},
				Limit: -1,
				Sort:  "name", Order: pagination.SortAsc,
			})
			require.NoError(t, listProjectsErr)
			require.EqualValues(t, len(tt.expected), page.TotalItems)

			names := make([]string, 0, len(items))
			for _, item := range items {
				names = append(names, item.Name)
			}
			assert.Equal(t, tt.expected, names)
		})
	}
}

func TestBuildDiscoveredComposeProjectUpdateRows(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()
	imageService := image.NewImageService(db, nil, nil, nil, nil, nil, nil, nil)
	now := time.Now().UTC()

	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("native-v1:classic:linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:media-image",
		Repository:     "docker.io/library/nginx",
		Tag:            "latest",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeDigest,
		CurrentVersion: "latest",
		CheckTime:      now,
	}).Error)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("native-v1:classic:linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:known-image",
		Repository:     "docker.io/library/redis",
		Tag:            "7",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeDigest,
		CurrentVersion: "7",
		CheckTime:      now,
	}).Error)
	rows := discoveryListingForTest(imageService).DiscoveredUpdateRows(ctx, []container.Summary{
		{
			ID:      "media-web",
			Names:   []string{"/media-web-1"},
			Image:   "nginx:latest",
			ImageID: "sha256:media-image",
			State:   "running",
			Labels: map[string]string{
				"com.docker.compose.project": "media",
				"com.docker.compose.service": "web",
			},
		},
		{
			ID:      "known-cache",
			Image:   "redis:7",
			ImageID: "sha256:known-image",
			State:   "running",
			Labels: map[string]string{
				"com.docker.compose.project": "known",
				"com.docker.compose.service": "cache",
			},
		},
		{
			ID:      "plain-container",
			Image:   "busybox:latest",
			ImageID: "sha256:plain-image",
			State:   "running",
			Labels:  map[string]string{},
		},
	}, map[string]struct{}{"known": {}}, iconcatalog.DefaultCatalog)

	require.Len(t, rows, 1)
	row := rows[0]
	assert.Equal(t, "compose:media", row.ID)
	assert.Equal(t, "media", row.Name)
	assert.True(t, row.IsDiscovered)
	assert.Equal(t, string(ProjectStatusRunning), row.Status)
	require.NotNil(t, row.UpdateInfo)
	assert.True(t, row.UpdateInfo.HasUpdate)
	assert.Equal(t, []string{"nginx:latest"}, row.UpdateInfo.UpdatedImageRefs)
	require.Len(t, row.RuntimeServices, 1)
	assert.Equal(t, "web", row.RuntimeServices[0].Name)
}

func TestBuildDiscoveredComposeProjectUpdateRows_FallsBackToImageID(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()
	imageService := image.NewImageService(db, nil, nil, nil, nil, nil, nil, nil)

	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("native-v1:classic:linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:media-image",
		Repository:     "registry.example.test/custom/media",
		Tag:            "latest",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeDigest,
		CurrentVersion: "latest",
		CheckTime:      time.Now().UTC(),
	}).Error)

	containers := []container.Summary{
		{
			ID:      "media-web",
			Image:   "nginx:latest",
			ImageID: "sha256:media-image",
			State:   "running",
			Labels: map[string]string{
				"com.docker.compose.project": "media",
				"com.docker.compose.service": "web",
			},
		},
		{
			ID:      "media-web-replica",
			Image:   "nginx:latest",
			ImageID: "sha256:media-image",
			State:   "running",
			Labels: map[string]string{
				"com.docker.compose.project": "media",
				"com.docker.compose.service": "web",
				labels.LabelUpdater:          "false",
			},
		},
		{
			ID:      "pinned-web",
			Image:   "nginx:1.25.0",
			ImageID: "sha256:media-image",
			State:   "running",
			Labels: map[string]string{
				"com.docker.compose.project": "pinned",
				"com.docker.compose.service": "web",
				labels.LabelUpdateStrategy:   "tag",
			},
		},
	}
	rows := discoveryListingForTest(imageService).DiscoveredUpdateRows(ctx, containers, map[string]struct{}{}, iconcatalog.DefaultCatalog)

	require.Len(t, rows, 1, "replicas count once and a tag policy never inherits the digest check")
	assert.Equal(t, "compose:media", rows[0].ID)
	assert.Equal(t, []string{"nginx:latest"}, rows[0].UpdateInfo.UpdatedImageRefs)

	// The tracked-project path resolves the same record through the runtime image ID.
	service := &ProjectService{imageService: imageService}
	service.initChildren(nil, nil)
	detail := projecttypes.Details{
		ID: "tracked",
		RuntimeServices: []projecttypes.RuntimeService{
			{
				Name:            "web",
				ContainerID:     "media-web",
				Image:           "nginx:latest",
				ImageID:         "sha256:media-image",
				ContainerLabels: containers[0].Labels,
			},
		},
	}
	service.enrichProjectUpdateInfo(ctx, &detail)
	require.True(t, detail.UpdateInfo.HasUpdate)
	assert.Equal(t, []string{"nginx:latest"}, detail.UpdateInfo.UpdatedImageRefs)
	detail.RuntimeServices[0].ImageID = ""
	service.enrichProjectUpdateInfo(ctx, &detail)
	require.False(t, detail.UpdateInfo.HasUpdate, "a missing runtime image ID inherits nothing")
}

func TestProjectService_ListProjects_FiltersArchivedProjects(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	projectsRoot := t.TempDir()
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	activePath := createComposeProjectDir(t, projectsRoot, "active-demo")
	archivedPath := createComposeProjectDir(t, projectsRoot, "archived-demo")
	require.NoError(t, db.Create(&Project{
		ID:      "project-active",
		Name:    "active-demo",
		DirName: new("active-demo"),
		Path:    activePath,
		Status:  ProjectStatusStopped,
	}).Error)
	require.NoError(t, db.Create(&Project{
		ID:         "project-archived",
		Name:       "archived-demo",
		DirName:    new("archived-demo"),
		Path:       archivedPath,
		Status:     ProjectStatusStopped,
		IsArchived: true,
		ArchivedAt: new(time.Now().UTC()),
	}).Error)

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)

	items, page, err := svc.ListProjects(ctx, pagination.QueryParams{
		Limit: -1,
		Sort:  "name", Order: pagination.SortAsc,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.TotalItems)
	require.Len(t, items, 1)
	assert.Equal(t, "active-demo", items[0].Name)

	items, page, err = svc.ListProjects(ctx, pagination.QueryParams{
		Filters: map[string]string{"archived": "true"},
		Limit:   -1,
		Sort:    "name", Order: pagination.SortAsc,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.TotalItems)
	require.Len(t, items, 1)
	assert.Equal(t, "archived-demo", items[0].Name)
	assert.True(t, items[0].IsArchived)

	items, page, err = svc.ListProjects(ctx, pagination.QueryParams{
		Filters: map[string]string{"archived": "all"},
		Limit:   -1,
		Sort:    "name", Order: pagination.SortAsc,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, page.TotalItems)
	require.Len(t, items, 2)
	assert.Equal(t, []string{"active-demo", "archived-demo"}, []string{items[0].Name, items[1].Name})
}
