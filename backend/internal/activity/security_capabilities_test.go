// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262.

package activity

import (
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSecurityCapabilitiesRequireScanPermission(t *testing.T) {
	read := authz.NewPermissionSet()
	read.AddEnv("example", authz.PermVulnsRead)
	ref := capability.ResourceRef{Kind: "host", ID: "example"}
	require.True(t, networkDescriptor(t, read, ref, "getSecurity").Available)
	require.False(t, networkDescriptor(t, read, ref, "runScan").Available)
	scan := authz.NewPermissionSet()
	scan.AddEnv("example", authz.PermVulnsScan)
	for _, ref := range []capability.ResourceRef{{Kind: "host", ID: "example"}, {Kind: "image", ID: "example~sha256:image"}, {Kind: "hub", ID: "0"}} {
		require.True(t, networkDescriptor(t, scan, ref, "runScan").Available)
	}
	for _, ref := range []capability.ResourceRef{{Kind: "host", ID: "other"}, {Kind: "image", ID: "other~sha256:image"}, {Kind: "image", ID: "example~"}, {Kind: "host", ID: "example~image"}, {Kind: "stack", ID: "example~image"}} {
		require.False(t, networkDescriptor(t, scan, ref, "runScan").Available)
	}
	require.False(t, networkDescriptor(t, scan, ref, "getSecurity").Available)
	require.False(t, networkDescriptor(t, scan, ref, "cancelScan").Available)
}
func TestSecuritySettingsCapabilitiesUseHubSettingsPermissions(t *testing.T) {
	for op, grant := range map[string]string{"getScanSettings": authz.PermSettingsRead, "saveScanSettings": authz.PermSettingsWrite, "setScanEnabled": authz.PermSettingsWrite} {
		p := authz.NewPermissionSet()
		p.AddEnv("0", grant)
		require.True(t, networkDescriptor(t, p, capability.ResourceRef{Kind: "hub", ID: "0"}, op).Available)
		require.False(t, networkDescriptor(t, p, capability.ResourceRef{Kind: "host", ID: "0"}, op).Available)
		foreign := authz.NewPermissionSet()
		foreign.AddEnv("example", grant)
		require.False(t, networkDescriptor(t, foreign, capability.ResourceRef{Kind: "hub", ID: "0"}, op).Available)
	}
}
