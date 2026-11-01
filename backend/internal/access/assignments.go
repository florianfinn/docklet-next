// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package access

import (
	"context"
	"github.com/florianfinn/docklet-next/types/user"
	"net/url"
	"slices"
)

// CleanAssignments removes stale UUIDs only when a complete fresh host read
// establishes absence. Unreachable hosts retain assignments for operator review.
func CleanAssignments(ctx context.Context, rows []user.AppAssignment) ([]user.AppAssignment, bool) {
	c, _ := ctx.Value(controllerKey{}).(*Controller)
	if c == nil || c.Projects == nil {
		return rows, false
	}
	byHost := map[string][]string{}
	checked := map[string]bool{}
	out := []user.AppAssignment{}
	for _, a := range rows {
		if !checked[a.EnvironmentID] {
			checked[a.EnvironmentID] = true
			projects, err := c.Projects(ctx, a.EnvironmentID, url.Values{"archived": {"all"}})
			if err == nil {
				ids := []string{}
				for _, v := range projects {
					ids = append(ids, v.ID)
				}
				byHost[a.EnvironmentID] = ids
			}
		}
		ids, ok := byHost[a.EnvironmentID]
		if !ok || slices.Contains(ids, a.ProjectID) {
			out = append(out, a)
		}
	}
	return out, len(out) != len(rows)
}
