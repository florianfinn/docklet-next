// Source: docklet-next/docs/design/runtime-control.md
// Revision: 52eeff421c6036d4ede2abe4ca5ee183d1516978e02646d81935316ccee25eae
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Migrated SQLite coverage of observed Compose-name consumers and caches.

package project

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

func TestComposeNameLookupAfterStoredRefresh(t *testing.T) {
	for _, warm := range []bool{false, true} {
		name := "cold"
		if warm {
			name = "warm"
		}
		t.Run(name, func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", identityServices)
			if warm {
				found, err := svc.GetProjectByComposeName(t.Context(), "alpha")
				require.NoError(t, err)
				require.Equal(t, row.ID, found.ID)
			}
			require.NoError(t, os.WriteFile(filepath.Join(row.Path, "compose.yaml"), []byte("name: beta_stack\n"+identityServices), 0600))
			svc.refreshComposeProjectName(t.Context(), row)
			require.NoError(t, svc.db.First(row, "id = ?", row.ID).Error)
			require.Equal(t, "alpha", row.Name)
			require.Equal(t, new("beta_stack"), row.ComposeProjectName)
			for i := 0; i < 2; i++ {
				input := "beta_stack"
				if i == 1 {
					input = "BETA_STACK!"
				}
				found, err := svc.GetProjectByComposeName(t.Context(), input)
				require.NoError(t, err, "Compose labels must resolve the observed effective owner")
				require.Equal(t, row.ID, found.ID)
			}
			_, err := svc.GetProjectByComposeName(t.Context(), "alpha")
			require.ErrorIs(t, err, common.ErrNotFound, "an overridden default is no longer a Compose label")
			_, cached := svc.composeNames.projectID("alpha").Get()
			require.False(t, cached)
			projectID, image, err := svc.ComposeServiceImage(t.Context(), "beta_stack", "app")
			require.NoError(t, err)
			require.Equal(t, row.ID, projectID)
			require.Equal(t, "registry.example.com/app:1", image)
		})
	}
}

func TestComposeNameLookupCreateAndUpdate(t *testing.T) {
	for _, tc := range []struct{ name, yaml, env, stored, effective string }{
		{"literal-create", "name: yaml_stack\n", "", "yaml_stack", "yaml_stack"},
		{"interpolated-create", "name: ${STACK_NAME}\n", "STACK_NAME=beta_stack\n", "alpha", "beta_stack"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := runtimeIdentityFixture(t)
			row, err := svc.CreateProject(t.Context(), "alpha", tc.yaml+identityServices, &tc.env, projecttypes.CreateProjectWorkspaceManifest{}, nil, nil, nil, usertypes.Actor{})
			require.NoError(t, err)
			require.Equal(t, tc.stored, row.Name, "literal create keeps its existing authoritative YAML policy")
			found, err := svc.GetProjectByComposeName(t.Context(), tc.effective)
			require.NoError(t, err)
			require.Equal(t, row.ID, found.ID)
			id, image, err := svc.ComposeServiceImage(t.Context(), tc.effective, "app")
			require.NoError(t, err)
			require.Equal(t, row.ID, id)
			require.Equal(t, "registry.example.com/app:1", image)
		})
	}
	t.Run("update-interpolated-add-change-remove", func(t *testing.T) {
		svc, root := runtimeIdentityFixture(t)
		configureProjectRuntimeDocker(t, nil)
		row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", identityServices)
		for _, effective := range []string{"beta_stack", "next_stack", "alpha"} {
			yaml := "name: ${STACK_NAME}\n" + identityServices
			if effective == "alpha" {
				yaml = identityServices
			}
			updated, err := svc.UpdateProject(t.Context(), row.ID, nil, &yaml, new("STACK_NAME="+effective+"\n"), nil, usertypes.Actor{})
			require.NoError(t, err)
			require.Equal(t, row.ID, updated.ID)
			require.Equal(t, "alpha", updated.Name)
			found, err := svc.GetProjectByComposeName(t.Context(), effective)
			require.NoError(t, err)
			require.Equal(t, row.ID, found.ID)
			if effective != "beta_stack" {
				_, err = svc.GetProjectByComposeName(t.Context(), "beta_stack")
				require.ErrorIs(t, err, common.ErrNotFound)
			}
		}
	})
	t.Run("update-literal-explicit-rename", func(t *testing.T) {
		svc, root := runtimeIdentityFixture(t)
		configureProjectRuntimeDocker(t, nil)
		row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", identityServices)
		for _, name := range []string{"beta_stack", "next_stack"} {
			yaml := "name: " + name + "\n" + identityServices
			updated, err := svc.UpdateProject(t.Context(), row.ID, nil, &yaml, nil, nil, usertypes.Actor{})
			require.NoError(t, err)
			require.Equal(t, row.ID, updated.ID)
			require.Equal(t, name, updated.Name, "explicit YAML edits retain the existing rename workflow")
			found, err := svc.GetProjectByComposeName(t.Context(), name)
			require.NoError(t, err)
			require.Equal(t, row.ID, found.ID)
		}
	})
}

func TestComposeNameLookupFilesystemSync(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", identityServices)
	for _, name := range []string{"first_stack", "second_stack"} {
		require.NoError(t, os.WriteFile(filepath.Join(row.Path, "compose.yaml"), []byte("name: ${STACK_NAME}\n"+identityServices), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte("STACK_NAME="+name+"\n"), 0600))
		require.NoError(t, svc.SyncProjectsFromFileSystem(t.Context()))
		found, err := svc.GetProjectByComposeName(t.Context(), name)
		require.NoError(t, err)
		require.Equal(t, row.ID, found.ID)
		require.Equal(t, "alpha", found.Name)
		require.Equal(t, new(name), found.ComposeProjectName)
	}
	_, err := svc.GetProjectByComposeName(t.Context(), "first_stack")
	require.ErrorIs(t, err, common.ErrNotFound)
	_, cached := svc.composeNames.projectID("first_stack").Get()
	require.False(t, cached)
}

func TestComposeNameLookupEffectiveOwnerPrecedesDefaultAlias(t *testing.T) {
	for _, warm := range []bool{false, true} {
		t.Run(map[bool]string{false: "cold", true: "warm"}[warm], func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			alias := runtimeIdentityProject(t, svc, root, "a-default-alias", "beta_stack", "name: other_stack\n"+identityServices)
			owner := runtimeIdentityProject(t, svc, root, "z-effective-owner", "alpha", "name: beta_stack\n"+identityServices)
			if warm {
				found, err := svc.GetProjectByComposeName(t.Context(), "beta_stack")
				require.NoError(t, err)
				require.Equal(t, alias.ID, found.ID, "without metadata the stored name remains the legacy fallback")
			}
			svc.refreshComposeProjectName(t.Context(), owner)
			found, err := svc.GetProjectByComposeName(t.Context(), "beta_stack")
			require.NoError(t, err)
			require.Equal(t, owner.ID, found.ID, "an exact observation must precede a legacy/default alias regardless of row order")
			_, err = svc.GetProjectByComposeName(t.Context(), "missing_stack")
			require.ErrorIs(t, err, common.ErrNotFound)
			cachedID, ok := svc.composeNames.projectID("beta_stack").Get()
			require.True(t, ok)
			require.Equal(t, owner.ID, cachedID, "rebuild must also prefer observed metadata over legacy aliases")
			svc.refreshComposeProjectName(t.Context(), alias)
			found, err = svc.GetProjectByComposeName(t.Context(), "beta_stack")
			require.NoError(t, err)
			require.Equal(t, owner.ID, found.ID, "a known overridden default must not steal an effective label")
		})
	}
}

func TestComposeNameLookupCacheRevalidationAndRebuild(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", identityServices)
	row.Name = "Custom Default!"
	row.ComposeProjectName = new("beta_stack")
	require.NoError(t, svc.db.Save(row).Error)
	svc.composeNames.put("beta_stack", row.ID)
	found, ok, err := svc.lookupProjectByCachedComposeName(t.Context(), "beta_stack")
	require.NoError(t, err)
	require.True(t, ok, "warm cache must validate the effective observation")
	require.Equal(t, row.ID, found.ID)
	for _, change := range []struct {
		column string
		value  any
	}{
		{"compose_project_name", "next_stack"},
		{"compose_project_name", nil},
		{"name", "Renamed Default!"},
	} {
		old := observedComposeName(*row)
		svc.composeNames.put(old, row.ID)
		require.NoError(t, svc.db.Model(row).Update(change.column, change.value).Error)
		require.NoError(t, svc.db.First(row, "id = ?", row.ID).Error)
		_, ok, err = svc.lookupProjectByCachedComposeName(t.Context(), old)
		require.NoError(t, err)
		require.False(t, ok, "changed or removed effective aliases must invalidate")
		_, ok = svc.composeNames.projectID(old).Get()
		require.False(t, ok)
	}
	row.ComposeProjectName = new("known_stack")
	require.NoError(t, svc.db.Save(row).Error)
	legacy := runtimeIdentityProject(t, svc, root, "example-legacy", "legacy", identityServices)
	require.NoError(t, svc.db.Model(legacy).Updates(map[string]any{"name": "Legacy Custom!", "compose_project_name": ""}).Error)
	svc.composeNames.replace(map[string]string{"obsolete": row.ID})
	_, err = svc.GetProjectByComposeName(t.Context(), "missing_stack")
	require.ErrorIs(t, err, common.ErrNotFound)
	for name, id := range map[string]string{"known_stack": row.ID, "legacycustom": legacy.ID} {
		cachedID, cached := svc.composeNames.projectID(name).Get()
		require.True(t, cached)
		require.Equal(t, id, cachedID, "rebuild must include metadata and legacy normalization")
	}
	for _, name := range []string{"obsolete", "renameddefault"} {
		_, ok = svc.composeNames.projectID(name).Get()
		require.False(t, ok, "rebuild must drop stale/default aliases")
	}
	found, err = svc.GetProjectByComposeName(t.Context(), "legacycustom")
	require.NoError(t, err)
	require.Equal(t, legacy.ID, found.ID)
	require.NoError(t, svc.db.Delete(row).Error)
	_, ok, err = svc.lookupProjectByCachedComposeName(t.Context(), "known_stack")
	require.NoError(t, err)
	require.False(t, ok)
	_, ok = svc.composeNames.projectID("known_stack").Get()
	require.False(t, ok)
	replacement := runtimeIdentityProject(t, svc, root, "example-replacement", "replacement", identityServices)
	require.NoError(t, svc.db.Model(replacement).Update("compose_project_name", "known_stack").Error)
	svc.composeNames.put("known_stack", row.ID)
	found, err = svc.GetProjectByComposeName(t.Context(), "known_stack")
	require.NoError(t, err)
	require.Equal(t, replacement.ID, found.ID, "a stale deleted ID cannot misroute a reused label")
}

func TestComposeNameLookupDuplicateWarnings(t *testing.T) {
	for _, mode := range []string{"create", "refresh", "unchanged", "default-alias"} {
		t.Run(mode, func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			owner := runtimeIdentityProject(t, svc, root, "example-owner", "alpha", "name: shared_stack\n"+identityServices)
			svc.refreshComposeProjectName(t.Context(), owner)
			path := filepath.Join(root, "peer")
			if mode != "create" {
				peer := runtimeIdentityProject(t, svc, root, "example-peer", "peer", identityServices)
				path = peer.Path
			}
			require.NoError(t, os.MkdirAll(path, 0755))
			name := "shared_stack"
			if mode == "default-alias" {
				name = "alpha"
			}
			require.NoError(t, os.WriteFile(filepath.Join(path, "compose.yaml"), []byte("name: "+name+"\n"+identityServices), 0600))
			if mode == "unchanged" {
				require.NoError(t, svc.upsertProjectForDir(t.Context(), "peer", path))
			}
			var log bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&log, &slog.HandlerOptions{Level: slog.LevelWarn})))
			t.Cleanup(func() { slog.SetDefault(previous) })
			require.NoError(t, svc.upsertProjectForDir(t.Context(), "peer", path))
			if mode == "default-alias" {
				require.NotContains(t, log.String(), "multiple project directories resolve")
			} else {
				require.Contains(t, log.String(), "multiple project directories resolve to the same compose project name")
				require.Contains(t, log.String(), `"composeProjectName":"shared_stack"`)
				require.Contains(t, log.String(), `"duplicates":1`)
			}
		})
	}
}

func TestComposeNameLookupDatabaseErrors(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", identityServices)
	svc.composeNames.put("alpha", row.ID)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := svc.GetProjectByComposeName(ctx, "alpha")
	require.ErrorIs(t, err, context.Canceled)
	_, _, err = svc.lookupProjectByCachedComposeName(ctx, "alpha")
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, svc.db.Exec("DROP TABLE projects").Error)
	_, err = svc.GetProjectByComposeName(t.Context(), "alpha")
	require.Error(t, err)
	require.NotErrorIs(t, err, common.ErrNotFound)
	_, _, err = svc.lookupProjectByCachedComposeName(t.Context(), "alpha")
	require.Error(t, err)
	require.NotErrorIs(t, err, common.ErrNotFound)
}
