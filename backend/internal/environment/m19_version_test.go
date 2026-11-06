// Source: docklet-next/issues/353
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original v14 refusal through the shared agent access envelope.

package environment

import (
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestM19V14RefusedBeforeAutoHeal(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newAgentFixture(t, mode, version.Info{AgentContractVersion: 14, AgentContractFingerprint: contract.AgentHubFingerprint})
			_, err := f.service.ExecuteRemoteRequest(t.Context(), f.id, "GET", "/api/environments/0/auto-heal", nil)
			require.ErrorIs(t, err, ErrAgentUpdateRequired)
			require.Zero(t, f.count("GET /api/environments/0/auto-heal"))
		})
	}
}
