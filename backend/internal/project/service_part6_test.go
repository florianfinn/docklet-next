// Source: getarcaneapp/arcane/backend/internal/project/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture.

package project

import (
	"encoding/json"
	"errors"

	"io"

	"net/http"
	"net/http/httptest"

	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"

	"github.com/moby/moby/api/types/container"

	"github.com/moby/moby/api/types/volume"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/config"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"

	"github.com/florianfinn/docklet-next/backend/internal/kv"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumes"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
)

func TestProjectService_UpdateProject_WritesThroughSymlinkedProjectPath(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	targetRoot := t.TempDir()
	targetPath := createComposeProjectDir(t, targetRoot, "demo-target")
	linkPath := filepath.Join(projectsRoot, "demo")
	require.NoError(t, os.Symlink(targetPath, linkPath))

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))
	require.NoError(t, settingsService.SetStringSetting(ctx, "followProjectSymlinks", "true"))

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	project := &Project{
		ID:      "proj-symlink-update",
		Name:    "demo",
		DirName: new("demo"),
		Path:    linkPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	updatedCompose := "services:\n  app:\n    image: nginx:1.27-alpine\n"
	updatedEnv := "FOO=updated\n"

	updated, err := svc.UpdateProject(ctx, project.ID, nil, new(updatedCompose), new(updatedEnv), nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, linkPath, updated.Path)

	composeBytes, readErr := os.ReadFile(filepath.Join(targetPath, "compose.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, updatedCompose, string(composeBytes))

	envBytes, readErr := os.ReadFile(filepath.Join(targetPath, ".env"))
	require.NoError(t, readErr)
	assert.Equal(t, updatedEnv, string(envBytes))
}

func TestProjectService_UpdateProject_WritesThroughExternalEnvSymlink(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))
	projectPath := createComposeProjectDir(t, projectsRoot, "demo")
	targetPath := filepath.Join(t.TempDir(), "project.env")
	targetPerm := os.FileMode(0o640)
	require.NoError(t, os.WriteFile(targetPath, []byte("FOO=original\n"), targetPerm))
	require.NoError(t, os.Chmod(targetPath, targetPerm))

	envPath := filepath.Join(projectPath, projects.EffectiveEnvFileName)
	if symlinkErr := os.Symlink(targetPath, envPath); symlinkErr != nil {
		t.Skipf("symlink creation is unavailable: %v", symlinkErr)
	}
	originalLinkTarget, err := os.Readlink(envPath)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)
	dirName := "demo"
	project := &Project{
		ID:      "proj-external-env-symlink-update",
		Name:    dirName,
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	updatedEnv := "FOO=updated\n"
	updated, err := svc.UpdateProject(ctx, project.ID, nil, nil, &updatedEnv, nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	targetContent, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Equal(t, updatedEnv, string(targetContent))
	currentLinkTarget, err := os.Readlink(envPath)
	require.NoError(t, err)
	assert.Equal(t, originalLinkTarget, currentLinkTarget)
	linkInfo, err := os.Lstat(envPath)
	require.NoError(t, err)
	require.NotZero(t, linkInfo.Mode()&os.ModeSymlink)
	if runtime.GOOS != "windows" {
		targetInfo, statErr := os.Stat(targetPath)
		require.NoError(t, statErr)
		assert.Equal(t, targetPerm, targetInfo.Mode().Perm())
	}
}

func TestProjectService_UpdateProject_RestoresExternalEnvSymlinkTargetWhenProjectSaveFails(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))
	projectPath := createComposeProjectDir(t, projectsRoot, "demo")
	targetPath := filepath.Join(t.TempDir(), "project.env")
	originalContent := "FOO=original\n"
	targetPerm := os.FileMode(0o640)
	require.NoError(t, os.WriteFile(targetPath, []byte(originalContent), targetPerm))
	require.NoError(t, os.Chmod(targetPath, targetPerm))

	envPath := filepath.Join(projectPath, projects.EffectiveEnvFileName)
	if symlinkErr := os.Symlink(targetPath, envPath); symlinkErr != nil {
		t.Skipf("symlink creation is unavailable: %v", symlinkErr)
	}
	originalLinkTarget, err := os.Readlink(envPath)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)
	dirName := "demo"
	project := &Project{
		ID:      "proj-external-env-symlink-rollback",
		Name:    dirName,
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("arcane_test_external_env_project_save_failure", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Project" {
			_ = tx.AddError(errors.New("forced project save failure"))
		}
	}))

	updatedEnv := "FOO=updated\n"
	_, err = svc.UpdateProject(ctx, project.ID, nil, nil, &updatedEnv, nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced project save failure")

	targetContent, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(targetContent))
	currentLinkTarget, err := os.Readlink(envPath)
	require.NoError(t, err)
	assert.Equal(t, originalLinkTarget, currentLinkTarget)
	linkInfo, err := os.Lstat(envPath)
	require.NoError(t, err)
	require.NotZero(t, linkInfo.Mode()&os.ModeSymlink)
	if runtime.GOOS != "windows" {
		targetInfo, statErr := os.Stat(targetPath)
		require.NoError(t, statErr)
		assert.Equal(t, targetPerm, targetInfo.Mode().Perm())
	}
}

func createComposeProjectDir(t *testing.T, root, name string) string {
	t.Helper()

	projectPath := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte("services:\n  app:\n    image: nginx:alpine\n"), 0o644))

	return projectPath
}

func configureProjectRuntimeDocker(t *testing.T, containers []container.Summary) {
	t.Helper()

	server := newProjectRuntimeDockerServer(t, containers)
	t.Setenv("DOCKER_HOST", dockerHostFromProjectRuntimeServerURL(t, server.URL))
}

func projectUpdatePreviewDirs(t *testing.T, projectsDir string) []string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(projectsDir, ".project-update-preview-*"))
	require.NoError(t, err)
	return matches
}

func newProjectRuntimeDockerServer(t *testing.T, containers []container.Summary) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			_, _ = io.WriteString(w, "OK")
		case strings.HasSuffix(r.URL.Path, "/version"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"ApiVersion":    "1.41",
				"MinAPIVersion": "1.24",
				"Version":       "24.0.0",
			})
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(containers)
		case strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json"):
			containerID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path[strings.LastIndex(r.URL.Path, "/containers/"):], "/containers/"), "/json")
			for _, c := range containers {
				if c.ID != containerID {
					continue
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(container.InspectResponse{
					ID: c.ID,
					State: &container.State{
						Status:  c.State,
						Running: c.State == container.StateRunning,
					},
				})
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func dockerHostFromProjectRuntimeServerURL(t *testing.T, serverURL string) string {
	t.Helper()

	parsed, err := url.Parse(serverURL)
	require.NoError(t, err)
	return "tcp://" + parsed.Host
}

func TestProjectService_RecoverProjectRenameJournals_RollsBackUncommittedDirectoryRename(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, oldDir)
	newPath := filepath.Join(projectsDir, newDir)
	require.NoError(t, os.MkdirAll(newPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(newPath, "compose.yaml"), []byte("services: {}\n"), 0o600))

	project := &Project{
		ID:      "proj-rename-recovery",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    oldPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseTargetsCopied,
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	require.NoError(t, svc.RecoverProjectRenameJournals(ctx))

	require.FileExists(t, filepath.Join(oldPath, "compose.yaml"))
	require.NoDirExists(t, newPath)
	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "nginx", fromDB.Name)
	require.Equal(t, oldPath, fromDB.Path)
	require.NotNil(t, fromDB.DirName)
	require.Equal(t, oldDir, *fromDB.DirName)
}

func TestProjectService_RecoverProjectRenameJournals_StartedPhaseSkipsVolumeRollback(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, oldDir)
	newPath := filepath.Join(projectsDir, newDir)
	require.NoError(t, os.MkdirAll(oldPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(oldPath, "compose.yaml"), []byte("services: {}\n"), 0o600))

	project := &Project{
		ID:      "proj-rename-started-volume-recovery",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    oldPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseStarted,
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

	require.NoError(t, svc.RecoverProjectRenameJournals(ctx))

	require.FileExists(t, filepath.Join(oldPath, "compose.yaml"))
	require.NoDirExists(t, newPath)
	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "nginx", fromDB.Name)
	require.Equal(t, oldPath, fromDB.Path)
	require.NotNil(t, fromDB.DirName)
	require.Equal(t, oldDir, *fromDB.DirName)
}

func TestProjectService_RecoverProjectRenameJournals_RelocatesTargetWhenBothPathsExist(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, oldDir)
	newPath := filepath.Join(projectsDir, newDir)
	require.NoError(t, os.MkdirAll(oldPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(oldPath, "compose.yaml"), []byte("services: {}\n"), 0o600))
	require.NoError(t, os.MkdirAll(newPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(newPath, "compose.yaml"), []byte("services: {}\n"), 0o600))

	project := &Project{
		ID:      "proj-rename-both-paths",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    oldPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseStarted,
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	require.NoError(t, svc.RecoverProjectRenameJournals(ctx))

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
	require.FileExists(t, filepath.Join(oldPath, "compose.yaml"))
	require.NoDirExists(t, newPath)

	conflictPaths, err := filepath.Glob(filepath.Join(projectsDir, ".web.rename-conflict-*"))
	require.NoError(t, err)
	require.Len(t, conflictPaths, 1)
	require.FileExists(t, filepath.Join(conflictPaths[0], "compose.yaml"))

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "nginx", fromDB.Name)
	require.Equal(t, oldPath, fromDB.Path)
	require.NotNil(t, fromDB.DirName)
	require.Equal(t, oldDir, *fromDB.DirName)

	newName := "web"
	require.NoError(t, svc.applyProjectRename(t.Context(), &fromDB, newName, projectsDir))
	require.Equal(t, "web", fromDB.Name)
	require.Equal(t, newPath, fromDB.Path)
	require.DirExists(t, newPath)
}

func TestProjectService_RecoverProjectRenameJournals_ClearsStartedJournalWhenDirectoryPathsMissing(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, oldDir)
	newPath := filepath.Join(projectsDir, newDir)

	project := &Project{
		ID:      "proj-rename-missing-paths",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    oldPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseStarted,
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	require.NoError(t, svc.RecoverProjectRenameJournals(ctx))

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoDirExists(t, oldPath)
	require.NoDirExists(t, newPath)
}

func TestProjectService_RecoverProjectRenameJournals_ClearsPreservedTargetJournalWhenPathExists(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, oldDir)
	newPath := filepath.Join(projectsDir, newDir)
	require.NoError(t, os.MkdirAll(oldPath, 0o755))

	project := &Project{
		ID:      "proj-rename-preserved-target",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    oldPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	var targetRemoved bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Name": "web_data"})) {
				return
			}
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			targetRemoved = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	kvService := kv.NewKVService(db)
	svc := NewProjectService(db, nil, nil, nil, &docker.DockerClientService{Client: newTestDockerClient(t, server)}, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
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

	require.NoError(t, svc.RecoverProjectRenameJournals(ctx))

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
	require.False(t, targetRemoved, "preserved target volume should remain for manual inspection")
	require.DirExists(t, oldPath)
}

func TestProjectService_RecoverProjectRenameJournals_ClearsCommittedJournal(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, oldDir)
	newPath := filepath.Join(projectsDir, newDir)
	require.NoError(t, os.MkdirAll(newPath, 0o755))

	project := &Project{
		ID:      "proj-rename-committed",
		Name:    "web",
		DirName: &newDir,
		Path:    newPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseOldVolumesRemoved,
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	require.NoError(t, svc.RecoverProjectRenameJournals(ctx))

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
	require.DirExists(t, newPath)
}

func TestProjectService_FinalizeProjectRenameAfterCommit_ClearsJournalAfterSourceCleanup(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	oldDir := "nginx"
	newDir := "web"
	project := &Project{
		ID:      "proj-rename-old-volumes-removed",
		Name:    "web",
		DirName: &newDir,
		Path:    filepath.Join(t.TempDir(), newDir),
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), kvService, nil)
	journal := &projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    filepath.Join(t.TempDir(), oldDir),
		NewPath:    project.Path,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseTargetsCopied,
	}
	require.NoError(t, svc.updates.WriteJournal(ctx, journal, projecttypes.RenameJournalPhaseTargetsCopied))

	migration := &fakeProjectVolumeRenameMigration{}
	journalActive := true
	projects.FinalizeRenameAfterCommit(ctx, svc.updates.Operations(), project.ID, migration, journal, &journalActive)
	require.True(t, migration.commitCalled)
	require.False(t, journalActive)

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestProjectService_FinalizeProjectRenameAfterCommit_KeepsJournalWhenSourceCleanupFails(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	oldDir := "nginx"
	newDir := "web"
	project := &Project{
		ID:      "proj-rename-cleanup-failure",
		Name:    "web",
		DirName: &newDir,
		Path:    filepath.Join(t.TempDir(), newDir),
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), kvService, nil)
	journal := &projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    filepath.Join(t.TempDir(), oldDir),
		NewPath:    project.Path,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseTargetsCopied,
	}
	require.NoError(t, svc.updates.WriteJournal(ctx, journal, projecttypes.RenameJournalPhaseTargetsCopied))

	migration := &fakeProjectVolumeRenameMigration{
		commitErr: volumes.NewSourceCleanupError("nginx_data", errors.New("source cleanup failed")),
	}
	journalActive := true
	projects.FinalizeRenameAfterCommit(ctx, svc.updates.Operations(), project.ID, migration, journal, &journalActive)
	require.True(t, migration.commitCalled)
	require.True(t, journalActive)

	raw, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.True(t, ok)

	var updatedJournal projecttypes.RenameJournal
	require.NoError(t, json.Unmarshal([]byte(raw), &updatedJournal))
	require.Equal(t, projecttypes.RenameJournalPhaseSourceCleanupPending, updatedJournal.Phase)
}

func TestProjectService_RecoverProjectRenameJournals_KeepsJournalWhenDirectoryRollbackFails(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	var targetRemoved atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "nginx_data"})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			if targetRemoved.Load() {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "web_data"})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			targetRemoved.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, "missing-parent", oldDir)
	newPath := filepath.Join(projectsDir, newDir)
	require.NoError(t, os.MkdirAll(newPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(newPath, "compose.yaml"), []byte("services: {}\n"), 0o600))

	project := &Project{
		ID:      "proj-rename-directory-rollback-fails",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    oldPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	svc := NewProjectService(db, nil, nil, nil, dockerService, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
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

	err = svc.RecoverProjectRenameJournals(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "rollback project directory rename")

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, targetRemoved.Load(), "target volume rollback should still run after directory rollback fails")
	require.FileExists(t, filepath.Join(newPath, "compose.yaml"), "failed directory rollback leaves the target path for retry")
	require.NoDirExists(t, oldPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "nginx", fromDB.Name)
	require.Equal(t, oldPath, fromDB.Path)
	require.NotNil(t, fromDB.DirName)
	require.Equal(t, oldDir, *fromDB.DirName)

	require.NoError(t, os.MkdirAll(filepath.Dir(oldPath), 0o755))
	require.NoError(t, svc.RecoverProjectRenameJournals(ctx))

	_, ok, err = kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
	require.FileExists(t, filepath.Join(oldPath, "compose.yaml"))
	require.NoDirExists(t, newPath)
}
