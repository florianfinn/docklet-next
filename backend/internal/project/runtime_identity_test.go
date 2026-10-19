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
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
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
	return &ProjectService{db: db, settingsService: settingsService}, root
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

func TestResolveRuntimeProjectIdentityUnreadableAndNameDisagreement(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project-a", "alpha", "services:\n  app:\n    image: example.invalid/app:1\n")
	require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte("COMPOSE_PROJECT_NAME=env_stack\n"), 0644))
	_, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), row.ID, "")
	require.ErrorContains(t, err, "differs")
	require.False(t, found)
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
