// Source: docklet-next/docs/design/resource-rights.md
// Revision: e009279c6b2029a407abcb9d899c85d06cd30d2f
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original zero-assignment environment list guard (M15 candidate finding DEF-C1).

package environment

import (
	"context"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/environment"
	"github.com/stretchr/testify/require"
)

// A user without assignments sees no environments; administrators still see all.
func TestZeroAssignmentUserListsNoEnvironments(t *testing.T) {
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)
	for _, id := range []string{"first", "second"} {
		require.NoError(t, db.Create(&Environment{ID: id, Name: id, ApiUrl: "https://agent.example.com", Enabled: true, Status: "online", AccessToken: new("example")}).Error)
	}
	h := NewHandler(svc, nil, nil, nil, nil, nil)
	list := func(ps *authz.PermissionSet) *handlerutil.Page[environment.Environment] {
		out, err := h.ListEnvironments(context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, ps), &ListEnvironmentsInput{Limit: 20})
		require.NoError(t, err)
		return out
	}
	user := list(access.Synthesize("viewer", nil))
	require.Empty(t, user.Body.Data)
	require.EqualValues(t, 0, user.Body.Pagination.TotalItems)
	require.Len(t, list(authz.SudoPermissionSet()).Body.Data, 2)
}
