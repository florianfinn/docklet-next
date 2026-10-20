// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Exact-byte v4 fingerprint and required runtime evidence vocabulary.

package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestAgentHubV4RuntimeFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/agent-hub-v4.json")
	require.NoError(t, err)
	digest := sha256.Sum256(body)
	require.Equal(t, AgentHubFingerprint, hex.EncodeToString(digest[:]))
	var fixture struct {
		Version    int      `json:"version"`
		Minimum    int      `json:"minimumAgentContractVersion"`
		Operations []string `json:"lifecycleOperations"`
		Identity   []string `json:"identityFields"`
		Knowledge  []string `json:"knowledgeFields"`
		Routes     []string `json:"restrictedRoutes"`
		Streaming  struct {
			Status int      `json:"postFlushFailureStatus"`
			Fields []string `json:"postFlushFailureFields"`
			Done   bool     `json:"doneOnConfirmedSuccessOnly"`
		} `json:"legacyStreaming"`
	}
	require.NoError(t, json.Unmarshal(body, &fixture))
	require.Equal(t, 4, AgentHubVersion)
	require.Equal(t, 4, MinimumAgentVersion)
	require.Equal(t, AgentHubVersion, fixture.Version)
	require.Equal(t, MinimumAgentVersion, fixture.Minimum)
	require.Equal(t, 200, fixture.Streaming.Status)
	require.True(t, fixture.Streaming.Done)
	stream, err := json.Marshal(activity.RuntimeStreamFailure{})
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(stream, &fields))
	require.Len(t, fields, len(fixture.Streaming.Fields))
	for _, name := range fixture.Streaming.Fields {
		require.Contains(t, fields, name)
	}

	require.Equal(t, []string{"deploy-project", "down-project", "restart-project", "redeploy-project", "start-container", "stop-container", "restart-container"}, fixture.Operations)
	for _, tc := range []struct {
		value  any
		fields []string
	}{
		{activity.RuntimeRunIdentity{}, fixture.Identity},
		{activity.RuntimeKnowledge{}, fixture.Knowledge},
	} {
		encoded, err := json.Marshal(tc.value)
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(encoded, &fields))
		require.Len(t, fields, len(tc.fields))
		for _, field := range tc.fields {
			require.Contains(t, fields, field)
		}
	}
	require.False(t, AgentCompatible(3, "39fc19066cb96d807cba48cb01ccf58ed9cfc45f15dbafb8ce2d2bccd7d4015e"))
	require.False(t, AgentCompatible(4, "mismatched"))
	require.True(t, AgentCompatible(4, AgentHubFingerprint))
	for _, route := range []string{"/api/environments/0/runtime-runs/example", "/api/environments/0/runtime-registration", "/api/environments/0/runtime-target"} {
		require.False(t, AgentRecoveryRoute("GET", route))
		require.False(t, AgentRecoveryRoute("POST", route))
	}
}
