// Source: docklet-next/issues/435
// Revision: 5b0fee810af34673e95c76ffb8af1d49b08c56a7
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original active counts agent contract boundary regression.

package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/florianfinn/docklet-next/types/vulnerability"
	"github.com/stretchr/testify/require"
)

func TestActiveSummariesContractBoundary(t *testing.T) {
	raw, err := os.ReadFile("testdata/agent-hub-v18.json")
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	require.Equal(t, AgentHubFingerprint, hex.EncodeToString(sum[:]))
	var manifest struct {
		Version       int
		Minimum       int `json:"minimumAgentContractVersion"`
		Vulnerability struct {
			Routes        []struct{ Method, Path, OperationID string }
			EdgeRoutes    []struct{ Method, Path, Command string }
			ResponseLimit int
		}
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, 18, manifest.Version)
	require.Equal(t, manifest.Version, manifest.Minimum)
	require.Equal(t, MinimumAgentVersion, manifest.Minimum)
	require.False(t, AgentCompatible(17, "221ff863722bf4dab6cca4e019f5ba5a8fb66b097a7e4a150ddee1e760379450"))
	const path = "/environments/{id}/images/vulnerabilities/active-summaries"
	found, edge := false, false
	for _, r := range manifest.Vulnerability.Routes {
		if r.Path == path {
			require.Equal(t, "POST", r.Method)
			require.Equal(t, "get-active-image-vulnerability-summaries", r.OperationID)
			found = true
		}
	}
	for _, r := range manifest.Vulnerability.EdgeRoutes {
		if r.Path == "/api"+path {
			require.Equal(t, "POST", r.Method)
			require.Equal(t, "vulnerability.active_summaries", r.Command)
			edge = true
		}
	}
	require.True(t, found)
	require.True(t, edge)
	require.Equal(t, vulnerability.ResponseLimit, manifest.Vulnerability.ResponseLimit)
	require.True(t, vulnerability.IsRoute("POST", "/api/environments/0/images/vulnerabilities/active-summaries"))
}
