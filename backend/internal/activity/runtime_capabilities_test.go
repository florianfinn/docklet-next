// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

package activity

import (
	"context"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/stretchr/testify/require"
)

func TestRuntimeCommandCapabilitiesRequireCompleteScopedGrants(t *testing.T) {
	for _, family := range []struct {
		kind, operation string
		grants          []string
	}{
		{"stack", "runStackCommand", []string{authz.PermActivitiesRead, authz.PermProjectsDeploy, authz.PermProjectsDown, authz.PermProjectsRestart}},
		{"container", "runContainerCommand", []string{authz.PermActivitiesRead, authz.PermContainersStart, authz.PermContainersStop, authz.PermContainersRestart}},
	} {
		for missing := -1; missing < len(family.grants); missing++ {
			permissions := authz.NewPermissionSet()
			for i, grant := range family.grants {
				if i != missing {
					permissions.AddEnv("example-host", grant)
				}
			}
			for _, target := range []struct {
				kind, id  string
				available bool
			}{
				{family.kind, "example-host~example-target", missing == -1},
				{family.kind, "another-host~example-target", false},
				{family.kind, "example-target", false},
				{family.kind, "~example-target", false},
				{family.kind, "example-host~target~invalid", false},
				{"hub", "0", false},
				{"volume", "example-host~example-target", false},
			} {
				ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, permissions)
				result, err := getCapabilities(ctx, &GetCapabilitiesInput{Kind: target.kind, ID: target.id})
				require.NoError(t, err)
				for _, descriptor := range result.Body.Data {
					if descriptor.OperationID == family.operation {
						require.Equal(t, target.available, descriptor.Available, "%s %s missing=%d", target.kind, target.id, missing)
						require.Equal(t, !target.available, descriptor.DisabledReasonCode != "")
					}
					switch descriptor.OperationID {
					case "deployStack", "restartService", "installStackUpdates", "subscribeLogs", "subscribeContainerLogs":
						require.False(t, descriptor.Available)
					}
				}
			}
		}
	}
}

func TestRuntimeCommandCapabilitiesAdminStillNeedsApplicableResource(t *testing.T) {
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
	for _, kind := range []string{"stack", "container", "hub"} {
		result, err := getCapabilities(ctx, &GetCapabilitiesInput{Kind: kind, ID: "example-host~example-target"})
		require.NoError(t, err)
		for _, descriptor := range result.Body.Data {
			if descriptor.OperationID == "runStackCommand" {
				require.Equal(t, kind == "stack", descriptor.Available)
			}
			if descriptor.OperationID == "runContainerCommand" {
				require.Equal(t, kind == "container", descriptor.Available)
			}
		}
	}
}
