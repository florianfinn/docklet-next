// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Migrated SQLite and real Compose file identity cases.

package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

func runtimeIdentityFixture(t *testing.T) (*ProjectService, string) {
	t.Helper()
	ctx := t.Context()
	db, err := database.InitializeHub(ctx, sqlitestore.URL(filepath.Join(t.TempDir(), "identity.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	settingsService, err := settings.NewSettingsService(ctx, db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, settingsService.Stop(context.WithoutCancel(ctx))) })
	root := filepath.Join(t.TempDir(), "projects")
	require.NoError(t, os.MkdirAll(root, 0755))
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", root))
	return NewProjectService(db, settingsService, nil, nil, nil, nil, nil, &config.Config{ProjectsDirectory: root}, nil, nil), root
}

func runtimeIdentityProject(t *testing.T, svc *ProjectService, root, id, directory, compose string) *Project {
	t.Helper()
	path := filepath.Join(root, directory)
	require.NoError(t, os.MkdirAll(path, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(path, "compose.yaml"), []byte(compose), 0644))
	row := &Project{Name: directory, DirName: &directory, Path: path, Status: ProjectStatusStopped}
	row.ID = id
	require.NoError(t, svc.db.Create(row).Error)
	return row
}

func TestResolveRuntimeProjectIdentityFreshFilesAndOwnership(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project-a", "alpha", "name: custom_stack\nservices:\n  app:\n    image: example.invalid/app:1\n")
	ctx := t.Context()
	ref, found, err := svc.ResolveRuntimeProjectIdentity(ctx, "", "custom_stack")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, row.ID, ref.ID)
	require.Equal(t, "custom_stack", ref.Name)
	ref, found, err = svc.ResolveRuntimeProjectIdentity(ctx, row.ID, "custom_stack")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, row.ID, ref.ID)
	_, found, err = svc.ResolveRuntimeProjectIdentity(ctx, row.ID, "old_stack")
	require.NoError(t, err)
	require.False(t, found)

	// A warm name cache and recorded name cannot override changed files.
	svc.composeNames.put("custom_stack", row.ID)
	row.ComposeProjectName = new("custom_stack")
	require.NoError(t, svc.db.Model(row).Update("compose_project_name", "custom_stack").Error)
	require.NoError(t, os.WriteFile(filepath.Join(row.Path, "compose.yaml"), []byte("name: changed_stack\nservices:\n  app:\n    image: example.invalid/app:1\n"), 0644))
	_, found, err = svc.ResolveRuntimeProjectIdentity(ctx, "", "custom_stack")
	require.NoError(t, err)
	require.False(t, found)
	ref, found, err = svc.ResolveRuntimeProjectIdentity(ctx, "", "changed_stack")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, row.ID, ref.ID)

	// Archived records still own their Compose identity; mutation refusal is
	// enforced by the existing project service methods.
	require.NoError(t, svc.db.Model(row).Update("is_archived", true).Error)
	ref, found, err = svc.ResolveRuntimeProjectIdentity(ctx, "", "changed_stack")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, row.ID, ref.ID)
	_, err = svc.getMutableProject(ctx, row.ID)
	require.Error(t, err)
}

func TestResolveRuntimeProjectIdentityEnvAndAmbiguity(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project-a", "alpha", "name: ${COMPOSE_PROJECT_NAME}\nservices:\n  app:\n    image: example.invalid/app:1\n")
	require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte("COMPOSE_PROJECT_NAME=first_stack\n"), 0644))
	ref, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), "", "first_stack")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, row.ID, ref.ID)
	require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte("COMPOSE_PROJECT_NAME=second_stack\n"), 0644))
	_, found, err = svc.ResolveRuntimeProjectIdentity(t.Context(), "", "first_stack")
	require.NoError(t, err)
	require.False(t, found)
	_, found, err = svc.ResolveRuntimeProjectIdentity(t.Context(), "", "second_stack")
	require.NoError(t, err)
	require.True(t, found)

	second := runtimeIdentityProject(t, svc, root, "example-project-b", "beta", "name: second_stack\nservices:\n  app:\n    image: example.invalid/app:1\n")
	_, found, err = svc.ResolveRuntimeProjectIdentity(t.Context(), "", "second_stack")
	require.NoError(t, err)
	require.False(t, found)
	_, found, err = svc.ResolveRuntimeProjectIdentity(t.Context(), row.ID, "second_stack")
	require.NoError(t, err)
	require.False(t, found)
	_ = second
}

func TestResolveRuntimeProjectIdentityEnvDefaultAndUnreadable(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project-a", "alpha", "services:\n  app:\n    image: example.invalid/app:1\n")
	require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte("COMPOSE_PROJECT_NAME=env_stack\n"), 0644))
	ref, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), row.ID, "")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "alpha", ref.Name)
	require.NoError(t, os.WriteFile(filepath.Join(row.Path, "compose.yaml"), []byte("not: [valid"), 0644))
	_, found, err = svc.ResolveRuntimeProjectIdentity(t.Context(), row.ID, "")
	require.Error(t, err)
	require.False(t, found)
	require.NoError(t, svc.db.Exec("DROP TABLE projects").Error)
	_, found, err = svc.ResolveRuntimeProjectIdentity(t.Context(), row.ID, "")
	require.Error(t, err)
	require.False(t, found)
}

func TestResolveRuntimeProjectIdentityUnrelatedUnreadableRowFailsClosed(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	healthy := runtimeIdentityProject(t, svc, root, "example-project-healthy", "healthy", "name: healthy_stack\nservices:\n  app:\n    image: example.invalid/app:1\n")
	unrelated := runtimeIdentityProject(t, svc, root, "example-project-broken", "broken", "not: [valid")
	for _, selector := range []struct{ id, name string }{{healthy.ID, ""}, {"", "healthy_stack"}} {
		ref, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), selector.id, selector.name)
		require.Error(t, err, "an unreadable project cannot be skipped during ambiguity checking")
		require.False(t, found)
		require.Empty(t, ref.ID)
	}
	require.NoError(t, os.WriteFile(filepath.Join(unrelated.Path, "compose.yaml"), []byte("name: unrelated_stack\nservices:\n  app:\n    image: example.invalid/app:1\n"), 0644))
	ref, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), healthy.ID, "")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, healthy.ID, ref.ID)
}

const identityServices = "services:\n  app:\n    image: registry.example.com/app:1\n"

func identityRewriteSameMtime(t *testing.T, path, content string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	require.NoError(t, os.Chtimes(path, info.ModTime(), info.ModTime()))
}

func TestProjectIdentityStoredMetadataAndDiscovery(t *testing.T) {
	for _, tc := range []struct{ name, stored, yaml, env, want string }{
		{"env-only", "custom_default", "", "COMPOSE_PROJECT_NAME=env_stack\n", "custom_default"},
		{"literal-conflict", "custom_default", "name: yaml_stack\n", "COMPOSE_PROJECT_NAME=env_stack\n", "yaml_stack"},
		{"interpolated", "custom_default", "name: ${COMPOSE_PROJECT_NAME}\n", "COMPOSE_PROJECT_NAME=env_stack\n", "env_stack"},
		{"custom", "custom_default", "", "", "custom_default"},
		{"normalized", " Custom Default! ", "", "COMPOSE_PROJECT_NAME=env_stack\n", "customdefault"},
		{"empty-default", "", "", "COMPOSE_PROJECT_NAME=env_stack\n", "env_stack"},
		{"invalid-default", "!!!", "", "COMPOSE_PROJECT_NAME=env_stack\n", "!!!"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			row := runtimeIdentityProject(t, svc, root, "example-project", "folder", tc.yaml+identityServices)
			row.Name = tc.stored
			require.NoError(t, svc.db.Save(row).Error)
			require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte(tc.env), 0600))
			for i := 0; i < 2; i++ {
				model, _, err := svc.loadComposeProjectForProject(t.Context(), row, nil)
				require.NoError(t, err)
				require.Equal(t, tc.want, model.Name)
				ref, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), row.ID, tc.want)
				require.NoError(t, err)
				require.True(t, found)
				require.Equal(t, model.Name, ref.Name)
				svc.refreshComposeProjectName(t.Context(), row)
				require.NoError(t, svc.upsertProjectForDir(t.Context(), "folder", row.Path))
				require.NoError(t, svc.db.First(row, "id = ?", row.ID).Error)
				require.Equal(t, tc.stored, row.Name, "metadata must not rewrite the executable default")
				require.Equal(t, 1, row.ServiceCount)
				if tc.want == projects.NormalizeProjectName(tc.stored) {
					require.Nil(t, row.ComposeProjectName)
				} else {
					require.Equal(t, new(tc.want), row.ComposeProjectName)
				}
				_, err = svc.getCachedComposeProject(t.Context(), row, svc.newProjectMetadataEnv(t.Context(), []Project{*row}))
				require.NoError(t, err)
			}
		})
	}
	for _, tc := range []struct{ name, yaml, env, want string }{
		{"env", "", "COMPOSE_PROJECT_NAME=env_stack\n", "env_stack"},
		{"literal-env-conflict", "name: yaml_stack\n", "COMPOSE_PROJECT_NAME=env_stack\n", "env_stack"},
		{"yaml", "name: yaml_stack\n", "", "yaml_stack"},
		{"directory", "", "", "folder"},
	} {
		t.Run("new-"+tc.name, func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			path := filepath.Join(root, "folder")
			require.NoError(t, os.MkdirAll(path, 0755))
			require.NoError(t, os.WriteFile(filepath.Join(path, "compose.yaml"), []byte(tc.yaml+identityServices), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(path, ".env"), []byte(tc.env), 0600))
			require.NoError(t, svc.SyncProjectsFromFileSystem(t.Context()))
			var row Project
			require.NoError(t, svc.db.Where("path = ?", path).First(&row).Error)
			require.Equal(t, tc.want, row.Name, "initial discovery retains its naming policy")
			id := row.ID
			require.NoError(t, svc.SyncProjectsFromFileSystem(t.Context()))
			require.NoError(t, svc.db.First(&row, "id = ?", id).Error)
			require.Equal(t, tc.want, row.Name)
			require.Equal(t, id, row.ID)
		})
	}
}

func TestProjectIdentityRefreshDoesNotChangeFutureDefault(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project", "folder", "name: yaml_stack\n"+identityServices)
	require.NoError(t, svc.db.Model(row).Update("name", " Custom Default! ").Error)
	row.Name = " Custom Default! "
	for i := 0; i < 2; i++ {
		svc.refreshComposeProjectName(t.Context(), row)
		require.Equal(t, " Custom Default! ", row.Name)
		require.Equal(t, new("yaml_stack"), row.ComposeProjectName)
		require.NoError(t, svc.upsertProjectForDir(t.Context(), "folder", row.Path))
		require.NoError(t, svc.db.First(row, "id = ?", row.ID).Error)
		require.Equal(t, " Custom Default! ", row.Name)
	}
	require.NoError(t, os.WriteFile(filepath.Join(row.Path, "compose.yaml"), []byte(identityServices), 0600))
	svc.refreshComposeProjectName(t.Context(), row)
	require.Nil(t, row.ComposeProjectName)
	ref, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), row.ID, "customdefault")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, row.ID, ref.ID)
}

func TestProjectIdentityExplicitRenameKeepsIDAndRefresh(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	configureProjectRuntimeDocker(t, nil)
	row := runtimeIdentityProject(t, svc, root, "example-project", "folder", identityServices)
	updated, err := svc.UpdateProject(t.Context(), row.ID, new("renamed"), nil, nil, nil, usertypes.Actor{ID: "example-owner", Username: "Operator"})
	require.NoError(t, err)
	require.Equal(t, row.ID, updated.ID)
	require.Equal(t, "renamed", updated.Name)
	require.NoDirExists(t, row.Path)
	require.DirExists(t, updated.Path)
	svc.refreshComposeProjectName(t.Context(), updated)
	require.NoError(t, svc.SyncProjectsFromFileSystem(t.Context()))
	ref, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), updated.ID, "renamed")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, row.ID, ref.ID)
}

func TestProjectIdentityFreshSelectionBypassesReadCaches(t *testing.T) {
	for _, source := range []string{"primary", "auxiliary", "primary-symlink", "auxiliary-symlink", "base", "override", "include", "selection"} {
		t.Run(source, func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			row := runtimeIdentityProject(t, svc, root, "example-project", "folder", "name: ${STACK_NAME}\n"+identityServices)
			envPath := filepath.Join(row.Path, ".env")
			require.NoError(t, os.WriteFile(envPath, []byte("STACK_NAME=first_stack\n"), 0600))
			path, content := envPath, "STACK_NAME=second_stack\n"
			switch source {
			case "auxiliary", "auxiliary-symlink":
				require.NoError(t, os.WriteFile(envPath, []byte("COMPOSE_ENV_FILES=aux.env\n"), 0600))
				path = filepath.Join(row.Path, "aux.env")
				require.NoError(t, os.WriteFile(path, []byte("STACK_NAME=first_stack\n"), 0600))
			case "base":
				path = filepath.Join(row.Path, "compose.yaml")
				content = "name: second_stack\n" + identityServices
			case "override":
				path = filepath.Join(row.Path, "compose.override.yaml")
				require.NoError(t, os.WriteFile(path, []byte("name: first_stack\n"), 0600))
				content = "name: second_stack\n"
			case "include":
				require.NoError(t, os.WriteFile(filepath.Join(row.Path, "compose.yaml"), []byte("include:\n  - extra.yaml\nname: ${STACK_NAME}\n"+identityServices), 0600))
				path = filepath.Join(row.Path, "extra.yaml")
				require.NoError(t, os.WriteFile(path, []byte("services:\n  worker:\n    image: registry.example.com/worker:1\n"), 0600))
				content = "services:\n  replacement:\n    image: registry.example.com/worker:2\n"
			case "selection":
				require.NoError(t, os.WriteFile(filepath.Join(row.Path, "next.yaml"), []byte("name: second_stack\n"+identityServices), 0600))
				content = "STACK_NAME=first_stack\nCOMPOSE_FILE=next.yaml\n"
			}
			if strings.HasSuffix(source, "symlink") {
				bytes, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, os.Remove(path))
				target := filepath.Join(t.TempDir(), "external.env")
				require.NoError(t, os.WriteFile(target, bytes, 0600))
				require.NoError(t, os.Symlink(target, path))
				path = target
			}
			memo := svc.newProjectMetadataEnv(t.Context(), []Project{*row})
			cached, err := svc.getCachedComposeProject(t.Context(), row, memo)
			require.NoError(t, err)
			require.Equal(t, "first_stack", cached.Name)
			ref, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), row.ID, "")
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, "first_stack", ref.Name)
			identityRewriteSameMtime(t, path, content)
			cachedAgain, err := svc.getCachedComposeProject(t.Context(), row, memo)
			require.NoError(t, err)
			require.Equal(t, "first_stack", cachedAgain.Name, "runtime freshness must survive an ordinary stale read cache")
			if source == "selection" {
				require.Equal(t, filepath.Join(row.Path, "compose.yaml"), memo.composeFiles[row.ID])
			}
			want := "second_stack"
			if source == "include" {
				want = "first_stack"
			}
			ref, found, err = svc.ResolveRuntimeProjectIdentity(t.Context(), row.ID, want)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, want, ref.Name)
			model, _, err := svc.loadComposeProjectForProject(t.Context(), row, nil)
			require.NoError(t, err)
			require.Equal(t, want, model.Name)
			if source == "include" {
				require.Contains(t, model.Services, "replacement")
				require.NotContains(t, model.Services, "worker")
			}
		})
	}
}

func TestProjectIdentityReadOnlyAndAvailabilityRecovery(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-healthy", "healthy", "name: healthy_stack\n"+identityServices+"    volumes:\n      - ./must-not-exist:/data\n")
	var before Project
	require.NoError(t, svc.db.First(&before, "id = ?", row.ID).Error)
	ref, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), row.ID, "")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "healthy_stack", ref.Name)
	require.NoDirExists(t, filepath.Join(row.Path, "must-not-exist"))
	require.NoFileExists(t, filepath.Join(row.Path, ".env"))
	var after Project
	require.NoError(t, svc.db.First(&after, "id = ?", row.ID).Error)
	require.Equal(t, before, after, "identity reads must not mutate the persisted project")
	_, found, err = svc.ResolveRuntimeProjectIdentity(t.Context(), "missing-id", "")
	require.NoError(t, err)
	require.False(t, found)
	peer := runtimeIdentityProject(t, svc, root, "example-peer", "peer", "name: other_stack\n"+identityServices)
	for _, failure := range []string{"missing", "malformed-yaml", "malformed-primary", "permission-locked"} {
		t.Run(failure, func(t *testing.T) {
			compose := filepath.Join(peer.Path, "compose.yaml")
			env := filepath.Join(peer.Path, ".env")
			switch failure {
			case "permission-locked":
				if os.Geteuid() == 0 {
					t.Skip("permission bits are ignored when running as root")
				}
				require.NoError(t, os.Chmod(compose, 0000))
				t.Cleanup(func() { require.NoError(t, os.Chmod(compose, 0600)) })
			case "missing":
				require.NoError(t, os.Remove(compose))
			case "malformed-yaml":
				require.NoError(t, os.WriteFile(compose, []byte("not: [valid"), 0600))
			case "malformed-primary":
				require.NoError(t, os.WriteFile(env, []byte("VALUE=\"unterminated\n"), 0600))
			}
			for _, selector := range []struct{ id, name string }{{row.ID, ""}, {"", "healthy_stack"}} {
				_, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), selector.id, selector.name)
				require.Error(t, err, "unknown peers block exact ID and parent/name lookup")
				require.False(t, found)
			}
			if failure == "permission-locked" {
				require.NoError(t, os.Chmod(compose, 0600))
			}
			require.NoError(t, os.WriteFile(compose, []byte("name: other_stack\n"+identityServices), 0600))
			if failure == "malformed-primary" {
				require.NoError(t, os.Remove(env))
			}
			for _, selector := range []struct{ id, name string }{{row.ID, ""}, {"", "healthy_stack"}} {
				_, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), selector.id, selector.name)
				require.NoError(t, err)
				require.True(t, found)
			}
		})
	}
	require.NoError(t, svc.db.Model(peer).Updates(map[string]any{"is_archived": true}).Error)
	require.NoError(t, os.WriteFile(filepath.Join(peer.Path, "compose.yaml"), []byte("name: healthy_stack\n"+identityServices), 0600))
	for _, selector := range []struct{ id, name string }{{row.ID, ""}, {peer.ID, ""}, {"", "healthy_stack"}, {"missing-id", "healthy_stack"}, {row.ID, "other_stack"}} {
		_, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), selector.id, selector.name)
		require.NoError(t, err)
		require.False(t, found)
	}
}

func TestProjectIdentityDiscoveryDatabaseError(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	require.NoError(t, svc.db.Exec("DROP TABLE projects").Error)
	err := svc.upsertProjectForDir(t.Context(), "folder", filepath.Join(root, "folder"))
	require.ErrorContains(t, err, "query existing project")
}

func TestProjectIdentityStoredEmptyDefaultDoesNotRetryDirectory(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project", "folder", identityServices)
	row.Name = ""
	require.NoError(t, svc.db.Save(row).Error)
	_, _, err := svc.loadComposeProjectForProject(t.Context(), row, nil)
	require.Error(t, err)
	_, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), row.ID, "")
	require.Error(t, err, "stored metadata must not retry with a different directory default")
	require.False(t, found)
	svc.refreshComposeProjectName(t.Context(), row)
	require.Equal(t, "", row.Name)
}
