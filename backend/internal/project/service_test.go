// Source: getarcaneapp/arcane/backend/internal/project/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture.

package project

import (
	"context"
	"encoding/json"

	"fmt"
	"io"

	"net/http"
	"net/http/httptest"

	"net/url"
	"os"
	"path/filepath"

	"strings"

	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"

	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/types/containerregistry"

	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"

	"github.com/moby/moby/api/pkg/authconfig"

	dockertypesimage "github.com/moby/moby/api/types/image"
	dockerregistry "github.com/moby/moby/api/types/registry"

	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
	"github.com/samber/mo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	buildtypes "go.getarcane.app/builds/types"
	"go.getarcane.app/sys/crypto"

	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"

	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
)

type testBuildBuilder struct {
	err error
}

func (b testBuildBuilder) BuildImage(_ context.Context, _ string, _ buildtypes.BuildRequest, _ io.Writer, _ string, _ *usertypes.Actor) (*buildtypes.BuildResult, error) {
	if b.err != nil {
		return nil, b.err
	}
	return &buildtypes.BuildResult{Provider: "local"}, nil
}

func (testBuildBuilder) BuildSettings() buildtypes.BuildSettings {
	return buildtypes.BuildSettings{}
}

var _ builder = testBuildBuilder{}

func setupProjectTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Project{}, &ProjectTag{}, &settings.SettingVariable{}, &imageupdate.ImageUpdateRecord{}, &event.Event{}))
	return &database.DB{DB: db}
}

func TestUpdateProjectWorkspaceRejectsInvalidManifestBeforeProjectLookup(t *testing.T) {
	service := &ProjectService{}
	service.initChildren(nil, nil)

	_, err := service.UpdateProjectWorkspace(t.Context(), "missing", projecttypes.WorkspaceUpdateManifest{}, nil, usertypes.Actor{})
	require.ErrorIs(t, err, common.ErrProjectWorkspaceBadRequest)
	require.ErrorContains(t, err, "revision")
}

func newSettingsServiceForTest(t *testing.T, ctx context.Context, db *database.DB) (*settings.SettingsService, error) {
	t.Helper()
	svc, err := settings.NewSettingsService(ctx, db)
	if err == nil {
		t.Cleanup(func() { require.NoError(t, svc.Stop(context.WithoutCancel(t.Context()))) })
	}
	return svc, err
}

func newTestDockerClient(t *testing.T, server *httptest.Server) *client.Client {
	t.Helper()

	cli, err := client.New(
		client.WithHost(server.URL),
		client.WithAPIVersion("1.41"),
		client.WithHTTPClient(server.Client()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func decodeRegistryAuth(t *testing.T, encoded string) dockerregistry.AuthConfig {
	t.Helper()
	cfg, err := authconfig.Decode(encoded)
	require.NoError(t, err)
	return *cfg
}

func newImagePullServerWithObserver(t *testing.T, inspectByRef map[string]dockertypesimage.InspectResponse, onPull func(fullRef, authHeader string)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/images/create"):
			fullRef := strings.TrimSpace(r.URL.Query().Get("fromImage"))
			tag := strings.TrimSpace(r.URL.Query().Get("tag"))
			if fullRef != "" && tag != "" {
				lastSlash := strings.LastIndex(fullRef, "/")
				lastColon := strings.LastIndex(fullRef, ":")
				if lastColon <= lastSlash {
					fullRef += ":" + tag
				}
			}
			if onPull != nil {
				onPull(fullRef, strings.TrimSpace(r.Header.Get("X-Registry-Auth")))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "Pulled", "id": fullRef})
		case strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			path := r.URL.Path
			imagePathIndex := strings.Index(path, "/images/")
			if !assert.NotEqual(t, -1, imagePathIndex) {
				return
			}
			encodedRef := strings.TrimSuffix(path[imagePathIndex+len("/images/"):], "/json")
			imageRef, err := url.PathUnescape(encodedRef)
			if !assert.NoError(t, err) {
				return
			}
			inspect, ok := inspectByRef[imageRef]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(inspect)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestProjectService_RefreshProjectImageRefs_PersistsBuildMetadata(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDB(t)
	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte(`services:
  explicit:
    build: .
    image: test2:latest
  worker:
    build:
      context: .
      dockerfile: Dockerfile
  regular:
    image: postgres:17
`), 0o644))

	proj := &Project{
		ID:   "project-build-image-refs",
		Name: "demo",
		Path: projectPath,
	}
	require.NoError(t, db.Create(proj).Error)

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	svc.refreshProjectImageRefs(ctx, proj)

	var saved Project
	require.NoError(t, db.First(&saved, "id = ?", proj.ID).Error)
	assert.ElementsMatch(t, []string{"test2:latest", "postgres:17"}, projects.ParseImageRefsJSON(saved.ImageRefsJSON))
	require.NotNil(t, saved.BuildImageRefsJSON)
	assert.ElementsMatch(t, []string{"test2:latest", "demo-worker"}, projects.ParseImageRefsJSON(*saved.BuildImageRefsJSON))
}

func TestProjectService_BackfillProjectImageRefs_RetriesOnlyMissingMetadata(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDB(t)
	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	validPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(validPath, "compose.yaml"), []byte(`services:
  app:
    build: .
    image: local-app:latest
`), 0o644))
	invalidPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(invalidPath, "compose.yaml"), []byte("services: [\n"), 0o644))
	regularPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(regularPath, "compose.yaml"), []byte(`services:
  app:
    image: nginx:latest
`), 0o644))

	valid := &Project{ID: "backfill-valid", Name: "valid", Path: validPath}
	invalid := &Project{ID: "backfill-invalid", Name: "invalid", Path: invalidPath}
	regular := &Project{ID: "backfill-regular", Name: "regular", Path: regularPath}
	require.NoError(t, db.Create(valid).Error)
	require.NoError(t, db.Create(invalid).Error)
	require.NoError(t, db.Create(regular).Error)

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	count, err := svc.BackfillProjectImageRefs(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, count)

	var savedValid Project
	require.NoError(t, db.First(&savedValid, "id = ?", valid.ID).Error)
	require.NotNil(t, savedValid.BuildImageRefsJSON)
	assert.Equal(t, []string{"local-app:latest"}, projects.ParseImageRefsJSON(*savedValid.BuildImageRefsJSON))

	var savedInvalid Project
	require.NoError(t, db.First(&savedInvalid, "id = ?", invalid.ID).Error)
	assert.Nil(t, savedInvalid.BuildImageRefsJSON)

	var savedRegular Project
	require.NoError(t, db.First(&savedRegular, "id = ?", regular.ID).Error)
	require.NotNil(t, savedRegular.BuildImageRefsJSON)
	assert.Equal(t, "[]", *savedRegular.BuildImageRefsJSON)

	require.NoError(t, os.Remove(filepath.Join(validPath, "compose.yaml")))
	require.NoError(t, os.Remove(filepath.Join(regularPath, "compose.yaml")))
	count, err = svc.BackfillProjectImageRefs(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NoError(t, db.First(&savedValid, "id = ?", valid.ID).Error)
	require.NotNil(t, savedValid.BuildImageRefsJSON)
	assert.Equal(t, []string{"local-app:latest"}, projects.ParseImageRefsJSON(*savedValid.BuildImageRefsJSON))
	require.NoError(t, db.First(&savedRegular, "id = ?", regular.ID).Error)
	require.NotNil(t, savedRegular.BuildImageRefsJSON)
	assert.Equal(t, "[]", *savedRegular.BuildImageRefsJSON)
}

func setupProjectDestroyTestService(t *testing.T) (*ProjectService, *database.DB, string) {
	t.Helper()

	ctx := t.Context()
	db := setupProjectTestDB(t)
	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsDir := t.TempDir()
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	eventService := event.NewEventService(db, config.Load(), nil)
	return NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil), db, projectsDir
}

func TestProjectService_DestroyProject_RemovesFilesWhenRequested(t *testing.T) {
	ctx := t.Context()
	svc, db, projectsDir := setupProjectDestroyTestService(t)

	projectPath := filepath.Join(projectsDir, "demo-remove")
	require.NoError(t, os.MkdirAll(projectPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "project-data.txt"), []byte("keep until destroy\n"), 0o644))

	dirName := "demo-remove"
	project := &Project{
		ID:      "project-destroy-remove-files",
		Name:    "demo-remove",
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	require.NoError(t, svc.DestroyProject(ctx, project.ID, true, false, usertypes.Actor{}))

	_, statErr := os.Stat(projectPath)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestProjectService_DestroyProject_PreservesFilesWhenRequested(t *testing.T) {
	ctx := t.Context()
	svc, db, projectsDir := setupProjectDestroyTestService(t)

	projectPath := filepath.Join(projectsDir, "demo-preserve")
	require.NoError(t, os.MkdirAll(projectPath, 0o755))
	projectDataPath := filepath.Join(projectPath, "project-data.txt")
	require.NoError(t, os.WriteFile(projectDataPath, []byte("preserve on destroy\n"), 0o644))

	dirName := "demo-preserve"
	project := &Project{
		ID:      "project-destroy-preserve-files",
		Name:    "demo-preserve",
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	require.NoError(t, svc.DestroyProject(ctx, project.ID, false, false, usertypes.Actor{}))

	assert.NoDirExists(t, projectPath)
	assert.NoFileExists(t, projectDataPath)

	entries, err := os.ReadDir(projectsDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	trashName := entries[0].Name()
	require.True(t, strings.HasPrefix(trashName, ".arcane-trash-demo-preserve-"), "trash dir name: %s", trashName)
	assert.FileExists(t, filepath.Join(projectsDir, trashName, "project-data.txt"))
}

func TestProjectService_GetProjectFromDatabaseByID(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, _ := newSettingsServiceForTest(t, ctx, db)
	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)

	proj := &Project{
		ID:   "p1",
		Name: "test-project",
		Path: "/tmp/test-project",
	}
	require.NoError(t, db.Create(proj).Error)

	// Test success
	found, err := svc.GetProjectFromDatabaseByID(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, "test-project", found.Name)

	// Test not found
	_, err = svc.GetProjectFromDatabaseByID(ctx, "non-existent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project not found")
}

func TestProjectService_UpdateProjectStatus(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()
	svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), nil, nil)

	proj := &Project{
		ID:     "p1",
		Status: ProjectStatusUnknown,
	}
	require.NoError(t, db.Create(proj).Error)

	err := svc.updateProjectStatus(ctx, "p1", ProjectStatusRunning)
	require.NoError(t, err)

	var updated Project
	require.NoError(t, db.First(&updated, "id = ?", "p1").Error)
	assert.Equal(t, ProjectStatusRunning, updated.Status)
	if updated.UpdatedAt != nil {
		assert.WithinDuration(t, time.Now(), *updated.UpdatedAt, time.Second)
	} else {
		assert.Fail(t, "UpdatedAt should not be nil")
	}
}

func TestProjectService_GetProjectByComposeName(t *testing.T) {
	ctx := t.Context()

	t.Run("exact match", func(t *testing.T) {
		db := setupProjectTestDB(t)
		svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), nil, nil)

		proj := &Project{
			ID:   "p1",
			Name: "myproject",
			Path: "/tmp/myproject",
		}
		require.NoError(t, db.Create(proj).Error)

		found, err := svc.GetProjectByComposeName(ctx, "myproject")
		require.NoError(t, err)
		assert.Equal(t, proj.ID, found.ID)
	})

	t.Run("normalized fallback", func(t *testing.T) {
		db := setupProjectTestDB(t)
		svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), nil, nil)

		proj := &Project{
			ID:   "p1",
			Name: "myproject",
			Path: "/tmp/myproject",
		}
		require.NoError(t, db.Create(proj).Error)

		found, err := svc.GetProjectByComposeName(ctx, "My Project!")
		require.NoError(t, err)
		assert.Equal(t, proj.ID, found.ID)
	})

	t.Run("display name in db, normalized compose label input", func(t *testing.T) {
		db := setupProjectTestDB(t)
		svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), nil, nil)

		display := &Project{
			ID:   "p2",
			Name: "My Project!",
			Path: "/tmp/my-project",
		}
		require.NoError(t, db.Create(display).Error)

		found, err := svc.GetProjectByComposeName(ctx, "myproject")
		require.NoError(t, err)
		assert.Equal(t, display.ID, found.ID)
	})

	t.Run("invalidates stale normalized cache entries after deletion", func(t *testing.T) {
		db := setupProjectTestDB(t)
		svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), nil, nil)

		original := &Project{
			ID:   "p3",
			Name: "My Project!",
			Path: "/tmp/my-project",
		}
		require.NoError(t, db.Create(original).Error)

		found, err := svc.GetProjectByComposeName(ctx, "myproject")
		require.NoError(t, err)
		assert.Equal(t, original.ID, found.ID)

		cachedProjectID, cached := svc.composeNames.projectID("myproject").Get()
		require.True(t, cached)
		assert.Equal(t, original.ID, cachedProjectID)

		require.NoError(t, db.Delete(&Project{}, "id = ?", original.ID).Error)

		replacement := &Project{
			ID:   "p4",
			Name: "My Project!",
			Path: "/tmp/my-project-recreated",
		}
		require.NoError(t, db.Create(replacement).Error)

		found, err = svc.GetProjectByComposeName(ctx, "myproject")
		require.NoError(t, err)
		assert.Equal(t, replacement.ID, found.ID)

		cachedProjectID, cached = svc.composeNames.projectID("myproject").Get()
		require.True(t, cached)
		assert.Equal(t, replacement.ID, cachedProjectID)
	})

	t.Run("invalidates stale normalized cache entries after rename", func(t *testing.T) {
		db := setupProjectTestDB(t)
		svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), nil, nil)

		original := &Project{
			ID:   "p5",
			Name: "My App!",
			Path: "/tmp/my-app",
		}
		require.NoError(t, db.Create(original).Error)

		found, err := svc.GetProjectByComposeName(ctx, "myapp")
		require.NoError(t, err)
		assert.Equal(t, original.ID, found.ID)

		cachedProjectID, cached := svc.composeNames.projectID("myapp").Get()
		require.True(t, cached)
		assert.Equal(t, original.ID, cachedProjectID)

		require.NoError(t, db.Model(&Project{}).Where("id = ?", original.ID).Updates(map[string]any{
			"name": "New Service",
			"path": "/tmp/new-service",
		}).Error)

		_, err = svc.GetProjectByComposeName(ctx, "myapp")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "project not found")

		_, cached = svc.composeNames.projectID("myapp").Get()
		assert.False(t, cached)
	})
}

func TestProjectService_PullProjectImages_UpdatesCurrentImageRecordAfterPull(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDB(t)

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	imageRef := "registry.example.com/team/app:1.2.3"
	repository := "registry.example.com/team/app"
	imageID := "sha256:team-app"
	imageDigest := digest.FromString("team-app-digest").String()
	buildRepository := "arcane.local/demo-builder/service"

	server := newImagePullServerWithObserver(t, map[string]dockertypesimage.InspectResponse{
		imageRef: {
			ID:          imageID,
			RepoTags:    []string{imageRef},
			RepoDigests: []string{repository + "@" + imageDigest},
		},
	}, nil)

	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	eventService := event.NewEventService(db, nil, nil)
	imageUpdateService := imageupdate.NewImageUpdateService(db, nil, nil, dockerService, nil, nil, nil)
	imageService := image.NewImageService(db, dockerService, nil, imageUpdateService, nil, eventService, nil, nil)
	svc := NewProjectService(db, settingsService, nil, imageService, dockerService, nil, nil, config.Load(), nil, nil)

	projectPath := createComposeProjectDir(t, projectsDir, "compose-pull")
	composeContent := fmt.Sprintf("services:\n  app:\n    image: %s\n  builder:\n    build: .\n", imageRef)
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte(composeContent), 0o644))

	dirName := "compose-pull"
	projectRecord := &Project{
		ID:      "project-pull",
		Name:    "compose-pull",
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(projectRecord).Error)

	now := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:old-full",
		Repository:     repository,
		Tag:            "1.2.3",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeDigest,
		CurrentVersion: "1.2.3",
		CheckTime:      now,
	}).Error)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:old-short",
		Repository:     "team/app",
		Tag:            "1.2.3",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeDigest,
		CurrentVersion: "1.2.3",
		CheckTime:      now.Add(time.Minute),
	}).Error)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:build-only",
		Repository:     buildRepository,
		Tag:            "latest",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeDigest,
		CurrentVersion: "latest",
		CheckTime:      now.Add(2 * time.Minute),
	}).Error)

	require.NoError(t, svc.PullProjectImages(ctx, projectRecord.ID, io.Discard, usertypes.SystemUser, nil))

	// sha256:old-* records represent update records for OTHER containers still running
	// the old image. Pulling the image for one container must not mark them as up-to-date
	// (#2453: updating one container was incorrectly removing others from the update list).
	var fullRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "sha256:old-full").First(&fullRecord).Error)
	assert.True(t, fullRecord.HasUpdate)

	var shortRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "sha256:old-short").First(&shortRecord).Error)
	assert.True(t, shortRecord.HasUpdate)

	var buildRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "sha256:build-only").First(&buildRecord).Error)
	assert.True(t, buildRecord.HasUpdate)

	// The newly pulled image itself is correctly marked as up-to-date.
	var currentRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", imageID).First(&currentRecord).Error)
	assert.False(t, currentRecord.HasUpdate)
	assert.Equal(t, repository, currentRecord.Repository)
	assert.Equal(t, "1.2.3", currentRecord.Tag)
	assert.Equal(t, imageDigest, mo.PointerToOption(currentRecord.CurrentDigest).OrEmpty())
	assert.Equal(t, imageDigest, mo.PointerToOption(currentRecord.LatestDigest).OrEmpty())
}

func TestProjectService_EnsureImagesPresent_UpdatesCurrentImageRecordAfterPull(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDB(t)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	imageRef := "registry.example.com/team/api:2.0.0"
	repository := "registry.example.com/team/api"
	imageID := "sha256:team-api"
	imageDigest := digest.FromString("team-api-digest").String()

	server := newImagePullServerWithObserver(t, map[string]dockertypesimage.InspectResponse{
		imageRef: {
			ID:          imageID,
			RepoTags:    []string{imageRef},
			RepoDigests: []string{repository + "@" + imageDigest},
		},
	}, nil)

	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	eventService := event.NewEventService(db, nil, nil)
	imageUpdateService := imageupdate.NewImageUpdateService(db, nil, nil, dockerService, nil, nil, nil)
	imageService := image.NewImageService(db, dockerService, nil, imageUpdateService, nil, eventService, nil, nil)
	svc := NewProjectService(db, settingsService, nil, imageService, dockerService, nil, nil, config.Load(), nil, nil)

	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:old-api",
		Repository:     repository,
		Tag:            "2.0.0",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeDigest,
		CurrentVersion: "2.0.0",
		CheckTime:      time.Now().UTC().Add(-time.Hour),
	}).Error)

	require.NoError(
		t,
		svc.composeCoordinator.EnsureImagesPresent(
			ctx,
			&types.Project{
				Services: types.Services{
					"api": {
						Image:      imageRef,
						PullPolicy: types.PullPolicyAlways,
					},
				},
			},
			io.Discard,
			svc.deployment.ImageOperations(
				nil,
				nil,
			),
		),
	)

	// sha256:old-api may still be in use by another container — pulling for one container
	// must not clear it (fixes #2453).
	var oldRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "sha256:old-api").First(&oldRecord).Error)
	assert.True(t, oldRecord.HasUpdate)

	var currentRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", imageID).First(&currentRecord).Error)
	assert.False(t, currentRecord.HasUpdate)
	assert.Equal(t, imageDigest, mo.PointerToOption(currentRecord.LatestDigest).OrEmpty())
}

func TestProjectService_PullImageForService_UpdatesCurrentImageRecordAfterPull(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDB(t)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	imageRef := "registry.example.com/team/worker:3.1.4"
	repository := "registry.example.com/team/worker"
	imageID := "sha256:team-worker"
	imageDigest := digest.FromString("team-worker-digest").String()

	server := newImagePullServerWithObserver(t, map[string]dockertypesimage.InspectResponse{
		imageRef: {
			ID:          imageID,
			RepoTags:    []string{imageRef},
			RepoDigests: []string{repository + "@" + imageDigest},
		},
	}, nil)

	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	eventService := event.NewEventService(db, nil, nil)
	imageUpdateService := imageupdate.NewImageUpdateService(db, nil, nil, dockerService, nil, nil, nil)
	imageService := image.NewImageService(db, dockerService, nil, imageUpdateService, nil, eventService, nil, nil)
	svc := NewProjectService(db, settingsService, nil, imageService, dockerService, nil, nil, config.Load(), nil, nil)

	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:old-worker",
		Repository:     repository,
		Tag:            "3.1.4",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeDigest,
		CurrentVersion: "3.1.4",
		CheckTime:      time.Now().UTC().Add(-time.Hour),
	}).Error)

	require.NoError(t, svc.deployment.Pull(ctx, imageRef, io.Discard, usertypes.SystemUser, nil))

	// sha256:old-worker may still be in use by another container — must not be cleared (fixes #2453).
	var oldRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "sha256:old-worker").First(&oldRecord).Error)
	assert.True(t, oldRecord.HasUpdate)

	var currentRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", imageID).First(&currentRecord).Error)
	assert.False(t, currentRecord.HasUpdate)
	assert.Equal(t, imageDigest, mo.PointerToOption(currentRecord.LatestDigest).OrEmpty())
}

func TestProjectService_ComposePullSelectedServices_ReconcilesOnlyOnSuccess(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDB(t)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	privateImageRef := "registry.example.com/team/app:9.9.9"
	privateRepository := "registry.example.com/team/app"
	privateImageID := "sha256:team-app-compose"
	privateImageDigest := digest.FromString("team-app-compose-digest").String()
	publicImageRef := "docker.io/library/nginx:1.27"
	publicRepository := "docker.io/library/nginx"
	publicImageID := "sha256:nginx-compose"
	publicImageDigest := digest.FromString("nginx-compose-digest").String()

	pullsByRef := map[string]int{}
	authHeadersByRef := map[string]string{}
	server := newImagePullServerWithObserver(t, map[string]dockertypesimage.InspectResponse{
		privateImageRef: {
			ID:          privateImageID,
			RepoTags:    []string{privateImageRef},
			RepoDigests: []string{privateRepository + "@" + privateImageDigest},
		},
		publicImageRef: {
			ID:          publicImageID,
			RepoTags:    []string{publicImageRef},
			RepoDigests: []string{publicRepository + "@" + publicImageDigest},
		},
	}, func(fullRef, authHeader string) {
		pullsByRef[fullRef]++
		authHeadersByRef[fullRef] = authHeader
	})

	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	eventService := event.NewEventService(db, nil, nil)
	imageUpdateService := imageupdate.NewImageUpdateService(db, nil, nil, dockerService, nil, nil, nil)
	require.NoError(t, db.AutoMigrate(&registry.ContainerRegistry{}))
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: strings.Repeat("a", 32)})
	registryService := registry.NewContainerRegistryService(db, nil, nil, nil)
	imageService := image.NewImageService(db, dockerService, registryService, imageUpdateService, nil, eventService, nil, nil)
	svc := NewProjectService(db, settingsService, nil, imageService, dockerService, nil, nil, config.Load(), nil, nil)

	projectDef := &types.Project{
		Name: "compose-selected",
		Services: types.Services{
			"app": {
				Name:  "app",
				Image: privateImageRef,
			},
			"app-copy": {
				Name:  "app-copy",
				Image: privateImageRef,
			},
			"sidecar": {
				Name:  "sidecar",
				Image: publicImageRef,
			},
			"builder": {
				Name:  "builder",
				Build: &types.BuildConfig{Context: "."},
			},
		},
	}

	now := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:selected-old",
		Repository:     privateRepository,
		Tag:            "9.9.9",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeDigest,
		CurrentVersion: "9.9.9",
		CheckTime:      now,
	}).Error)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ID:             "sha256:sidecar-old",
		Repository:     publicRepository,
		Tag:            "1.27",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeDigest,
		CurrentVersion: "1.27",
		CheckTime:      now,
	}).Error)

	credentials := []containerregistry.Credential{{
		URL:      "https://registry.example.com",
		Username: "arcane-user",
		Token:    "arcane-token",
		Enabled:  true,
	}}

	_, savedErr := registryService.CreateRegistry(ctx, containerregistry.CreateContainerRegistryRequest{URL: credentials[0].URL, Username: credentials[0].Username, Token: credentials[0].Token})
	require.NoError(t, savedErr)
	require.NoError(t, svc.composeCoordinator.PullServices(ctx, projectDef, []string{"app", "app-copy", "sidecar", "builder"}, svc.deployment.ImageOperations(nil, credentials), nil))
	assert.Equal(t, 1, pullsByRef[privateImageRef], "duplicate service refs should only be pulled once")
	assert.Equal(t, 1, pullsByRef[publicImageRef], "selected public image should still be pulled")
	assert.Len(t, pullsByRef, 2, "build-backed services should not trigger image pulls")

	privateAuth := decodeRegistryAuth(t, authHeadersByRef[privateImageRef])
	assert.Equal(t, "arcane-user", privateAuth.Username)
	assert.Equal(t, "arcane-token", privateAuth.Password)
	assert.Equal(t, "registry.example.com", privateAuth.ServerAddress)
	assert.Empty(t, authHeadersByRef[publicImageRef], "public image pull should not receive unrelated registry auth")

	// sha256:selected-old may still be used by another container — must not be cleared (fixes #2453).
	var selectedRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "sha256:selected-old").First(&selectedRecord).Error)
	assert.True(t, selectedRecord.HasUpdate)

	var sidecarRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "sha256:sidecar-old").First(&sidecarRecord).Error)
	assert.True(t, sidecarRecord.HasUpdate)

	var privateCurrentRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", privateImageID).First(&privateCurrentRecord).Error)
	assert.False(t, privateCurrentRecord.HasUpdate)
	assert.Equal(t, privateImageDigest, mo.PointerToOption(privateCurrentRecord.LatestDigest).OrEmpty())

	var publicCurrentRecord imageupdate.ImageUpdateRecord
	require.NoError(t, db.WithContext(ctx).Where("id = ?", publicImageID).First(&publicCurrentRecord).Error)
	assert.False(t, publicCurrentRecord.HasUpdate)
	assert.Equal(t, publicImageDigest, mo.PointerToOption(publicCurrentRecord.LatestDigest).OrEmpty())
}
