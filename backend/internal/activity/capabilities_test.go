// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original implementation for issue 38.
package activity

import (
	"testing"

	"github.com/florianfinn/docklet-next/types/capability"
	"github.com/stretchr/testify/require"
)

func TestCapabilityDescriptorAvailabilityInvariantInternal(t *testing.T) {
	ref := capability.ResourceRef{Kind: "environment", ID: "env-1"}
	descriptors := capabilityDescriptorsInternal(ref)
	require.Len(t, descriptors, len(adapterOperationIDs)+len(backupOperationIDs)+len(settingsOperationIDs))
	available := map[string]bool{"listImages": true, "listVolumes": true, "getVolume": true, "getTopology": true, "subscribeMetrics": true, "listStacks": true, "getStack": true, "listContainers": true, "listHosts": true, "listActions": true, "listEvents": true, "subscribeActions": true, "subscribeChanges": true}
	for _, descriptor := range descriptors {
		require.Equal(t, ref, descriptor.ResourceRef)
		require.Equal(t, available[descriptor.OperationID], descriptor.Available)
		require.Equal(t, !descriptor.Available, descriptor.DisabledReasonCode != "")
	}
}
