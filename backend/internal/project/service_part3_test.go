// Source: getarcaneapp/arcane/backend/internal/project/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture.

package project

import (
	"os"
	"path/filepath"

	"testing"

	"github.com/docker/compose/v5/pkg/api"

	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"

	"github.com/moby/moby/api/types/container"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"

	"github.com/florianfinn/docklet-next/backend/internal/event"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
)

func TestProjectService_UpdateProject_RenameFailsWhenTargetDirectoryExists(t *testing.T) {
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

	targetPath := filepath.Join(projectsDir, "bar")
	require.NoError(t, os.MkdirAll(targetPath, 0o755))

	project := &Project{
		ID:      "proj-2",
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
	assert.Contains(t, err.Error(), "project directory already exists")
	assert.DirExists(t, originalPath)
	assert.DirExists(t, targetPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	assert.Equal(t, "Foo", fromDB.Name)
	assert.Equal(t, originalPath, fromDB.Path)
	require.NotNil(t, fromDB.DirName)
	assert.Equal(t, "Foo", *fromDB.DirName)
}

func TestProjectService_UpdateProject_RenameFailsWhenProjectRunning(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	originalDirName := "Foo"
	originalPath := filepath.Join(projectsDir, originalDirName)
	require.NoError(t, os.MkdirAll(originalPath, 0o755))

	project := &Project{
		ID:      "proj-3",
		Name:    "Foo",
		DirName: &originalDirName,
		Path:    originalPath,
		Status:  ProjectStatusRunning,
	}
	require.NoError(t, db.Create(project).Error)

	_, err = svc.UpdateProject(ctx, project.ID, new("bar"), nil, nil, nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "project must be stopped before renaming (current status: running)")
	assert.DirExists(t, originalPath)
	assert.NoDirExists(t, filepath.Join(projectsDir, "bar"))

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	assert.Equal(t, "Foo", fromDB.Name)
	assert.Equal(t, originalPath, fromDB.Path)
	require.NotNil(t, fromDB.DirName)
	assert.Equal(t, "Foo", *fromDB.DirName)
}

func TestProjectService_UpdateProject_RenameRejectsStaleStoppedWhenRuntimeIsRunning(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)
	configureProjectRuntimeDocker(t, []container.Summary{
		{
			ID:     "app-container",
			Names:  []string{"/foo-app-1"},
			Image:  "nginx:alpine",
			State:  container.StateRunning,
			Status: "Up 30 seconds",
			Labels: map[string]string{
				api.ProjectLabel:    "foo",
				api.ServiceLabel:    "app",
				api.ConfigHashLabel: "app-hash",
				api.WorkingDirLabel: filepath.Join(projectsDir, "Foo"),
			},
		},
	})

	originalDirName := "Foo"
	originalPath := createComposeProjectDir(t, projectsDir, originalDirName)

	project := &Project{
		ID:      "proj-stale-stopped-running-rename",
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
	assert.Contains(t, err.Error(), "project must be stopped before renaming (current status: running)")
	assert.DirExists(t, originalPath)
	assert.NoDirExists(t, filepath.Join(projectsDir, "bar"))

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	assert.Equal(t, "Foo", fromDB.Name)
	assert.Equal(t, originalPath, fromDB.Path)
	assert.Equal(t, ProjectStatusStopped, fromDB.Status)
}

func TestProjectService_UpdateProject_RenameResolvesUnknownStoppedStatusBeforeVolumeMigration(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	server := newProjectRuntimeDockerServer(t, nil)
	t.Setenv("DOCKER_HOST", dockerHostFromProjectRuntimeServerURL(t, server.URL))

	originalDirName := "Foo"
	originalPath := createComposeProjectDir(t, projectsDir, originalDirName)
	statusReason := "stale runtime status"

	project := &Project{
		ID:           "proj-unknown-stopped-rename",
		Name:         "Foo",
		DirName:      &originalDirName,
		Path:         originalPath,
		Status:       ProjectStatusUnknown,
		StatusReason: &statusReason,
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
	assert.Equal(t, ProjectStatusStopped, updated.Status)
	assert.Nil(t, updated.StatusReason)
	assert.Equal(t, 1, updated.ServiceCount)
	assert.Equal(t, 0, updated.RunningCount)
	assert.NoDirExists(t, originalPath)
	assert.DirExists(t, expectedPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	assert.Equal(t, "bar", fromDB.Name)
	assert.Equal(t, expectedPath, fromDB.Path)
	assert.Equal(t, ProjectStatusStopped, fromDB.Status)
	assert.Nil(t, fromDB.StatusReason)
	assert.Equal(t, 1, fromDB.ServiceCount)
	assert.Equal(t, 0, fromDB.RunningCount)
}

func TestProjectService_UpdateProject_RenameRejectsUnknownWhenRuntimeIsRunning(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	server := newProjectRuntimeDockerServer(t, []container.Summary{
		{
			ID:     "app-container",
			Names:  []string{"/foo-app-1"},
			Image:  "nginx:alpine",
			State:  container.StateRunning,
			Status: "Up 30 seconds",
			Labels: map[string]string{
				api.ProjectLabel:    "foo",
				api.ServiceLabel:    "app",
				api.ConfigHashLabel: "app-hash",
				api.WorkingDirLabel: "/host/path/projects/Foo",
			},
		},
	})
	t.Setenv("DOCKER_HOST", dockerHostFromProjectRuntimeServerURL(t, server.URL))

	originalDirName := "Foo"
	originalPath := createComposeProjectDir(t, projectsDir, originalDirName)
	statusReason := "stale runtime status"

	project := &Project{
		ID:           "proj-unknown-running-rename",
		Name:         "Foo",
		DirName:      &originalDirName,
		Path:         originalPath,
		Status:       ProjectStatusUnknown,
		StatusReason: &statusReason,
	}
	require.NoError(t, db.Create(project).Error)

	_, err = svc.UpdateProject(ctx, project.ID, new("bar"), nil, nil, nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "project must be stopped before renaming (current status: running)")
	assert.DirExists(t, originalPath)
	assert.NoDirExists(t, filepath.Join(projectsDir, "bar"))

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	assert.Equal(t, "Foo", fromDB.Name)
	assert.Equal(t, originalPath, fromDB.Path)
	assert.Equal(t, ProjectStatusUnknown, fromDB.Status)
	require.NotNil(t, fromDB.StatusReason)
	assert.Equal(t, statusReason, *fromDB.StatusReason)
}

func TestProjectService_UpdateProject_ValidatesComposeUsingExistingProjectName(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	dirName := "demo"
	projectPath := filepath.Join(projectsDir, dirName)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))

	project := &Project{
		ID:      "proj-compose-name",
		Name:    "demo",
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	compose := `name: ${COMPOSE_PROJECT_NAME}
services:
  app:
    image: nginx:alpine
`
	env := "COMPOSE_PROJECT_NAME=\n"

	updated, err := svc.UpdateProject(ctx, project.ID, nil, new(compose), new(env), nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "demo", updated.Name)
}

func TestProjectService_UpdateProject_AllowsMissingEnvFileDuringComposeValidation(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	dirName := "env-required"
	projectPath := filepath.Join(projectsDir, dirName)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))

	project := &Project{
		ID:      "proj-env-file",
		Name:    "env-required",
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

	updated, err := svc.UpdateProject(ctx, project.ID, nil, new(compose), nil, nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.NoError(t, err)
	require.NotNil(t, updated)

	_, statErr := os.Stat(filepath.Join(projectPath, ".env"))
	require.NoError(t, statErr)
}

func TestProjectService_UpdateProject_EnvRetargetWritesExplicitIdenticalComposeToSelectedBase(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	dirName := "retarget-explicit"
	projectPath := filepath.Join(projectsDir, dirName)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))

	baseContent := "services:\n  base:\n    image: nginx:alpine\n"
	altContent := "services:\n  alt:\n    image: busybox:latest\n"
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte(baseContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "alt.yaml"), []byte(altContent), 0o644))

	project := &Project{
		ID:      "proj-retarget-explicit",
		Name:    dirName,
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	// An explicit composeContent is authoritative even when it is
	// byte-identical to the previous base: the caller intends the retargeted
	// base to carry this content.
	explicitCompose := baseContent
	_, err = svc.UpdateProject(ctx, project.ID, nil, &explicitCompose, new("COMPOSE_FILE=alt.yaml\n"), nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})
	require.NoError(t, err)

	altOnDisk, readErr := os.ReadFile(filepath.Join(projectPath, "alt.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, baseContent, string(altOnDisk), "an explicit submission must be written to the newly selected base")

	baseOnDisk, readErr := os.ReadFile(filepath.Join(projectPath, "compose.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, baseContent, string(baseOnDisk))

	envOnDisk, readErr := os.ReadFile(filepath.Join(projectPath, ".env"))
	require.NoError(t, readErr)
	assert.Contains(t, string(envOnDisk), "COMPOSE_FILE=alt.yaml")
}

func TestProjectService_UpdateProject_EnvRetargetWithoutComposePayloadPreservesNewBase(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	dirName := "retarget-omit"
	projectPath := filepath.Join(projectsDir, dirName)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))

	baseContent := "services:\n  base:\n    image: nginx:alpine\n"
	altContent := "services:\n  alt:\n    image: busybox:latest\n"
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte(baseContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "alt.yaml"), []byte(altContent), 0o644))

	project := &Project{
		ID:      "proj-retarget-omit",
		Name:    dirName,
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	// Clients omit composeContent when the compose editor is unchanged: an
	// env-only retarget must switch the selection without touching either file.
	_, err = svc.UpdateProject(ctx, project.ID, nil, nil, new("COMPOSE_FILE=alt.yaml\n"), nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})
	require.NoError(t, err)

	altOnDisk, readErr := os.ReadFile(filepath.Join(projectPath, "alt.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, altContent, string(altOnDisk), "the newly selected base must keep its own content")

	baseOnDisk, readErr := os.ReadFile(filepath.Join(projectPath, "compose.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, baseContent, string(baseOnDisk))

	envOnDisk, readErr := os.ReadFile(filepath.Join(projectPath, ".env"))
	require.NoError(t, readErr)
	assert.Contains(t, string(envOnDisk), "COMPOSE_FILE=alt.yaml")
}

func TestProjectService_UpdateProject_EnvRetargetWritesEditedComposeToSelectedBase(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	dirName := "retarget-edit"
	projectPath := filepath.Join(projectsDir, dirName)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))

	baseContent := "services:\n  base:\n    image: nginx:alpine\n"
	altContent := "services:\n  alt:\n    image: busybox:latest\n"
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte(baseContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "alt.yaml"), []byte(altContent), 0o644))

	project := &Project{
		ID:      "proj-retarget-edit",
		Name:    dirName,
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	editedCompose := "services:\n  alt:\n    image: redis:alpine\n"
	_, err = svc.UpdateProject(ctx, project.ID, nil, &editedCompose, new("COMPOSE_FILE=alt.yaml\n"), nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})
	require.NoError(t, err)

	altOnDisk, readErr := os.ReadFile(filepath.Join(projectPath, "alt.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, editedCompose, string(altOnDisk), "an edited compose must be written to the newly selected base")

	baseOnDisk, readErr := os.ReadFile(filepath.Join(projectPath, "compose.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, baseContent, string(baseOnDisk))
}

func TestProjectService_UpdateProject_AllowsMissingLocalIncludeDuringComposeValidation(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	dirName := "include-new"
	projectPath := filepath.Join(projectsDir, dirName)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))

	project := &Project{
		ID:      "proj-missing-include",
		Name:    "include-new",
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	compose := `include:
  - metadata.yaml
services:
  app:
    image: nginx:alpine
`

	updated, err := svc.UpdateProject(ctx, project.ID, nil, new(compose), nil, nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.NoError(t, err)
	require.NotNil(t, updated)

	includePath := filepath.Join(projectPath, "metadata.yaml")
	assert.NoFileExists(t, includePath)

	details, err := svc.GetProjectDetails(ctx, project.ID, projecttypes.AllDetails())
	require.NoError(t, err)
	require.Len(t, details.IncludeFiles, 1)
	assert.Equal(t, "metadata.yaml", details.IncludeFiles[0].RelativePath)
}

func TestProjectService_CreateProject_AllowsExternalInclude(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, os.WriteFile(filepath.Join(projectsDir, "shared.yaml"), []byte("services: {}\n"), 0o644))

	compose := `include:
  - ../shared.yaml
services:
  app:
    image: nginx:alpine
`

	project, err := svc.CreateProject(ctx, "with-external-include", compose, nil, projecttypes.CreateProjectWorkspaceManifest{}, nil, nil, nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})
	require.NoError(t, err)
	require.NotNil(t, project)
	assert.DirExists(t, filepath.Join(projectsDir, "with-external-include"))
	assert.FileExists(t, filepath.Join(projectsDir, "shared.yaml"))
}

func TestProjectService_UpdateProject_AllowsExternalInclude(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	dirName := "external-include"
	projectPath := filepath.Join(projectsDir, dirName)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectsDir, "shared.yaml"), []byte("services: {}\n"), 0o644))

	project := &Project{
		ID:      "proj-external-include",
		Name:    "external-include",
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	user := usertypes.Actor{ID: "u1", Username: "tester"}

	compose := `include:
  - ../shared.yaml
services:
  app:
    image: nginx:alpine
`
	updated, err := svc.UpdateProject(ctx, project.ID, nil, new(compose), nil, nil, user)
	require.NoError(t, err)
	require.NotNil(t, updated)

	override := `include:
  - ../shared.yaml
`
	_, err = svc.UpdateProject(ctx, project.ID, nil, nil, nil, new(override), user)
	require.NoError(t, err)
}

func TestProjectService_CreateProject_CommitsWorkspaceAndConfigurationTogether(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()
	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	svc := NewProjectService(db, settingsService, event.NewEventService(db, nil, nil), nil, nil, nil, nil, config.Load(), nil, nil)
	uploadIndex := 0
	manifest := projecttypes.CreateProjectWorkspaceManifest{FileChanges: []projecttypes.WorkspaceFileChange{{
		Operation: projecttypes.FileOpCreateFile, RelativePath: "config/app.txt", UploadIndex: &uploadIndex,
	}}}

	created, err := svc.CreateProject(
		ctx,
		"atomic-workspace",
		"services:\n  app:\n    image: nginx:alpine\n",
		new("VALUE=one\n"),
		manifest,
		map[int][]byte{0: []byte("workspace content\n")},
		nil,
		nil,
		usertypes.Actor{ID: "u1", Username: "tester"},
	)
	require.NoError(t, err)
	require.NotNil(t, created)
	require.FileExists(t, filepath.Join(created.Path, projects.DefaultComposeFileName))
	require.FileExists(t, filepath.Join(created.Path, ".env"))
	require.FileExists(t, filepath.Join(created.Path, "config", "app.txt"))
	content, err := os.ReadFile(filepath.Join(created.Path, "config", "app.txt"))
	require.NoError(t, err)
	require.Equal(t, "workspace content\n", string(content))
}

func TestProjectService_CreateProject_RollsBackInvalidWorkspaceManifest(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()
	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	svc := NewProjectService(db, settingsService, event.NewEventService(db, nil, nil), nil, nil, nil, nil, config.Load(), nil, nil)
	manifest := projecttypes.CreateProjectWorkspaceManifest{FileChanges: []projecttypes.WorkspaceFileChange{{
		Operation: projecttypes.FileOpDelete, RelativePath: "missing.txt",
	}}}

	created, err := svc.CreateProject(
		ctx,
		"invalid-workspace",
		"services:\n  app:\n    image: nginx:alpine\n",
		nil,
		manifest,
		nil,
		nil,
		nil,
		usertypes.Actor{ID: "u1", Username: "tester"},
	)
	require.Error(t, err)
	require.Nil(t, created)
	require.NoDirExists(t, filepath.Join(projectsDir, "invalid-workspace"))

	var count int64
	require.NoError(t, db.Model(&Project{}).Where("name = ?", "invalid-workspace").Count(&count).Error)
	require.Zero(t, count)
}

func TestProjectService_UpdateProject_UsesExistingEnvFileDuringComposeValidation(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	dirName := "env-existing"
	projectPath := filepath.Join(projectsDir, dirName)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, ".env"), []byte("FOO=bar\n"), 0o600))

	project := &Project{
		ID:      "proj-existing-env-file",
		Name:    "env-existing",
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

	updated, err := svc.UpdateProject(ctx, project.ID, nil, new(compose), nil, nil, usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})

	require.NoError(t, err)
	require.NotNil(t, updated)

	envBytes, readErr := os.ReadFile(filepath.Join(projectPath, ".env"))
	require.NoError(t, readErr)
	assert.Equal(t, "FOO=bar\n", string(envBytes))
}

// newProjectServiceForOverrideTest builds a project with a base compose
// file on disk and returns the service, the project record, and the project path.
func newProjectServiceForOverrideTest(t *testing.T, dirName string) (*ProjectService, *Project, string) {
	t.Helper()
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	eventService := event.NewEventService(db, nil, nil)
	svc := NewProjectService(db, settingsService, eventService, nil, nil, nil, nil, config.Load(), nil, nil)

	projectPath := filepath.Join(projectsDir, dirName)
	require.NoError(t, os.MkdirAll(projectPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte("services:\n  app:\n    image: nginx:alpine\n"), 0o600))

	project := &Project{
		ID:      "proj-" + dirName,
		Name:    dirName,
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	return svc, project, projectPath
}

func TestProjectService_UpdateProject_CreatesOverrideWithDefaultName(t *testing.T) {
	ctx := t.Context()
	svc, project, projectPath := newProjectServiceForOverrideTest(t, "override-create")

	override := "services:\n  app:\n    image: busybox:latest\n"
	_, err := svc.UpdateProject(ctx, project.ID, nil, nil, nil, new(override), usertypes.Actor{
		ID:       "u1",
		Username: "tester",
	})
	require.NoError(t, err)

	// A project with no override on disk gets the .yaml default, matching Arcane's
	// compose.yaml base default (not compose.override.yml, candidates[0]).
	overrideBytes, readErr := os.ReadFile(filepath.Join(projectPath, "compose.override.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, override, string(overrideBytes))

	details, err := svc.GetProjectDetails(ctx, project.ID, projecttypes.DetailsOptions{IncludeComposeContent: true})
	require.NoError(t, err)
	assert.Equal(t, "compose.override.yaml", details.OverrideFileName)
	assert.Equal(t, override, details.OverrideContent)
}
