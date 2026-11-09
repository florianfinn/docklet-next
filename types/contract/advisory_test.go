// Source: docklet-next/issues/437
// Revision: 2c49b8db789d39b644e27a85a09e277707b4fe12
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original unreleased v17 advisory route catalogue assertion. M21 #437: advisory detail.

package contract

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/types/vulnerability"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestAdvisoryJoinsUnreleasedV17(t *testing.T) {
	b, err := os.ReadFile("testdata/agent-hub-v17.json")
	require.NoError(t, err)
	var manifest struct {
		Version       int
		Vulnerability struct {
			Routes     []struct{ Method, Path, OperationID string }
			EdgeRoutes []struct{ Method, Path, Command string }
		}
	}
	require.NoError(t, json.Unmarshal(b, &manifest))
	require.Equal(t, 17, manifest.Version)
	found, edge := false, false
	for _, r := range manifest.Vulnerability.Routes {
		if r.OperationID == "get-vulnerability-advisory" {
			require.Equal(t, "GET", r.Method)
			require.Equal(t, "/environments/{id}/vulnerabilities/advisories/{key}", r.Path)
			found = true
		}
	}
	for _, r := range manifest.Vulnerability.EdgeRoutes {
		if r.Command == "vulnerability.advisory" {
			require.Equal(t, "GET", r.Method)
			edge = true
		}
	}
	require.True(t, found)
	require.True(t, edge)
	for _, id := range []string{"vendor/advisory", "%", "?", "Ünicode", "MixedCase"} {
		key := vulnerability.AdvisoryKey(id)
		decoded, err := vulnerability.DecodeAdvisoryKey(key)
		require.NoError(t, err)
		require.Equal(t, id, decoded)
		require.True(t, vulnerability.IsRoute("GET", "/api/environments/0/vulnerabilities/advisories/"+key))
	}
}
