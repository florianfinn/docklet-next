// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original nested recovery evidence fingerprint coverage.

package contract

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestRecoveryV18NestedSchemas(t *testing.T) {
	body, err := os.ReadFile("testdata/agent-hub-v18.json")
	require.NoError(t, err)
	var fixture struct {
		ImageLifecycle struct {
			Schemas map[string]struct {
				SHA256 string `json:"sha256"`
			} `json:"schemas"`
		} `json:"imageLifecycle"`
	}
	require.NoError(t, json.Unmarshal(body, &fixture))
	for _, name := range []string{"UpdaterRecoveryResult", "UpdaterAppliedCandidate", "UpdaterVerifiedImageRequest"} {
		require.Len(t, fixture.ImageLifecycle.Schemas[name].SHA256, 64, name)
	}
	require.Equal(t, 18, AgentHubVersion)
	require.Equal(t, 18, MinimumAgentVersion)
}
