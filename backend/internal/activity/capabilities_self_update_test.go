// Source: docklet-next/issues/439
// Revision: 289b0d2b52f64855cedad315840e6b3a64ea4d28
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original self-update descriptor matrix.

package activity

import (
	"context"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/stretchr/testify/require"
)

// The hub and each agent can be updated by administrators only; other
// references and non-administrators receive a disabled descriptor.
func TestSelfUpdateDescriptor(t *testing.T) {
	envAdmin := &authz.PermissionSet{PerEnv: map[string]map[string]struct{}{"example": {authz.PermSystemUpgrade: {}, authz.PermEnvironmentsUpdate: {}}, "0": {authz.PermSystemUpgrade: {}}}}
	simpleUser := &authz.PermissionSet{UserID: "example-user", PerEnv: map[string]map[string]struct{}{"example": {authz.PermSystemUpgrade: {}}}}
	for _, tc := range []struct {
		name, kind, id, want string
		ps                   *authz.PermissionSet
	}{
		{"hub admin", "hub", "0", "", authz.SudoPermissionSet()},
		{"agent admin", "host", "example", "", authz.SudoPermissionSet()},
		{"local host ref", "host", "0", "capability.unavailable", authz.SudoPermissionSet()},
		{"scoped ref", "host", "example~app", "capability.unavailable", authz.SudoPermissionSet()},
		{"stack ref", "stack", "example~app", "capability.unavailable", authz.SudoPermissionSet()},
		{"hub environment grants", "hub", "0", "access.admin-required", envAdmin},
		{"agent environment grants", "host", "example", "access.admin-required", envAdmin},
		{"simple user", "host", "example", "access.admin-required", simpleUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, tc.ps)
			out, err := getCapabilities(ctx, &GetCapabilitiesInput{Kind: tc.kind, ID: tc.id})
			require.NoError(t, err)
			found := false
			for _, d := range out.Body.Data {
				if d.OperationID == "selfUpdate" {
					found = true
					require.Equal(t, []any{tc.want == "", tc.want}, []any{d.Available, d.DisabledReasonCode})
				}
			}
			require.True(t, found)
		})
	}
}
