// Source: getarcaneapp/arcane/backend/internal/project/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture.

package project

import (
	"context"
	"encoding/json"
	"errors"

	"io"

	"net/http"
	"net/http/httptest"

	"os"
	"path/filepath"

	"strings"

	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"

	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"

	"github.com/moby/moby/api/types/container"
	dockertypesimage "github.com/moby/moby/api/types/image"
	dockerregistry "github.com/moby/moby/api/types/registry"

	"github.com/opencontainers/go-digest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/config"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
)

func TestProjectService_ComposePullSelectedServices_LeavesRecordsWhenPullFails(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDB(t)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	imageRef := "registry.example.com/team/app:9.9.9"
	repository := "registry.example.com/team/app"

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/images/create") {
			http.Error(w, "pull failed", http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(failingServer.Close)

	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, failingServer)}
	imageUpdateService := imageupdate.NewImageUpdateService(db, nil, nil, dockerService, nil, nil, nil)
	imageService := image.NewImageService(db, dockerService, nil, imageUpdateService, nil, event.NewEventService(db, nil, nil), nil, nil)
	svc := NewProjectService(db, settingsService, nil, imageService, dockerService, nil, nil, config.Load(), nil, nil)

	projectDef := &types.Project{
		Name: "compose-selected",
		Services: types.Services{
			"app": {
				Name:  "app",
				Image: imageRef,
			},
		},
	}

	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:selected-old",
		Repository:     repository,
		Tag:            "9.9.9",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeDigest,
		CurrentVersion: "9.9.9",
		CheckTime:      time.Now().UTC().Add(-time.Hour),
	}).Error)

	err = svc.composeCoordinator.PullServices(ctx, projectDef, []string{"app"}, svc.deployment.ImageOperations(nil, nil), nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to pull image")

	var selectedRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "sha256:selected-old").First(&selectedRecord).Error)
	assert.True(t, selectedRecord.HasUpdate)

	var count int64
	require.NoError(t, db.WithContext(ctx).Model(&imageupdate.ImageUpdateRecord{}).Where("id = ?", "sha256:team-app-compose").Count(&count).Error)
	assert.Zero(t, count)
}

func TestProjectService_UpdateProjectServicesHardFailsWhenPullFails(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDB(t)
	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	imageRef := "registry.example.com/team/app:9.9.9"
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/images/create") {
			http.Error(w, "compose pull failed", http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(failingServer.Close)

	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, failingServer)}
	imageUpdateService := imageupdate.NewImageUpdateService(db, nil, nil, dockerService, nil, nil, nil)
	imageService := image.NewImageService(db, dockerService, nil, imageUpdateService, nil, event.NewEventService(db, nil, nil), nil, nil)

	projectPath := createComposeProjectDir(t, projectsDir, "compose-update-pull-fail")
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(
				projectPath,
				"compose.yaml",
			),
			[]byte(
				"services:\n  app:\n    image: "+imageRef+"\n    labels:\n      com.getarcaneapp.arcane.updater.strategy: digest\n",
			),
			0o644,
		),
	)

	projectRecord := &Project{
		ID:      "project-update-pull-fail",
		Name:    "compose-update-pull-fail",
		DirName: new("compose-update-pull-fail"),
		Path:    projectPath,
		Status:  ProjectStatusRunning,
	}
	require.NoError(t, db.Create(projectRecord).Error)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:selected-old",
		Repository:     "registry.example.com/team/app",
		Tag:            "9.9.9",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeDigest,
		CurrentVersion: "9.9.9",
		CheckTime:      time.Now().UTC().Add(-time.Hour),
	}).Error)

	originalComposeUp := composeUpProjectServices
	t.Cleanup(func() {
		composeUpProjectServices = originalComposeUp
	})
	upCalled := false
	composeUpProjectServices = func(context.Context, *types.Project, []string, bool, bool, bool, map[string]dockerregistry.AuthConfig, time.Duration) error {
		upCalled = true
		return errors.New("compose up should not run")
	}

	svc := NewProjectService(db, settingsService, nil, imageService, dockerService, nil, nil, config.Load(), nil, nil)
	err = svc.UpdateProjectServices(ctx, projectRecord.ID, []string{"app"}, usertypes.SystemUser, true)
	require.Error(t, err)
	require.ErrorContains(t, err, "pull updated service images")
	assert.False(t, upCalled, "compose up must not run after a pull failure")

	var persistedProject Project
	require.NoError(t, db.WithContext(ctx).Where("id = ?", projectRecord.ID).First(&persistedProject).Error)
	assert.Equal(t, ProjectStatusRunning, persistedProject.Status)

	var persistedRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "sha256:selected-old").First(&persistedRecord).Error)
	assert.True(t, persistedRecord.HasUpdate)
}

func TestProjectService_UpdateProjectServicesForcesRecreate(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDB(t)
	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	imageRef := "registry.example.com/team/app:9.9.9"
	repository := "registry.example.com/team/app"
	imageID := "sha256:project-update-force"
	imageDigest := digest.FromString("project-update-force-digest").String()

	server := newImagePullServerWithObserver(t, map[string]dockertypesimage.InspectResponse{
		imageRef: {
			ID:          imageID,
			RepoTags:    []string{imageRef},
			RepoDigests: []string{repository + "@" + imageDigest},
		},
	}, func(fullRef, _ string) {
		assert.Equal(t, imageRef, fullRef, "namespace dependents must not be pulled")
	})

	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	imageUpdateService := imageupdate.NewImageUpdateService(db, nil, nil, dockerService, nil, nil, nil)
	eventService := event.NewEventService(db, nil, nil)
	imageService := image.NewImageService(db, dockerService, nil, imageUpdateService, nil, eventService, nil, nil)

	projectPath := createComposeProjectDir(t, projectsDir, "compose-update-force")
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(
				projectPath,
				"compose.yaml",
			),
			[]byte(
				"services:\n  app:\n    image: "+imageRef+"\n    labels:\n      com.getarcaneapp.arcane.updater.strategy: digest\n  sidecar:\n    image: "+
					"busybox:latest\n    network_mode: service:app\n  dormant:\n    image: busybox:latest\n    network_mode: "+
					"service:app\n  never:\n    image: busybox:latest\n    pid: service:app\n  gated:\n    image: "+
					"busybox:latest\n    network_mode: service:app\n    profiles: [extra]\n  unrelated:\n    image: "+
					"busybox:latest\n",
			),
			0o644,
		),
	)

	composeContainer := func(service string, state container.ContainerState) container.Summary {
		return container.Summary{
			ID:     service + "-container",
			Names:  []string{"/compose-update-force-" + service + "-1"},
			State:  state,
			Labels: map[string]string{api.ProjectLabel: "compose-update-force", api.ServiceLabel: service, api.ConfigHashLabel: service + "-hash"},
		}
	}
	runtimeServer := newProjectRuntimeDockerServer(t, []container.Summary{
		composeContainer("app", container.StateRunning),
		composeContainer("sidecar", container.StateRunning),
		composeContainer("dormant", container.StateExited),
	})
	t.Setenv("DOCKER_HOST", dockerHostFromProjectRuntimeServerURL(t, runtimeServer.URL))

	projectRecord := &Project{
		ID:      "project-update-force",
		Name:    "compose-update-force",
		DirName: new("compose-update-force"),
		Path:    projectPath,
		Status:  ProjectStatusRunning,
	}
	require.NoError(t, db.Create(projectRecord).Error)

	originalComposeStop := composeStopProjectServices
	originalComposeUp := composeUpProjectServices
	t.Cleanup(func() {
		composeStopProjectServices = originalComposeStop
		composeUpProjectServices = originalComposeUp
	})
	var stopped []string
	composeStopProjectServices = func(_ context.Context, _ *types.Project, services []string) error {
		stopped = services
		return nil
	}
	upCalled := false
	forceRecreate := false
	composeUpProjectServices = func(
		_ context.Context,
		selected *types.Project,
		services []string,
		removeOrphans, force, _ bool,
		_ map[string]dockerregistry.AuthConfig,
		_ time.Duration,
	) error {
		assert.True(t, eventService.ShouldSuppressDaemonEvent("container", "replacement", "app", selected.Name))
		assert.False(t, eventService.ShouldSuppressDaemonEvent("image", "pulled-image", "", ""))
		upCalled = true
		assert.Equal(t, []string{"app", "dormant", "sidecar"}, selected.ServiceNames(), "stopped dependents stay in the model for recreation")
		forceRecreate = force
		assert.Equal(t, []string{"app", "sidecar"}, services, "stopped dependents are not started")
		assert.False(t, removeOrphans)
		return errors.New("compose up failed after assertion")
	}

	svc := NewProjectService(db, settingsService, eventService, imageService, dockerService, nil, nil, config.Load(), nil, nil)
	err = svc.UpdateProjectServices(ctx, projectRecord.ID, []string{"app"}, usertypes.SystemUser, true)
	require.Error(t, err)
	assert.True(t, eventService.ShouldSuppressDaemonEvent("container", "replacement", "app", "compose-update-force"), "failed updates retain correlation through rollback grace")
	assert.False(t, eventService.ShouldSuppressDaemonEvent("container", "unrelated", "unrelated", "other-project"))
	assert.True(t, upCalled)
	assert.Equal(t, []string{"app", "sidecar"}, stopped)
	assert.True(t, forceRecreate, "service updates must force recreate after pulling the updated image")
}

type fakeProjectVolumeRenameMigration struct {
	applyCalled    bool
	commitCalled   bool
	rollbackCalled bool
	applyErr       error
	commitErr      error
	rollbackErr    error
}

func (m *fakeProjectVolumeRenameMigration) Apply(context.Context) error {
	m.applyCalled = true
	return m.applyErr
}

func (m *fakeProjectVolumeRenameMigration) Rollback(context.Context) error {
	m.rollbackCalled = true
	return m.rollbackErr
}

func (m *fakeProjectVolumeRenameMigration) Commit(context.Context) error {
	m.commitCalled = true
	return m.commitErr
}

func TestProjectService_UpdateProject_RenameFailsWhenVolumeMigrationPreparationFails(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	eventService := event.NewEventService(db, nil, nil)
	dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			_, _ = io.WriteString(w, "OK")
		case strings.HasSuffix(r.URL.Path, "/version"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]string{
				"ApiVersion":    "1.41",
				"MinAPIVersion": "1.24",
				"Version":       "24.0.0",
			})) {
				return
			}
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/bar_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"Name": "bar_data",
			})) {
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(dockerServer.Close)
	t.Setenv("DOCKER_HOST", dockerHostFromProjectRuntimeServerURL(t, dockerServer.URL))

	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, dockerServer)}
	svc := NewProjectService(db, settingsService, eventService, nil, dockerService, nil, nil, config.Load(), nil, nil)

	originalDirName := "Foo"
	originalPath := createComposeProjectDir(t, projectsDir, originalDirName)
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(
				originalPath,
				"compose.yaml",
			),
			[]byte(
				"services:\n  app:\n    image: nginx:alpine\n    volumes:\n      - data:/data\nvolumes:\n  data:\n    driver: local\n",
			),
			0o644,
		),
	)

	project := &Project{
		ID:      "proj-volume-conflict",
		Name:    "Foo",
		DirName: &originalDirName,
		Path:    originalPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	_, err = svc.UpdateProject(ctx, project.ID, new("bar"), nil, nil, nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "target volume already exists")
	assert.DirExists(t, originalPath)
	assert.NoDirExists(t, filepath.Join(projectsDir, "bar"))

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	assert.Equal(t, "Foo", fromDB.Name)
	assert.Equal(t, originalPath, fromDB.Path)
}

func TestProjectService_ApplyProjectUpdateWithRenameJournal_AppliesVolumeMigrationWhenNameChanges(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	migration := &fakeProjectVolumeRenameMigration{}

	originalDirName := "Foo"
	originalPath := createComposeProjectDir(t, projectsDir, originalDirName)

	project := &Project{
		ID:      "proj-volume-success",
		Name:    "Foo",
		DirName: &originalDirName,
		Path:    originalPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	projectForUpdate := *project
	journalActive := false
	projectStateCommitted := false
	err = svc.applyProjectUpdate(ctx, &projectForUpdate, true, "bar", projectsDir, nil, nil, nil, migration, nil, &journalActive, &projectStateCommitted)
	require.NoError(t, err)

	assert.True(t, migration.applyCalled)
	assert.True(t, migration.commitCalled)
	assert.False(t, migration.rollbackCalled)
	assert.True(t, projectStateCommitted)
	assert.Equal(t, "bar", projectForUpdate.Name)
	assert.DirExists(t, filepath.Join(projectsDir, "bar"))
	assert.NoDirExists(t, originalPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	assert.Equal(t, "bar", fromDB.Name)
	assert.Equal(t, filepath.Join(projectsDir, "bar"), fromDB.Path)
}

func TestProjectService_PrepareProjectRenameVolumeMigrationForUpdate_UsesComposePreview(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"Name":   "nginx_data",
				"Driver": "local",
				"Labels": map[string]string{
					api.ProjectLabel: "nginx",
					api.VolumeLabel:  "data",
				},
			})) {
				return
			}
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(dockerServer.Close)

	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, dockerServer)}
	svc := NewProjectService(db, settingsService, nil, nil, dockerService, nil, nil, config.Load(), nil, nil)

	projectPath := filepath.Join(projectsDir, "nginx")
	require.NoError(t, os.MkdirAll(projectPath, 0o755))
	oldCompose := "services:\n  app:\n    image: nginx:alpine\n    volumes:\n      - data:/data\nvolumes:\n  data:\n    driver: local\n"
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte(oldCompose), 0o644))

	project := &Project{
		ID:      "proj-preview-volume-rename",
		Name:    "nginx",
		DirName: new("nginx"),
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}

	t.Run("skips volume made explicit in pending compose", func(t *testing.T) {
		newCompose := "services:\n  app:\n    image: nginx:alpine\n    volumes:\n      - data:/data\nvolumes:\n  data:\n    name: fixed-data\n"

		migration, prepareErr := svc.prepareProjectRenameVolumeMigration(ctx, project, "web", projectsDir, &newCompose, nil, nil)

		require.NoError(t, prepareErr)
		require.Nil(t, migration)
		bytes, readErr := os.ReadFile(filepath.Join(projectPath, "compose.yaml"))
		require.NoError(t, readErr)
		require.Equal(t, oldCompose, string(bytes))
		require.Empty(t, projectUpdatePreviewDirs(t, projectsDir))
	})

	t.Run("plans unchanged auto-managed volume from pending compose", func(t *testing.T) {
		newCompose := "services:\n  app:\n    image: nginx:alpine\n    volumes:\n      - data:/data\nvolumes:\n  data:\n    driver: local\n"

		migration, prepareErr := svc.prepareProjectRenameVolumeMigration(ctx, project, "web", projectsDir, &newCompose, nil, nil)

		require.NoError(t, prepareErr)
		require.NotNil(t, migration)
		journalSource, ok := migration.(volumetypes.JournalSource)
		require.True(t, ok)
		journalVolumes := journalSource.JournalVolumes()
		require.Len(t, journalVolumes, 1)
		require.Equal(t, "data", journalVolumes[0].Key)
		require.Equal(t, "nginx_data", journalVolumes[0].OldName)
		require.Equal(t, "web_data", journalVolumes[0].NewName)
		require.Empty(t, projectUpdatePreviewDirs(t, projectsDir))
	})

	t.Run("plans auto-managed volume when pending compose name renames project", func(t *testing.T) {
		newCompose := "name: web\nservices:\n  app:\n    image: nginx:alpine\n    volumes:\n      - data:/data\nvolumes:\n  data:\n    driver: local\n"

		migration, prepareErr := svc.prepareProjectRenameVolumeMigration(ctx, project, "web", projectsDir, &newCompose, nil, nil)

		require.NoError(t, prepareErr)
		require.NotNil(t, migration)
		journalSource, ok := migration.(volumetypes.JournalSource)
		require.True(t, ok)
		journalVolumes := journalSource.JournalVolumes()
		require.Len(t, journalVolumes, 1)
		require.Equal(t, "data", journalVolumes[0].Key)
		require.Equal(t, "nginx_data", journalVolumes[0].OldName)
		require.Equal(t, "web_data", journalVolumes[0].NewName)
		require.Empty(t, projectUpdatePreviewDirs(t, projectsDir))
	})

	t.Run("plans interpolated explicit name from pending compose", func(t *testing.T) {
		newCompose := "services:\n  app:\n    image: nginx:alpine\n    volumes:\n      - data:/data\nvolumes:\n  data:\n    name: ${DATA_VOLUME:-nginx_data}\n"

		migration, prepareErr := svc.prepareProjectRenameVolumeMigration(ctx, project, "web", projectsDir, &newCompose, nil, nil)

		require.NoError(t, prepareErr)
		require.NotNil(t, migration)
		journalSource, ok := migration.(volumetypes.JournalSource)
		require.True(t, ok)
		journalVolumes := journalSource.JournalVolumes()
		require.Len(t, journalVolumes, 1)
		require.Equal(t, "data", journalVolumes[0].Key)
		require.Equal(t, "nginx_data", journalVolumes[0].OldName)
		require.Equal(t, "web_data", journalVolumes[0].NewName)
		require.Empty(t, projectUpdatePreviewDirs(t, projectsDir))
	})
}

func TestProjectService_ApplyProjectUpdateWithRenameJournal_RollsBackVolumeMigrationWhenProjectSaveFails(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	migration := &fakeProjectVolumeRenameMigration{}

	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("arcane_test_project_save_failure", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Project" {
			_ = tx.AddError(errors.New("forced project save failure"))
		}
	}))

	originalDirName := "Foo"
	originalPath := createComposeProjectDir(t, projectsDir, originalDirName)

	project := &Project{
		ID:      "proj-volume-save-fail",
		Name:    "Foo",
		DirName: &originalDirName,
		Path:    originalPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	projectForUpdate := *project
	journalActive := false
	projectStateCommitted := false
	err = withProjectRenameRollback(ctx, &projectForUpdate, &projectStateCommitted, func() error {
		return svc.applyProjectUpdate(ctx, &projectForUpdate, true, "bar", projectsDir, nil, nil, nil, migration, nil, &journalActive, &projectStateCommitted)
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced project save failure")
	assert.True(t, migration.applyCalled)
	assert.False(t, migration.commitCalled)
	assert.True(t, migration.rollbackCalled)
	assert.False(t, projectStateCommitted)
	assert.DirExists(t, originalPath)
	assert.NoDirExists(t, filepath.Join(projectsDir, "bar"))
}

func TestProjectService_ApplyProjectUpdateWithRenameJournal_SucceedsCommittedRenameWhenSourceCleanupFails(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	migration := &fakeProjectVolumeRenameMigration{
		commitErr: errors.New("source cleanup failed"),
	}

	originalDirName := "Foo"
	originalPath := createComposeProjectDir(t, projectsDir, originalDirName)

	project := &Project{
		ID:      "proj-volume-cleanup-fail",
		Name:    "Foo",
		DirName: &originalDirName,
		Path:    originalPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	projectForUpdate := *project
	journalActive := false
	projectStateCommitted := false
	err = withProjectRenameRollback(ctx, &projectForUpdate, &projectStateCommitted, func() error {
		return svc.applyProjectUpdate(ctx, &projectForUpdate, true, "bar", projectsDir, nil, nil, nil, migration, nil, &journalActive, &projectStateCommitted)
	})
	require.NoError(t, err)
	require.True(t, migration.applyCalled)
	require.True(t, migration.commitCalled)
	require.False(t, migration.rollbackCalled)
	require.True(t, projectStateCommitted)
	require.NoDirExists(t, originalPath)
	require.DirExists(t, filepath.Join(projectsDir, "bar"))

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "bar", fromDB.Name)
	require.Equal(t, filepath.Join(projectsDir, "bar"), fromDB.Path)
}

func TestProjectService_UpdateProject_ClearsJournalForNonRenameWhenRecoveryDockerUnavailable(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	eventService := event.NewEventService(db, nil, nil)
	kvService := kv.NewKVService(db)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), kvService, nil)

	oldDir := "nginx"
	projectPath := createComposeProjectDir(t, projectsDir, oldDir)
	project := &Project{
		ID:      "proj-non-rename-recovery-docker-unavailable",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    projectPath,
		NewPath:    filepath.Join(projectsDir, "web"),
		OldDirName: &oldDir,
		NewDirName: "web",
		Phase:      projecttypes.RenameJournalPhaseTargetsCopied,
		Volumes: []volumetypes.JournalVolume{
			{
				Key:     "data",
				OldName: "nginx_data",
				NewName: "web_data",
			},
		},
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	envContent := "FOO=bar\n"
	updated, err := svc.UpdateProject(ctx, project.ID, nil, nil, &envContent, nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.NoError(t, err)
	require.Equal(t, "nginx", updated.Name)

	envBytes, err := os.ReadFile(filepath.Join(projectPath, ".env"))
	require.NoError(t, err)
	require.Equal(t, envContent, string(envBytes))

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestProjectService_UpdateProject_AllowsRenameAfterJournalRecoveryWithoutDockerService(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	eventService := event.NewEventService(db, nil, nil)
	kvService := kv.NewKVService(db)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), kvService, nil)
	configureProjectRuntimeDocker(t, nil)

	oldDir := "nginx"
	projectPath := createComposeProjectDir(t, projectsDir, oldDir)
	project := &Project{
		ID:      "proj-rename-recovery-docker-unavailable",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    projectPath,
		NewPath:    filepath.Join(projectsDir, "web"),
		OldDirName: &oldDir,
		NewDirName: "web",
		Phase:      projecttypes.RenameJournalPhaseTargetsCopied,
		Volumes: []volumetypes.JournalVolume{
			{
				Key:     "data",
				OldName: "nginx_data",
				NewName: "web_data",
			},
		},
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	updated, err := svc.UpdateProject(ctx, project.ID, new("web"), nil, nil, nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, "web", updated.Name)
	require.NoDirExists(t, projectPath)
	require.DirExists(t, filepath.Join(projectsDir, "web"))

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestProjectService_UpdateProject_RenamesDirectoryWhenNameChanges(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)
	configureProjectRuntimeDocker(t, nil)

	originalDirName := "Foo"
	originalPath := createComposeProjectDir(t, projectsDir, originalDirName)

	project := &Project{
		ID:      "proj-1",
		Name:    "Foo",
		DirName: &originalDirName,
		Path:    originalPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	updated, err := svc.UpdateProject(ctx, project.ID, new("bar"), nil, nil, nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.NoError(t, err)

	expectedPath := filepath.Join(projectsDir, "bar")
	assert.Equal(t, "bar", updated.Name)
	assert.Equal(t, expectedPath, updated.Path)
	require.NotNil(t, updated.DirName)
	assert.Equal(t, "bar", *updated.DirName)
	assert.NoDirExists(t, originalPath)
	assert.DirExists(t, expectedPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	assert.Equal(t, "bar", fromDB.Name)
	assert.Equal(t, expectedPath, fromDB.Path)
	require.NotNil(t, fromDB.DirName)
	assert.Equal(t, "bar", *fromDB.DirName)
}
