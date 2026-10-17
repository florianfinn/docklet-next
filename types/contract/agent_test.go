// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original implementation for issue 38.
package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

func TestAgentHubDetailFixture(t *testing.T) {
	fixture, err := os.ReadFile("testdata/agent-hub-v3.json")
	require.NoError(t, err)
	digest := sha256.Sum256(fixture)
	require.Equal(t, AgentHubFingerprint, hex.EncodeToString(digest[:]))
	var manifest struct {
		Version                     int      `json:"version"`
		OnboardingCheckFields       []string `json:"onboardingCheckFields"`
		MinimumAgentContractVersion int      `json:"minimumAgentContractVersion"`
		ProxiedOperations           []struct {
			OperationID      string   `json:"operationId"`
			DetailFields     []string `json:"detailFields"`
			OutputLineFields []string `json:"outputLineFields"`
		} `json:"proxiedOperations"`
	}
	require.NoError(t, json.Unmarshal(fixture, &manifest))
	require.Equal(t, AgentHubVersion, manifest.Version)
	require.Equal(t, MinimumAgentVersion, manifest.MinimumAgentContractVersion)
	require.Len(t, manifest.ProxiedOperations, 1)
	require.Equal(t, "get-activity", manifest.ProxiedOperations[0].OperationID)
	line := activity.OutputLine{Timestamp: time.Now().UTC(), Stream: "stdout", Text: "raw", Step: "pull", Level: activity.MessageLevelInfo}
	detail := activity.Detail{Output: []activity.OutputLine{line}}
	for _, item := range []struct {
		value  any
		fields []string
	}{
		{version.OnboardingChecks{}, manifest.OnboardingCheckFields},
		{detail, manifest.ProxiedOperations[0].DetailFields},
		{line, manifest.ProxiedOperations[0].OutputLineFields},
	} {
		encoded, err := json.Marshal(item.value)
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(encoded, &fields))
		require.Len(t, fields, len(item.fields))
		for _, field := range item.fields {
			require.Contains(t, fields, strings.TrimSuffix(field, "?"))
		}
	}
}
