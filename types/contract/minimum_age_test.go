// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

package contract

import (
	u "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMinimumAgeContractBoundary(t *testing.T) {
	require.Equal(t, 26, AgentHubVersion)
	require.Equal(t, 26, u.MinimumAgeContractVersion)
	require.Equal(t, 18, MinimumAgentVersion)
	for version, fingerprint := range map[int]string{18: AgentHubV18Fingerprint, 19: AgentHubV19Fingerprint, 20: AgentHubV20Fingerprint, 21: AgentHubV21Fingerprint, 22: AgentHubV22Fingerprint, 23: AgentHubV23Fingerprint, 24: AgentHubV24Fingerprint, 25: AgentHubV25Fingerprint} {
		require.True(t, AgentCompatible(version, fingerprint))
		require.False(t, AgentCompatible(version, "different"))
		if version >= 20 {
			require.True(t, ModuleCompatible(version, fingerprint))
		}
	}
}
