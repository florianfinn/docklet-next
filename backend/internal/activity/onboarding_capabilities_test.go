// Source: docklet-next/docs/design/identity.md
// Revision: 81439fab6a8ec5232161dc933fbd41bec41885fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original administrator host capability tests.

package activity

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOnboardingCapabilitiesRequireAdmin(t *testing.T) {
	for _, admin := range []bool{false, true} {
		permissions := authz.EnvironmentPermissionSet("example")
		if admin {
			permissions = authz.SudoPermissionSet()
		}
		ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, permissions)
		result, err := getCapabilities(ctx, &GetCapabilitiesInput{Kind: "hub", ID: "0"})
		require.NoError(t, err)
		for _, descriptor := range result.Body.Data {
			switch descriptor.OperationID {
			case "registerHost", "issueAgentToken", "removeHost", "discoverHub":
				require.Equal(t, admin, descriptor.Available)
				require.Equal(t, !admin, descriptor.DisabledReasonCode != "")
			}
		}
	}
}
