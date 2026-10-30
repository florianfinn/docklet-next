// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original restart ordinary-command version gate proof.

package environment

import (
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAgentRestartVersionGateZeroSends(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newAgentFixture(t, mode, version.Info{AgentContractVersion: contract.MinimumAgentVersion - 1, AgentContractFingerprint: contract.AgentHubFingerprint})
			_, err := f.service.ExecuteRemoteRequest(t.Context(), f.id, "POST", "/api/system/agent/restart", nil)
			require.ErrorIs(t, err, ErrAgentUpdateRequired)
			require.Zero(t, f.count("POST /api/system/agent/restart"))
		})
	}
}
