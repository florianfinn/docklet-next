// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal descriptor rights matrix.

package activity

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
	"github.com/stretchr/testify/require"
)

func TestSettingsCapabilitiesRequireGlobalAdmin(t *testing.T) {
	admin := authz.NewPermissionSet()
	for _, permission := range authz.AllPermissions() {
		admin.Global[permission] = struct{}{}
	}
	partial := authz.NewPermissionSet()
	partial.Global[authz.AllPermissions()[0]] = struct{}{}
	hub, host := capability.ResourceRef{Kind: "hub", ID: "0"}, capability.ResourceRef{Kind: "host", ID: "example"}
	for _, operation := range settingsOperationIDs {
		t.Run(operation, func(t *testing.T) {
			own, foreign := hub, host
			if operation == "getAutoHealSettings" || operation == "saveAutoHealSettings" {
				own, foreign = host, hub
			}
			require.True(t, storageDescriptor(t, admin, own, operation))
			require.False(t, storageDescriptor(t, partial, own, operation))
			require.False(t, storageDescriptor(t, admin, foreign, operation))
			require.False(t, storageDescriptor(t, admin, capability.ResourceRef{Kind: own.Kind, ID: "example~data"}, operation))
		})
	}
}
