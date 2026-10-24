// Source: getarcaneapp/arcane/backend/internal/project/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture.

package project

import (
	"context"

	"errors"
	"fmt"

	"os"
	"path/filepath"
	"runtime"

	"testing"

	"github.com/docker/compose/v5/pkg/api"

	usertypes "github.com/florianfinn/docklet-next/types/user"

	"github.com/moby/moby/api/types/container"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/config"

	"github.com/florianfinn/docklet-next/backend/internal/project/children/listing"

	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
)

func TestProjectService_ArchiveProject_RequiresStoppedProject(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsRoot := t.TempDir()
	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	projectPath := createComposeProjectDir(t, projectsRoot, "running-demo")
	// DB row says stopped; the live Docker state below is what must block the archive.
	require.NoError(t, db.Create(&Project{
		ID:      "project-running",
		Name:    "running-demo",
		DirName: new("running-demo"),
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}).Error)

	configureProjectRuntimeDocker(t, []container.Summary{
		{
			ID:     "app-container",
			Names:  []string{"/running-demo-app-1"},
			Image:  "nginx:alpine",
			State:  container.StateRunning,
			Status: "Up 30 seconds",
			Labels: map[string]string{
				api.ProjectLabel:    "running-demo",
				api.ServiceLabel:    "app",
				api.ConfigHashLabel: "app-hash",
				api.WorkingDirLabel: "/host/path/projects/running-demo",
			},
		},
	})

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	err = svc.ArchiveProject(ctx, "project-running", usertypes.Actor{ID: "user-1", Username: "tester"})
	require.Error(t, err)
	require.ErrorIs(t, err, common.ErrProjectMustBeStopped)

	var stored Project
	require.NoError(t, db.First(&stored, "id = ?", "project-running").Error)
	assert.False(t, stored.IsArchived)
	assert.Nil(t, stored.ArchivedAt)
}

func TestProjectService_ArchiveProject_TogglesArchiveFlag(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsRoot := t.TempDir()
	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	projectPath := createComposeProjectDir(t, projectsRoot, "stopped-demo")
	require.NoError(t, db.Create(&Project{
		ID:      "project-stopped",
		Name:    "stopped-demo",
		DirName: new("stopped-demo"),
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}).Error)

	configureProjectRuntimeDocker(t, nil)

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	user := usertypes.Actor{ID: "user-1", Username: "tester"}

	require.NoError(t, svc.ArchiveProject(ctx, "project-stopped", user))
	var stored Project
	require.NoError(t, db.First(&stored, "id = ?", "project-stopped").Error)
	assert.True(t, stored.IsArchived)
	assert.NotNil(t, stored.ArchivedAt)

	require.NoError(t, svc.UnarchiveProject(ctx, "project-stopped", user))
	var unarchived Project
	require.NoError(t, db.First(&unarchived, "id = ?", "project-stopped").Error)
	assert.False(t, unarchived.IsArchived)
	assert.Nil(t, unarchived.ArchivedAt)
}

func TestProjectService_ArchiveProject_LiveVerificationErrorPolicy(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsRoot := t.TempDir()
	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	unreachablePath := createComposeProjectDir(t, projectsRoot, "unreachable-demo")
	require.NoError(t, db.Create(&Project{
		ID:      "project-unreachable",
		Name:    "unreachable-demo",
		DirName: new("unreachable-demo"),
		Path:    unreachablePath,
		Status:  ProjectStatusStopped,
	}).Error)

	noComposePath := filepath.Join(projectsRoot, "no-compose-demo")
	require.NoError(t, os.MkdirAll(noComposePath, 0o755))
	require.NoError(t, db.Create(&Project{
		ID:      "project-no-compose",
		Name:    "no-compose-demo",
		DirName: new("no-compose-demo"),
		Path:    noComposePath,
		Status:  ProjectStatusStopped,
	}).Error)

	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:1")

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	user := usertypes.Actor{ID: "user-1", Username: "tester"}

	err = svc.ArchiveProject(ctx, "project-unreachable", user)
	require.Error(t, err)
	var stored Project
	require.NoError(t, db.First(&stored, "id = ?", "project-unreachable").Error)
	assert.False(t, stored.IsArchived)

	require.NoError(t, svc.ArchiveProject(ctx, "project-no-compose", user))
	var archived Project
	require.NoError(t, db.First(&archived, "id = ?", "project-no-compose").Error)
	assert.True(t, archived.IsArchived)
}

func TestProjectService_ProjectListRows_PersistsInferredServiceCount(t *testing.T) {
	db := setupProjectTestDB(t)
	projectsDir := t.TempDir()
	projectPath := filepath.Join(projectsDir, "inferred")
	require.NoError(t, os.MkdirAll(projectPath, 0o755))
	inferred := Project{ID: "inferred", Name: "inferred", Path: projectPath, ServiceCount: 0}
	known := Project{ID: "known", Name: "known", Path: filepath.Join(projectsDir, "known"), ServiceCount: 4}
	require.NoError(t, db.Create(&inferred).Error)
	require.NoError(t, db.Create(&known).Error)
	service := &ProjectService{db: db}
	service.initChildren(nil, nil)

	containers := []container.Summary{
		{ID: "c1", State: "running", Names: []string{"/inferred-web"}, Labels: map[string]string{
			api.ProjectLabel: "inferred", api.ServiceLabel: "web", api.WorkingDirLabel: projectPath,
		}},
		{ID: "c2", State: "exited", Names: []string{"/inferred-db"}, Labels: map[string]string{
			api.ProjectLabel: "inferred", api.ServiceLabel: "db", api.WorkingDirLabel: projectPath,
		}},
		{ID: "c3", State: "running", Names: []string{"/known-web"}, Labels: map[string]string{
			api.ProjectLabel: "known", api.ServiceLabel: "web", api.WorkingDirLabel: known.Path,
		}},
	}
	service.listing = listing.New(func(context.Context) ([]container.Summary, error) { return containers, nil }, nil, nil)
	snapshot := service.listing.Snapshot(t.Context())

	rows := service.projectListRows(t.Context(), projectsDir, []Project{inferred, known}, snapshot)
	require.Len(t, rows, 2)
	require.Equal(t, 2, rows[0].ServiceCount)
	require.Equal(t, 4, rows[1].ServiceCount)

	// The inferred count is persisted because the plain list path sorts and
	// paginates on this column in SQL.
	var stored Project
	require.NoError(t, db.First(&stored, "id = ?", "inferred").Error)
	require.Equal(t, 2, stored.ServiceCount)

	var storedKnown Project
	require.NoError(t, db.First(&storedKnown, "id = ?", "known").Error)
	require.Equal(t, 4, storedKnown.ServiceCount, "a persisted non-zero count must not be overwritten by the live container count")
}

func TestProjectService_ListProjects_WithDerivedStatusFilter_AllowsAllPageSizeSentinel(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	for i := range 25 {
		projectPath := createComposeProjectDir(t, projectsRoot, fmt.Sprintf("stopped-%02d", i))
		require.NoError(t, db.Create(&Project{
			ID:      fmt.Sprintf("project-%02d", i),
			Name:    fmt.Sprintf("stopped-%02d", i),
			DirName: new(fmt.Sprintf("stopped-%02d", i)),
			Path:    projectPath,
			Status:  ProjectStatusStopped,
		}).Error)
	}

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	configureProjectRuntimeDocker(t, nil)

	items, page, err := svc.ListProjects(ctx, pagination.QueryParams{
		Filters: map[string]string{
			"status": string(ProjectStatusStopped),
		},
		Limit: -1,
		Sort:  "name", Order: pagination.SortAsc,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 25, page.TotalItems)
	require.Len(t, items, 25)
	assert.Equal(t, "stopped-00", items[0].Name)
	assert.Equal(t, "stopped-24", items[len(items)-1].Name)
}

func TestProjectService_DeployProject_StopsOnBuildPreparationError(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()
	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	projectDir := filepath.Join(projectsRoot, "demo")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	composeContent := "services:\n  web:\n    pull_policy: build\n    build:\n      context: .\n"

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte(composeContent), 0o644))
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot+":"+projectsRoot))

	proj := &Project{
		ID:     "p1",
		Name:   "demo",
		Path:   projectDir,
		Status: ProjectStatusStopped,
	}
	require.NoError(t, db.Create(proj).Error)

	buildSvc := testBuildBuilder{err: errors.New("boom build")}
	svc := NewProjectService(db, settingsService, nil, nil, nil, buildSvc, nil, config.Load(), nil, nil)

	err = svc.DeployProject(ctx, "p1", usertypes.Actor{ID: "u1", Username: "tester"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to prepare project images for deploy")
	assert.Contains(t, err.Error(), "boom build")

	var updated Project
	require.NoError(t, db.First(&updated, "id = ?", "p1").Error)
	assert.Equal(t, ProjectStatusStopped, updated.Status)
}

func TestProjectService_DeployProject_BuildsGeneratedImageWithoutPull(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()
	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	projectDir := filepath.Join(projectsRoot, "demo")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))
	composeContent := "services:\n  caddy:\n    build:\n      dockerfile_inline: |\n        FROM caddy:builder AS builder\n     " +
		"   RUN xcaddy build --with github.com/caddyserver/replace-response\n\n        FROM caddy:latest\n      " +
		"  COPY --from=builder /usr/bin/caddy /usr/bin/caddy\n"

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte(composeContent), 0o644))
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot+":"+projectsRoot))

	proj := &Project{
		ID:     "p-generated",
		Name:   "build-test",
		Path:   projectDir,
		Status: ProjectStatusStopped,
	}
	require.NoError(t, db.Create(proj).Error)

	buildSvc := testBuildBuilder{err: errors.New("boom build")}
	svc := NewProjectService(db, settingsService, nil, nil, nil, buildSvc, nil, config.Load(), nil, nil)

	err = svc.DeployProject(ctx, proj.ID, usertypes.Actor{ID: "u1", Username: "tester"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to prepare project images for deploy")
	assert.Contains(t, err.Error(), "boom build")
	assert.NotContains(t, err.Error(), "failed to pull image arcane.local/")
	assert.NotContains(t, err.Error(), "failed to resolve reference \"arcane.local/")
}

func TestProjectService_SyncProjectsFromFileSystem_IgnoresSymlinkedProjectDirsWhenDisabled(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	targetRoot := t.TempDir()
	createComposeProjectDir(t, projectsRoot, "regular")
	linkTarget := createComposeProjectDir(t, targetRoot, "linked-target")
	require.NoError(t, os.Symlink(linkTarget, filepath.Join(projectsRoot, "linked")))

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))
	require.NoError(t, settingsService.SetStringSetting(ctx, "followProjectSymlinks", "false"))

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	items, err := svc.ListAllProjects(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "regular", items[0].Name)
	assert.Equal(t, filepath.Join(projectsRoot, "regular"), items[0].Path)
}

func TestProjectService_SyncProjectsFromFileSystem_DetectsSymlinkedProjectDirsWhenEnabled(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	targetRoot := t.TempDir()
	linkTarget := createComposeProjectDir(t, targetRoot, "linked-target")
	linkPath := filepath.Join(projectsRoot, "linked")
	require.NoError(t, os.Symlink(linkTarget, linkPath))

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))
	require.NoError(t, settingsService.SetStringSetting(ctx, "followProjectSymlinks", "true"))

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	items, err := svc.ListAllProjects(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "linked", items[0].Name)
	assert.Equal(t, linkPath, items[0].Path)
}

func TestProjectService_SyncProjectsFromFileSystem_DiscoversNestedProjectsAndRelativePaths(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	nestedPath := createComposeProjectDir(t, projectsRoot, filepath.Join("main-project", "sub-project1"))
	topLevelPath := createComposeProjectDir(t, projectsRoot, "project2")

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	items, page, err := svc.ListProjects(ctx, pagination.QueryParams{
		Sort: "path", Order: pagination.SortAsc,
		Limit: -1,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, page.TotalItems)
	require.Len(t, items, 2)

	assert.Equal(t, "main-project/sub-project1", items[0].RelativePath)
	assert.Equal(t, nestedPath, items[0].Path)
	assert.Equal(t, "sub-project1", items[0].DirName)

	assert.Equal(t, "project2", items[1].RelativePath)
	assert.Equal(t, topLevelPath, items[1].Path)
	assert.Equal(t, "project2", items[1].DirName)
}

func TestProjectService_SyncProjectsFromFileSystem_RespectsConfiguredScanMaxDepth(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	topLevelPath := createComposeProjectDir(t, projectsRoot, "project1")
	createComposeProjectDir(t, projectsRoot, filepath.Join("group", "project2"))

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))
	t.Setenv("PROJECT_SCAN_MAX_DEPTH", "1")

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	items, err := svc.ListAllProjects(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "project1", items[0].Name)
	assert.Equal(t, topLevelPath, items[0].Path)
}

func TestProjectService_SyncProjectsFromFileSystem_RemovesDeletedNestedProject(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	projectPath := createComposeProjectDir(t, projectsRoot, filepath.Join("main-project", "sub-project1"))

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	items, err := svc.ListAllProjects(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)

	require.NoError(t, os.Remove(filepath.Join(projectPath, "compose.yaml")))
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	items, err = svc.ListAllProjects(ctx)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestProjectService_SyncProjectsFromFileSystem_PreservesProjectsWhenDirectoryEmptyOrUnmounted(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	// An existing-but-empty projects directory simulates a mis-mapped or unmounted
	// projects volume: GetProjectsDirectory resolves (and MkdirAll's) it, discovery
	// finds nothing, and every stored project path is now missing on disk.
	projectsRoot := t.TempDir()
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	// Seed a small deployment's worth of filesystem-managed projects whose
	// directories do not exist. Each would individually be pruned as "directory no
	// longer exists"; together they would wipe the table — exactly what the
	// mass-wipe guard must prevent, even for deployments with only a handful of
	// projects (the guard must not give small installs a free pass).
	const seeded = 3
	for i := range seeded {
		require.NoError(t, db.WithContext(ctx).Create(&Project{
			Name:   fmt.Sprintf("project-%d", i),
			Path:   filepath.Join(projectsRoot, fmt.Sprintf("project-%d", i)),
			Status: ProjectStatusStopped,
		}).Error)
	}

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	items, err := svc.ListAllProjects(ctx)
	require.NoError(t, err)
	assert.Len(t, items, seeded, "an empty/mis-mapped projects directory must not wipe existing project records")
}

func TestProjectService_SyncProjectsFromFileSystem_PreservesProjectWithAmbiguousCustomCompose(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	projectPath := createComposeProjectDir(t, projectsRoot, "ambiguous-project")

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	items, err := svc.ListAllProjects(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)

	// Replace the standard compose.yaml with two custom-named compose files. The
	// directory still holds compose content, but DetectComposeFile can't pick one
	// and returns common.ErrComposeFileNotFound. The reconcile must NOT delete the
	// record: the project's files are intact on disk and it may be deployable.
	require.NoError(t, os.Remove(filepath.Join(projectPath, "compose.yaml")))
	composeBody := []byte("services:\n  app:\n    image: nginx:alpine\n")
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "app.yaml"), composeBody, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "extra.yaml"), composeBody, 0o644))

	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	items, err = svc.ListAllProjects(ctx)
	require.NoError(t, err)
	assert.Len(t, items, 1, "project with ambiguous compose files must not be deleted")
	assert.Equal(t, projectPath, items[0].Path)
}

func TestProjectService_SyncProjectsFromFileSystem_RemovesProjectsBeyondReducedScanMaxDepth(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	topLevelPath := createComposeProjectDir(t, projectsRoot, "project1")
	nestedPath := createComposeProjectDir(t, projectsRoot, filepath.Join("group", "project2"))

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	// Initial sync at the default scan depth discovers both the top-level and
	// the nested project, persisting them to the database.
	defaultSvc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, defaultSvc.SyncProjectsFromFileSystem(ctx))

	items, err := defaultSvc.ListAllProjects(ctx)
	require.NoError(t, err)
	require.Len(t, items, 2)

	// Lowering the scan depth must prune the nested project from the database on
	// the next sync, even though its compose file still exists on disk.
	t.Setenv("PROJECT_SCAN_MAX_DEPTH", "1")
	depthLimitedSvc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, depthLimitedSvc.SyncProjectsFromFileSystem(ctx))

	items, err = depthLimitedSvc.ListAllProjects(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "project1", items[0].Name)
	assert.Equal(t, topLevelPath, items[0].Path)

	// The pruned project's files must remain untouched on disk so raising the
	// depth again re-discovers it.
	assert.FileExists(t, filepath.Join(nestedPath, "compose.yaml"))
}

func TestProjectService_SyncProjectsFromFileSystem_PreservesDBRecordsWhenDirectoryUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission-denied behavior is not portable to Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires a non-root UID to trigger permission-denied on ReadDir")
	}

	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	createComposeProjectDir(t, projectsRoot, "project1")

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	before, err := svc.ListAllProjects(ctx)
	require.NoError(t, err)
	require.Len(t, before, 1)

	unreadable := t.TempDir()
	require.NoError(t, os.Chmod(unreadable, 0))
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o700) })

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", unreadable))

	syncErr := svc.SyncProjectsFromFileSystem(ctx)
	require.Error(t, syncErr, "sync should propagate the permission-denied error")

	after, err := svc.ListAllProjects(ctx)
	require.NoError(t, err)
	assert.Len(t, after, 1, "DB records must not be wiped when discovery fails")
	assert.Equal(t, before[0].Path, after[0].Path)
}

// TestProjectService_SyncProjectsFromFileSystem_DiscoversReadableProjectsDespiteUnreadableNestedSibling
// covers the regression from GitHub issue #3080: a permission-denied error on a
// non-root nested subdirectory used to abort discovery of ALL projects. Unlike
// TestProjectService_SyncProjectsFromFileSystem_PreservesDBRecordsWhenDirectoryUnreadable
// above (which makes the projects ROOT itself unreadable and expects sync to still
// error), this test makes only a NESTED subdirectory unreadable and expects sync to
// succeed, discover the readable sibling project, and leave alone any existing DB
// record whose path happens to live under the now-skipped unreadable subtree.
func TestProjectService_SyncProjectsFromFileSystem_DiscoversReadableProjectsDespiteUnreadableNestedSibling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission-denied behavior is not portable to Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("test requires a non-root UID to trigger permission-denied on ReadDir")
	}

	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	project1Path := createComposeProjectDir(t, projectsRoot, "project1")

	unreadableDir := filepath.Join(projectsRoot, "adguard-cheetah", "workingdir")
	require.NoError(t, os.MkdirAll(unreadableDir, 0o755))
	t.Cleanup(func() { _ = os.Chmod(unreadableDir, 0o700) })
	require.NoError(t, os.Chmod(unreadableDir, 0))

	// Seed a DB row (bypassing the filesystem) whose Path sits under the
	// now-unreadable subtree, simulating a project Arcane previously knew about.
	strandedDirName := "stranded-project"
	require.NoError(t, db.Create(&Project{
		ID:      "stranded-under-unreadable",
		Name:    "stranded-project",
		DirName: &strandedDirName,
		Path:    filepath.Join(unreadableDir, "stranded-project"),
		Status:  ProjectStatusStopped,
	}).Error)

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx), "sync must succeed despite the unreadable nested directory")

	after, err := svc.ListAllProjects(ctx)
	require.NoError(t, err)

	var foundReadableSibling bool
	var foundStrandedRecord bool
	for _, p := range after {
		if p.Path == project1Path {
			foundReadableSibling = true
		}
		if p.ID == "stranded-under-unreadable" {
			foundStrandedRecord = true
		}
	}

	assert.True(t, foundReadableSibling, "readable sibling project must still be discovered")
	// evaluateProjectCleanup only prunes on os.IsNotExist; a permission-denied
	// stat falls into the "keep the record" branch, so the stranded record must survive.
	assert.True(t, foundStrandedRecord, "DB record under the unreadable subtree must not be pruned by a permission-denied stat")
}

func TestProjectService_SyncProjectsFromFileSystem_AllowsDuplicateLeafDirectoriesInDifferentParents(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	firstPath := createComposeProjectDir(t, projectsRoot, filepath.Join("main-project1", "app"))
	secondPath := createComposeProjectDir(t, projectsRoot, filepath.Join("main-project2", "app"))

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	var items []Project
	require.NoError(t, db.WithContext(ctx).Order("path asc").Find(&items).Error)
	require.Len(t, items, 2)

	require.NotNil(t, items[0].DirName)
	require.NotNil(t, items[1].DirName)
	assert.Equal(t, "app", *items[0].DirName)
	assert.Equal(t, "app", *items[1].DirName)
	assert.Equal(t, firstPath, items[0].Path)
	assert.Equal(t, secondPath, items[1].Path)
}

func TestProjectService_SyncProjectsFromFileSystem_DetectsNestedSymlinkedProjectDirsWhenEnabled(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	targetRoot := t.TempDir()
	targetPath := createComposeProjectDir(t, targetRoot, filepath.Join("main-project", "sub-project1"))
	linkPath := filepath.Join(projectsRoot, "linked-root")
	require.NoError(t, os.Symlink(filepath.Join(targetRoot, "main-project"), linkPath))

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))
	require.NoError(t, settingsService.SetStringSetting(ctx, "followProjectSymlinks", "true"))

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	items, page, err := svc.ListProjects(ctx, pagination.QueryParams{
		Sort: "path", Order: pagination.SortAsc,
		Limit: -1,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.TotalItems)
	require.Len(t, items, 1)
	assert.Equal(t, filepath.Join(linkPath, "sub-project1"), items[0].Path)
	assert.Equal(t, "linked-root/sub-project1", items[0].RelativePath)
	assert.Equal(t, targetPath, filepath.Join(targetRoot, "main-project", "sub-project1"))
}

func TestProjectService_SyncProjectsFromFileSystem_RemovesSymlinkedProjectsWhenDisabled(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	targetRoot := t.TempDir()
	linkTarget := createComposeProjectDir(t, targetRoot, "linked-target")
	linkPath := filepath.Join(projectsRoot, "linked")
	require.NoError(t, os.Symlink(linkTarget, linkPath))

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))
	require.NoError(t, settingsService.SetStringSetting(ctx, "followProjectSymlinks", "true"))

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	items, err := svc.ListAllProjects(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)

	require.NoError(t, settingsService.SetStringSetting(ctx, "followProjectSymlinks", "false"))
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	items, err = svc.ListAllProjects(ctx)
	require.NoError(t, err)
	assert.Empty(t, items)

	_, statErr := os.Lstat(linkPath)
	require.NoError(t, statErr)
}

func TestProjectService_SyncProjectsFromFileSystem_RefreshesServiceCountOnComposeChange(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	projectPath := createComposeProjectDir(t, projectsRoot, "demo")

	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	var project Project
	require.NoError(t, db.WithContext(ctx).Where("path = ?", projectPath).First(&project).Error)
	assert.Equal(t, 1, project.ServiceCount)

	updatedCompose := "services:\n  app:\n    image: nginx:alpine\n  worker:\n    image: busybox:latest\n"
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte(updatedCompose), 0o644))

	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))
	require.NoError(t, db.WithContext(ctx).Where("id = ?", project.ID).First(&project).Error)
	assert.Equal(t, 2, project.ServiceCount)
}

func TestProjectService_SyncProjectsFromFileSystem_AlignsNameToEffectiveComposeName(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	projectPath := createComposeProjectDir(t, projectsRoot, "aitools")
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte(`name: ai_tools
services:
  app:
    image: nginx:alpine
`), 0o644))

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	var project Project
	require.NoError(t, db.WithContext(ctx).Where("path = ?", projectPath).First(&project).Error)
	assert.Equal(t, "ai_tools", project.Name)
	require.NotNil(t, project.DirName)
	assert.Equal(t, "aitools", *project.DirName)
	require.NotNil(t, project.ComposeProjectName)
	assert.Equal(t, "ai_tools", *project.ComposeProjectName)
}

func TestProjectService_SyncProjectsFromFileSystem_PreservesValidCustomNameWithoutExplicitComposeName(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)

	projectsRoot := t.TempDir()
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsRoot))

	projectPath := createComposeProjectDir(t, projectsRoot, "folder-name")
	dirName := "folder-name"
	project := &Project{
		ID:      "proj-custom-name",
		Name:    "custom-name",
		DirName: &dirName,
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))

	var fromDB Project
	require.NoError(t, db.WithContext(ctx).Where("id = ?", project.ID).First(&fromDB).Error)
	assert.Equal(t, "custom-name", fromDB.Name)
	require.NotNil(t, fromDB.DirName)
	assert.Equal(t, "folder-name", *fromDB.DirName)
	require.Nil(t, fromDB.ComposeProjectName)
}
