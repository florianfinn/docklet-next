// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original v13 producer allowlist and typed metadata schema contract tests.

package api

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHB9AgentAndEdgeHistoryAllowlist(t *testing.T) {
	require.Equal(t, 15, contract.AgentHubVersion)
	require.Equal(t, 15, contract.MinimumAgentVersion)
	for _, cfg := range []*config.Config{{AgentMode: true}, {EdgeAgent: true}} {
		api := setupAPIForSpecMode(cfg)
		sample := api.OpenAPI().Paths["/environments/{id}/history/sample"]
		require.NotNil(t, sample)
		require.Equal(t, "get-history-sample", sample.Get.OperationID)
		for _, path := range []string{"/environments/{id}/history/host", "/environments/{id}/runtime-conditions", "/history/capacity", "/auth/me/ui-preferences"} {
			require.Nil(t, api.OpenAPI().Paths[path], path)
		}
	}
	command, ok := edge.ResolveEdgeCommandName("GET", "/api/environments/0/history/sample", false).Get()
	require.True(t, ok)
	require.Equal(t, "history.sample", command)
	spec := SetupAPIForSpec().OpenAPI()
	raw, err := json.Marshal(spec.Components.Schemas.Map())
	require.NoError(t, err)
	for _, field := range []string{"ConditionEventMetadata", "LoadAlertMetadata", "growthBytesPerDay", "daysUntilFull", "preferences.no-user"} {
		require.Contains(t, string(raw), field)
	}
}
