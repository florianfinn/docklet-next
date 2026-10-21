// Source: docklet-next/docs/design/stack-authoring.md
// Revision: e1fda6eb0c13b6b5cd95a59f65836bf68339de61
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original stack authoring capability tests for issue #138.

package activity

import (
	"context"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/stretchr/testify/require"
)

func authoringDescriptorInternal(t *testing.T, permissions *authz.PermissionSet, kind, id, operation string) bool {
	t.Helper()
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, permissions)
	result, err := getCapabilities(ctx, &GetCapabilitiesInput{Kind: kind, ID: id})
	require.NoError(t, err)
	for _, descriptor := range result.Body.Data {
		if descriptor.OperationID == operation {
			require.Equal(t, !descriptor.Available, descriptor.DisabledReasonCode != "")
			return descriptor.Available
		}
	}
	t.Fatalf("missing descriptor %s", operation)
	return false
}

func TestAuthoringCapabilitiesScopeAndPermissions(t *testing.T) {
	full := authz.EnvironmentPermissionSet("example")
	cases := []struct {
		name, kind, id, operation string
		permissions               *authz.PermissionSet
		want                      bool
	}{
		{"new stack on host", "host", "example", "deployStack", full, true},
		{"existing stack deploy", "stack", "example~p1", "deployStack", full, true},
		{"save existing", "stack", "example~p1", "saveStackFiles", full, true},
		{"labels existing", "stack", "example~p1", "setStackLabels", full, true},
		{"convert on host", "host", "example", "convertDockerRun", full, true},
		{"save needs stack ref", "host", "example", "saveStackFiles", full, false},
		{"convert needs host ref", "stack", "example~p1", "convertDockerRun", full, false},
		{"other environment", "stack", "other~p1", "deployStack", full, false},
		{"hub ref", "hub", "0", "deployStack", full, false},
		{"malformed stack ref", "stack", "example~p~1", "saveStackFiles", full, false},
		{"no permissions", "host", "example", "deployStack", authz.EnvironmentPermissionSet("other"), false},
		{"admin", "host", "example", "deployStack", authz.SudoPermissionSet(), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, authoringDescriptorInternal(t, tc.permissions, tc.kind, tc.id, tc.operation))
		})
	}
}
