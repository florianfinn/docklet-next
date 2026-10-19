// Source: docklet-next/docs/design/runtime-control.md
// Revision: cdfb339023d76259b5de81589b8081255683560f7f129b911cab8db4b23a642a
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Original effective Compose-label consequence regression coverage.

package listing

import (
	"context"
	"testing"
	"time"

	"github.com/docker/compose/v5/pkg/api"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/project"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestComposeNameListingObservationPriority(t *testing.T) {
	groups := map[string][]container.Summary{
		"alpha":      {{ID: "stale", Labels: map[string]string{api.WorkingDirLabel: "/srv/projects/alpha"}}},
		"beta_stack": {{ID: "current"}},
	}
	row := project.Record{ID: "owner", Name: "alpha", ComposeProjectName: new("beta_stack"), Path: "/srv/projects/alpha"}
	require.Equal(t, "current", ProjectContainers(row, groups)[0].ID)
	delete(groups, "beta_stack")
	require.Empty(t, ProjectContainers(row, groups), "stale default must not substitute for absent observation")
	known := KnownComposeProjectNames([]project.Record{row})
	require.Contains(t, known, "beta_stack")
	require.NotContains(t, known, "alpha", "orphan default group must remain discoverable")
	for _, metadata := range []*string{nil, new("")} {
		row.Name = "__Legacy Custom!"
		row.ComposeProjectName = metadata
		require.Contains(t, KnownComposeProjectNames([]project.Record{row}), "legacycustom")
		require.Equal(t, "legacy", ProjectContainers(row, map[string][]container.Summary{"legacycustom": {{ID: "legacy"}}})[0].ID)
	}
	row.ComposeProjectName = nil
	row.Name = "missing"
	require.Equal(t, "stale", ProjectContainers(row, groups)[0].ID, "unique working-directory fallback retained")
	groups["other"] = []container.Summary{{ID: "ambiguous", Labels: map[string]string{api.WorkingDirLabel: row.Path}}}
	require.Empty(t, ProjectContainers(row, groups), "ambiguous directory still refuses")
}

func TestComposeNameListingOwnerCollision(t *testing.T) {
	now := time.Now()
	owner := project.Record{ID: "z-owner", Name: "alpha", ComposeProjectName: new("beta_stack"), UpdatedAt: &now}
	alias := project.Record{ID: "a-alias", Name: "Beta_Stack", UpdatedAt: &now}
	svc := New(func(context.Context) ([]container.Summary, error) {
		return []container.Summary{{ID: "current", Image: "registry.example.com/app:1", State: "running", Labels: map[string]string{api.ProjectLabel: "beta_stack", api.ServiceLabel: "web"}}}, nil
	}, nil, nil)
	for _, records := range [][]project.Record{{alias, owner}, {owner, alias}} {
		rows, _ := Rows("", "", records, svc.Snapshot(t.Context()))
		for _, r := range rows {
			if r.ID == owner.ID {
				require.Len(t, r.RuntimeServices, 1)
				require.Equal(t, "current", r.RuntimeServices[0].ContainerID)
			} else {
				require.Empty(t, r.RuntimeServices, "normalized legacy alias cannot steal observed owner")
			}
		}
		counts := svc.StatusCounts(t.Context(), records)
		require.Equal(t, 1, counts.RunningProjects)
	}
}

func TestComposeNameListingDiscoveryDoesNotHideReplacedDefault(t *testing.T) {
	records := []project.Record{{ID: "owner", Name: "alpha", ComposeProjectName: new("beta_stack")}}
	svc := New(nil, nil, func([]string, []project.RuntimeService, map[string]*imagetypes.UpdateInfo) *project.UpdateInfo {
		return &project.UpdateInfo{HasUpdate: true}
	})
	containers := []container.Summary{
		{ID: "orphan", Image: "registry.example.com/app:1", Labels: map[string]string{api.ProjectLabel: "alpha", api.ServiceLabel: "app"}},
		{ID: "current", Image: "registry.example.com/app:1", Labels: map[string]string{api.ProjectLabel: "beta_stack", api.ServiceLabel: "app"}},
	}
	rows := svc.DiscoveredUpdateRows(t.Context(), containers, KnownComposeProjectNames(records), "")
	require.Len(t, rows, 1)
	require.Equal(t, "compose:alpha", rows[0].ID)
	require.Equal(t, "orphan", rows[0].RuntimeServices[0].ContainerID)
}
