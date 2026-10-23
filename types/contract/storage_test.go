// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 60d2471b62b12e1af64c54060a50102a2e39d1c1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original v5 storage checks for #179; preserved historical fingerprint at v6.
package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentHubV5StorageBoundary(t *testing.T) {
	body, err := os.ReadFile("testdata/agent-hub-v5.json")
	require.NoError(t, err)
	digest := sha256.Sum256(body)
	require.Equal(t, "eed087ecf9125541a6dbb3208322578ce83c8c59bf864e97603fd632cca0fe65", hex.EncodeToString(digest[:]))
	var fixture struct {
		Version      int    `json:"version"`
		Minimum      int    `json:"minimumAgentContractVersion"`
		EdgeProtocol string `json:"edgeProtocol"`
	}
	require.NoError(t, json.Unmarshal(body, &fixture))
	require.False(t, AgentCompatible(5, "eed087ecf9125541a6dbb3208322578ce83c8c59bf864e97603fd632cca0fe65"))
	require.Equal(t, 5, fixture.Version)
	require.Equal(t, 5, fixture.Minimum)
	require.Equal(t, "tunnel/v1 (unchanged)", fixture.EdgeProtocol)
	require.False(t, AgentCompatible(4, "f8e019e4d431e6e489051935e4ecbd2e85cd86a1edb32cd128a5ea1d988bfff3"))
	require.True(t, AgentCompatible(AgentHubVersion, AgentHubFingerprint))
	require.False(t, AgentCompatible(AgentHubVersion, "different"))
}
