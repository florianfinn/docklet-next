// Source: docklet-next/docs/design/runtime-control.md
// Revision: 793b7fb08935a2e1b83d65aa6e2354a30330cd46d3ac6894b8d13d82938497cf
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Original effective Compose-label consequence regression coverage.

package project

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/project/children/listing"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

// This fixture is also consumed by the real web mappings. Its identity fields
// must come from serialization of both actual service response paths.
func assertComposeResponse(t *testing.T, svc *ProjectService, id, name, label string) {
	t.Helper()
	details, err := svc.GetProjectDetails(t.Context(), id, projecttypes.DetailsOptions{})
	require.NoError(t, err)
	for _, params := range []pagination.QueryParams{
		{Limit: -1}, {Limit: -1, Sort: "status"},
	} {
		rows, _, err := svc.ListProjects(t.Context(), params)
		require.NoError(t, err)
		var row *projecttypes.Details
		for i := range rows {
			if rows[i].ID == id {
				row = &rows[i]
			}
		}
		require.NotNil(t, row, "actual listing must retain project")
		for _, response := range []projecttypes.Details{details, *row} {
			data, err := json.Marshal(response)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, json.Unmarshal(data, &wire))
			require.Equal(t, id, wire["id"])
			require.Equal(t, name, wire["name"])
			if label == "" {
				require.NotContains(t, wire, "composeProjectName")
			} else {
				require.Equal(t, label, wire["composeProjectName"], "actual serialized response must expose effective label")
			}
			if id == "example-project" && name == "alpha" && label == "beta_stack" {
				fixture, err := os.ReadFile("../../../web/src/data/http/project-ownership.test-support.ts")
				require.NoError(t, err)
				var expected map[string]any
				_, literal, found := strings.Cut(string(fixture), "export default ")
				require.True(t, found)
				require.NoError(t, json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimSpace(literal), ";")), &expected))
				for key, value := range expected {
					require.Equal(t, value, wire[key], "shared web fixture field %s", key)
				}
			}
		}
	}
	stored, err := svc.GetProjectFromDatabaseByID(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, name, stored.Name)
}

func TestComposeNameResponseExternalRefreshAndSync(t *testing.T) {
	for _, mode := range []string{"refresh", "sync"} {
		for _, kind := range []string{"literal", "interpolated"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				composeLogDocker(t)
				svc, root := runtimeIdentityFixture(t)
				row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", identityServices)
				for _, label := range []string{"beta_stack", "next_stack", "alpha"} {
					yaml := identityServices
					if label != "alpha" {
						value := label
						if kind == "interpolated" {
							value = "${STACK_NAME}"
						}
						yaml = "name: " + value + "\n" + yaml
					}
					require.NoError(t, os.WriteFile(filepath.Join(row.Path, "compose.yaml"), []byte(yaml), 0600))
					require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte("STACK_NAME="+label+"\n"), 0600))
					if mode == "sync" {
						require.NoError(t, svc.SyncProjectsFromFileSystem(t.Context()))
					} else {
						svc.refreshComposeProjectName(t.Context(), row)
					}
					assertComposeResponse(t, svc, row.ID, "alpha", label)
				}
			})
		}
	}
}

func TestComposeNameResponseCreateAndUpdate(t *testing.T) {
	for _, mode := range []string{"create", "update"} {
		t.Run(mode, func(t *testing.T) {
			composeLogDocker(t)
			svc, _ := runtimeIdentityFixture(t)
			yaml := "name: ${STACK_NAME}\n" + identityServices
			content := yaml
			if mode == "update" {
				content = identityServices
			}
			env := "STACK_NAME=beta_stack\n"
			row, err := svc.CreateProject(t.Context(), "alpha", content, &env, projecttypes.CreateProjectWorkspaceManifest{}, nil, nil, nil, usertypes.Actor{})
			require.NoError(t, err)
			if mode == "update" {
				_, err = svc.UpdateProject(t.Context(), row.ID, nil, &yaml, nil, nil, usertypes.Actor{})
				require.NoError(t, err)
			}
			assertComposeResponse(t, svc, row.ID, "alpha", "beta_stack")
			for _, label := range []string{"next_stack", "alpha"} {
				content = yaml
				if label == "alpha" {
					content = identityServices
				}
				_, err = svc.UpdateProject(t.Context(), row.ID, nil, &content, new("STACK_NAME="+label+"\n"), nil, usertypes.Actor{})
				require.NoError(t, err)
				assertComposeResponse(t, svc, row.ID, "alpha", label)
			}
		})
	}
}

func TestComposeNameResponseRecordedAvailabilityAndLegacy(t *testing.T) {
	composeLogDocker(t)
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", "name: beta_stack\n"+identityServices)
	svc.refreshComposeProjectName(t.Context(), row)
	require.NoError(t, os.WriteFile(filepath.Join(row.Path, ".env"), []byte("BROKEN='unterminated\n"), 0600))
	assertComposeResponse(t, svc, row.ID, "alpha", "beta_stack")
	require.NoError(t, os.RemoveAll(row.Path))
	assertComposeResponse(t, svc, row.ID, "alpha", "beta_stack")
	for _, metadata := range []*string{nil, new("")} {
		for _, pair := range [][2]string{{"Legacy Custom!", "legacycustom"}, {"__--Foo_Bar!", "foo_bar"}, {"!!!", "!!!"}, {"", ""}} {
			require.NoError(t, svc.db.Model(row).Updates(map[string]any{"name": pair[0], "compose_project_name": metadata}).Error)
			assertComposeResponse(t, svc, row.ID, pair[0], pair[1])
		}
	}
}

func TestComposeNameResponseListingEffectiveGroup(t *testing.T) {
	composeLogDocker(t)
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", "name: beta_stack\n"+identityServices)
	svc.refreshComposeProjectName(t.Context(), row)
	svc.listing = listing.New(func(ctx context.Context) ([]container.Summary, error) {
		return []container.Summary{
			{ID: "stale", State: "exited", Image: "registry.example.com/app:1", Labels: map[string]string{"com.docker.compose.project": "alpha", "com.docker.compose.service": "old"}},
			{ID: "current", State: "running", Image: "registry.example.com/app:1", Labels: map[string]string{"com.docker.compose.project": "beta_stack", "com.docker.compose.service": "app"}},
		}, nil
	}, nil, nil)
	rows, _, err := svc.ListProjects(t.Context(), pagination.QueryParams{Limit: -1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, rows[0].RuntimeServices, 1)
	require.Equal(t, "current", rows[0].RuntimeServices[0].ContainerID)
}

func TestComposeNameResponseOwnersAcrossPagesAndSystemAliases(t *testing.T) {
	composeLogDocker(t)
	svc, root := runtimeIdentityFixture(t)
	owner := runtimeIdentityProject(t, svc, root, "z-owner", "alpha", "name: beta_stack\n"+identityServices)
	svc.refreshComposeProjectName(t.Context(), owner)
	alias := runtimeIdentityProject(t, svc, root, "a-alias", "beta_stack", identityServices)
	containers := []container.Summary{{ID: "current", State: "running", Image: "registry.example.com/app:1", Labels: map[string]string{"com.docker.compose.project": "beta_stack", "com.docker.compose.service": "app"}}}
	svc.listing = listing.New(func(context.Context) ([]container.Summary, error) { return containers, nil }, nil, nil)
	for _, params := range []pagination.QueryParams{{Limit: 1, Search: "beta_stack"}, {Limit: -1}, {Limit: -1, Sort: "status"}} {
		rows, _, err := svc.ListProjects(t.Context(), params)
		require.NoError(t, err)
		for _, row := range rows {
			if row.ID == alias.ID {
				require.Empty(t, row.RuntimeServices, "off-page observed owner must beat this alias")
			} else {
				require.Equal(t, owner.ID, row.ID)
				require.Len(t, row.RuntimeServices, 1)
			}
		}
	}
	require.NoError(t, svc.db.Model(owner).Update("is_archived", true).Error)
	rows, _, err := svc.ListProjects(t.Context(), pagination.QueryParams{Limit: -1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].RuntimeServices, "archived observed owners still exclude aliases")
	require.NoError(t, svc.db.Model(owner).Update("is_archived", false).Error)
	// The system agent uses only the old default; it must not hide the actual
	// owner whose stable display/default still happens to be that alias.
	containers = append(containers, container.Summary{ID: "agent", Labels: map[string]string{"com.docker.compose.project": "alpha", "io.docklet.system": "agent"}})
	rows, _, err = svc.ListProjects(t.Context(), pagination.QueryParams{Limit: -1, Search: "alpha"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, owner.ID, rows[0].ID)
}
