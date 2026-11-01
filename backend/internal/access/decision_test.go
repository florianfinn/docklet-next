// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package access

import (
	"context"
	"errors"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"net/url"
	"testing"
)

func testPrincipal(level string) Principal {
	return Principal{ID: "viewer", Assignments: []dto.AppAssignment{{EnvironmentID: "0", ProjectID: "project-uuid", Level: level}}}
}
func TestAssignmentAndLevelOrder(t *testing.T) {
	ctx := t.Context()
	p := testPrincipal("restricted")
	for _, id := range []string{"missing", "foreign", "same-name-new-uuid"} {
		require.Equal(t, NotFound, Decide(ctx, p, "update-project", "0", Params{ProjectID: id}))
	}
	require.Equal(t, FullRequired, Decide(ctx, p, "update-project", "0", Params{ProjectID: "project-uuid"}))
	require.Equal(t, AdminRequired, Decide(ctx, p, "destroy-project", "0", Params{ProjectID: "missing"}))
	for _, body := range []string{`{}`, `null`, ``} {
		require.Equal(t, Allow, Decide(ctx, p, "deploy-project", "0", Params{ProjectID: "project-uuid", Body: []byte(body)}))
	}
	for _, body := range []string{`{"pullPolicy":"missing"}`, `{"forceRecreate":false}`, `{"removeOrphans":false}`, `{"recreateVolumes":false}`} {
		require.Equal(t, FullRequired, Decide(ctx, p, "deploy-project", "0", Params{ProjectID: "project-uuid", Body: []byte(body)}))
		require.Equal(t, Allow, Decide(ctx, testPrincipal("full"), "deploy-project", "0", Params{ProjectID: "project-uuid", Body: []byte(body)}))
	}
}
func TestContainerMembershipUsesFreshRuntime(t *testing.T) {
	p := testPrincipal("restricted")
	id := "new-canonical-id"
	reads := 0
	c := &Controller{Runtime: func(_ context.Context, env, project string) ([]Container, error) {
		reads++
		require.Equal(t, "0", env)
		require.Equal(t, "project-uuid", project)
		return []Container{{ID: id, Names: []string{"/service-name"}}}, nil
	}}
	ctx := WithController(t.Context(), c)
	for _, ref := range []string{id, "service-name", "/service-name"} {
		require.Equal(t, Allow, Decide(ctx, p, "get-container", "0", Params{ContainerID: ref}))
	}
	require.Equal(t, NotFound, Decide(ctx, p, "get-container", "0", Params{ContainerID: "standalone"}))
	id = "redeployed-canonical-id"
	require.Equal(t, Allow, Decide(ctx, p, "get-container", "0", Params{ContainerID: id}))
	require.Equal(t, 5, reads)
	c.Runtime = func(context.Context, string, string) ([]Container, error) { return nil, errors.New("unavailable") }
	require.Equal(t, Unavailable, Decide(ctx, p, "get-container", "0", Params{ContainerID: id}))
}
func TestOwnRunUsesCallerIdentity(t *testing.T) {
	ctx := WithController(t.Context(), &Controller{RunOwner: func(context.Context, string, string) (string, error) { return "viewer", nil }})
	require.Equal(t, Allow, Decide(ctx, testPrincipal("restricted"), "get-hub-runtime-run", "0", Params{RunID: "run"}))
	p := testPrincipal("restricted")
	p.ID = "different-viewer"
	require.Equal(t, NotFound, Decide(ctx, p, "get-hub-runtime-run", "0", Params{RunID: "run"}))
	require.Equal(t, AdminRequired, Decide(ctx, p, "release-runtime-run", "0", Params{RunID: "run"}))
}
func TestProjectListFiltersBeforePagination(t *testing.T) {
	rows := []projectDTO.Details{{ID: "foreign", Name: "Alpha"}, {ID: "project-uuid", Name: "Zebra"}}
	ctx := WithController(t.Context(), &Controller{Projects: func(_ context.Context, _ string, q url.Values) ([]projectDTO.Details, error) {
		require.Empty(t, q.Get("start"))
		require.Empty(t, q.Get("limit"))
		require.Empty(t, q.Get("search"))
		return rows, nil
	}})
	out, err := ProjectList(ctx, testPrincipal("restricted"), "0", url.Values{"limit": {"1"}, "search": {"zeb"}})
	require.NoError(t, err)
	require.Len(t, out.Data, 1)
	require.Equal(t, "project-uuid", out.Data[0].ID)
	require.EqualValues(t, 1, out.Pagination.TotalItems)
	require.EqualValues(t, 1, out.Pagination.GrandTotalItems)
	require.Len(t, rows, 2)
	p := testPrincipal("restricted")
	p.Assignments = nil
	out, err = ProjectList(ctx, p, "0", nil)
	require.NoError(t, err)
	require.Empty(t, out.Data)
	require.Zero(t, out.Pagination.TotalItems)
}
func TestSupportingFullPermissions(t *testing.T) {
	ps := Synthesize("viewer", testPrincipal("full").Assignments)
	for _, grant := range []string{"images:pull", "image-updates:read", "image-updates:check", "activities:read", "projects:update"} {
		require.True(t, ps.Allows(grant, "0"), grant)
	}
	require.False(t, ps.Allows("containers:create", "0"))
	require.False(t, ps.IsGlobalAdmin())
}
