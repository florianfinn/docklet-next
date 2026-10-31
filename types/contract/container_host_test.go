// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M14 container and host operations foundation.

package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestAgentHubV11ContainerHostFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/agent-hub-v11.json")
	require.NoError(t, err)
	digest := sha256.Sum256(data)
	require.Equal(t, AgentHubFingerprint, hex.EncodeToString(digest[:]))
	var fixture struct {
		Version    int
		Minimum    int `json:"minimumAgentContractVersion"`
		Operations struct {
			Schemas           map[string]json.RawMessage
			RuntimeOperations []string
		} `json:"containerHostOperations"`
	}
	require.NoError(t, json.Unmarshal(data, &fixture))
	require.Equal(t, 11, fixture.Version)
	require.Equal(t, 11, fixture.Minimum)
	for _, name := range []string{"ContainerOriginAssignment", "ContainerRecreateRequest", "ContainerRecreateResult", "ContainerProblem", "ContainerShellQuery", "ContainerShellClose", "ContainerShellResize", "ContainerShellExit", "EnvironmentAgentRestartResponse", "EnvironmentAgentRestartRun", "EnvironmentHostProblem", "SystemHealthResponse", "ActivityRuntimeOptions", "ActivityRuntimeSnapshot"} {
		require.Contains(t, fixture.Operations.Schemas, name)
	}
	require.JSONEq(t, `{"additionalProperties":false,"properties":{"type":{"enum":["close"],"type":"string"}},"required":["type"],"type":"object"}`, string(fixture.Operations.Schemas["ContainerShellClose"]))
	for _, operation := range []string{"recreate-container", "redeploy-container", "stop-project"} {
		require.Contains(t, fixture.Operations.RuntimeOperations, operation)
	}
	require.True(t, AgentCompatible(11, AgentHubFingerprint))
	require.False(t, AgentCompatible(10, AgentHubFingerprint))
	require.False(t, AgentCompatible(11, "different"))
}
