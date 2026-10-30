// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package contract

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestNotificationAutoHealBoundaryVersion(t *testing.T) {
	require.Equal(t, 10, AgentHubVersion)
	require.Equal(t, 10, MinimumAgentVersion)
	b, e := os.ReadFile("testdata/agent-hub-v10.json")
	require.NoError(t, e)
	var fixture struct {
		Minimum int `json:"minimumAgentContractVersion"`
	}
	require.NoError(t, json.Unmarshal(b, &fixture))
	require.Equal(t, MinimumAgentVersion, fixture.Minimum)
}
