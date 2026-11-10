// Source: docklet-next/issues/437
// Revision: 2c49b8db789d39b644e27a85a09e277707b4fe12
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original per-occurrence ignore descriptor regression. M21 #437: advisory detail.

package activity

import (
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAdvisoryIgnoreCapabilitiesUseExistingManageGrant(t *testing.T) {
	for _, op := range []string{"ignore-vulnerability", "unignore-vulnerability"} {
		read := authz.NewPermissionSet()
		read.AddEnv("example", authz.PermVulnsRead)
		ref := capability.ResourceRef{Kind: "host", ID: "example"}
		require.False(t, networkDescriptor(t, read, ref, op).Available)
		manage := authz.NewPermissionSet()
		manage.AddEnv("example", authz.PermVulnsManage)
		require.True(t, networkDescriptor(t, manage, ref, op).Available)
		require.False(t, networkDescriptor(t, manage, capability.ResourceRef{Kind: "host", ID: "other"}, op).Available)
	}
}
