// Source: docklet-next/docs/design/network-management.md
// Revision: 81b69b33c80a454465f54a1f7289c1bc5b5a3381
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentHubV6NetworkBoundary(t *testing.T) {
	body, err := os.ReadFile("testdata/agent-hub-v6.json")
	require.NoError(t, err)
	digest := sha256.Sum256(body)
	require.Equal(t, "be97c8565af13b50db70f2447a1aa47cddb38fff918ddf03c04a3670aa046e25", hex.EncodeToString(digest[:]))
	var fixture struct {
		Version      int
		Minimum      int    `json:"minimumAgentContractVersion"`
		EdgeProtocol string `json:"edgeProtocol"`
		Network      struct {
			Routes     []struct{ Method, Path, OperationID string }
			Fields     map[string][]string
			ErrorCodes []string
		}
	}
	require.NoError(t, json.Unmarshal(body, &fixture))
	require.Equal(t, 6, fixture.Version)
	require.Equal(t, 6, fixture.Minimum)
	require.Less(t, fixture.Version, AgentHubVersion)
	require.Less(t, fixture.Minimum, MinimumAgentVersion)
	require.Equal(t, "tunnel/v1 (unchanged)", fixture.EdgeProtocol)
	require.Len(t, fixture.Network.Routes, 9)
	require.Contains(t, fixture.Network.Fields["NetworkCreateOptions"], "enableIPv4")
	require.Equal(t, []string{"aliases", "containerId", "ipv4Address", "ipv6Address"}, fixture.Network.Fields["NetworkConnectContainerRequest"])
	require.Contains(t, fixture.Network.ErrorCodes, "network.overlap")
	require.False(t, AgentCompatible(5, "eed087ecf9125541a6dbb3208322578ce83c8c59bf864e97603fd632cca0fe65"))
	require.False(t, AgentCompatible(6, "be97c8565af13b50db70f2447a1aa47cddb38fff918ddf03c04a3670aa046e25"))
	require.False(t, AgentCompatible(6, "different"))
}
