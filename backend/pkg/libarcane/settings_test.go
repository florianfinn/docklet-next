// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: c477a3d9aba28f8ae7ab6683b56e380a36fa1aa1c6d8e7de3bffeb4b61b7305f
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original vulnerability execution, persistence and contract foundation for issue #260.

package libarcane

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVulnerabilityRetiredKeysNeverSync(t *testing.T) {
	keys := TimeoutSettingKeys()
	require.NotContains(t, keys, "trivyConcurrentScanContainers")
	require.False(t, IsCronSettingKey("vulnerabilityScanInterval"))
	for _, key := range []string{"trivyResourceLimitsEnabled", "trivyCpuLimit", "trivyMemoryLimitMb", "trivyScanTimeout", "proxyRequestTimeout"} {
		require.Contains(t, keys, key)
	}
	for _, key := range []string{"trivyIgnoreUnfixed", "vulnerabilityScanEnabled", "vulnerabilityScanSchedule"} {
		require.NotContains(t, keys, key)
	}
}
