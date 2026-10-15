// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original implementation for issue 38.
package version

import (
	"testing"

	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/stretchr/testify/require"
)

func TestAppVersionAdvertisesAgentContract(t *testing.T) {
	service := NewVersionService(nil, true, "dev", "unknown", nil, nil, nil, nil)
	info := service.GetAppVersionInfo(t.Context())
	require.Equal(t, contract.AgentHubVersion, info.AgentContractVersion)
	require.Equal(t, contract.AgentHubFingerprint, info.AgentContractFingerprint)
	require.Equal(t, contract.MinimumAgentVersion, info.MinimumAgentContractVersion)
}
