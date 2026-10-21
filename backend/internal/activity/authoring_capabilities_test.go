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

func TestDeployCapabilityMatchesRuntimeAdmission(t *testing.T) {
	partial := &authz.PermissionSet{PerEnv: map[string]map[string]struct{}{"example": {
		authz.PermProjectsCreate: {}, authz.PermProjectsUpdate: {}, authz.PermProjectsDeploy: {}, authz.PermActivitiesRead: {},
	}}}
	// Without down/restart the guarded up admission is refused, so deploy must not be advertised.
	require.False(t, authoringDescriptorInternal(t, partial, "host", "example", "deployStack"))
	require.False(t, authoringDescriptorInternal(t, partial, "stack", "example~p1", "deployStack"))
	require.False(t, authoringDescriptorInternal(t, partial, "stack", "example~p1", "runStackCommand"))
	require.True(t, authoringDescriptorInternal(t, partial, "stack", "example~p1", "saveStackFiles"))
}

func TestHubDeployEntryNeedsOneUsableEnvironment(t *testing.T) {
	all := map[string]struct{}{authz.PermProjectsCreate: {}, authz.PermActivitiesRead: {}, authz.PermProjectsDeploy: {}, authz.PermProjectsDown: {}, authz.PermProjectsRestart: {}}
	runtime := map[string]struct{}{authz.PermActivitiesRead: {}, authz.PermProjectsDeploy: {}, authz.PermProjectsDown: {}, authz.PermProjectsRestart: {}}
	split := &authz.PermissionSet{PerEnv: map[string]map[string]struct{}{"h1": {authz.PermProjectsCreate: {}}, "h2": runtime}}
	require.False(t, authoringDescriptorInternal(t, split, "hub", "0", "deployStack"))
	mixed := &authz.PermissionSet{Global: map[string]struct{}{authz.PermProjectsCreate: {}}, PerEnv: map[string]map[string]struct{}{"h2": runtime}}
	require.True(t, authoringDescriptorInternal(t, mixed, "hub", "0", "deployStack"))
	require.True(t, authoringDescriptorInternal(t, &authz.PermissionSet{Global: all}, "hub", "0", "deployStack"))
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
		{"hub entry point", "hub", "0", "deployStack", full, true},
		{"hub entry point with rights elsewhere", "hub", "0", "deployStack", authz.EnvironmentPermissionSet("other"), true},
		{"hub entry point without rights", "hub", "0", "deployStack", &authz.PermissionSet{}, false},
		{"hub save", "hub", "0", "saveStackFiles", full, false},
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
