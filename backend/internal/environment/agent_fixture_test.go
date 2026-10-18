// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package environment

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

func serveCompatibleAgentCheck(t *testing.T, w http.ResponseWriter, r *http.Request) bool {
	t.Helper()
	if r.URL.Path == "/api/environments/0/system/health" {
		require.Equal(t, http.MethodHead, r.Method)
		require.NotEmpty(t, r.Header.Get("X-Arcane-Agent-Token"))
		require.Equal(t, r.Header.Get("X-Arcane-Agent-Token"), r.Header.Get("X-API-Key"))
		w.WriteHeader(http.StatusOK)
		return true
	}
	if r.URL.Path != "/api/app-version" {
		return false
	}
	require.Equal(t, http.MethodGet, r.Method)
	require.NoError(t, json.NewEncoder(w).Encode(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}))
	return true
}
