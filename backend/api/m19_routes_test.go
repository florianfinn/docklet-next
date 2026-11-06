// Source: docklet-next/issues/356
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original mode route-set and removed-operation inventory regression guards.

package api

import (
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestM19ImportHubOnlyAndSearchRemovalInventory(t *testing.T) {
	for _, cfg := range []*config.Config{{}, {AgentMode: true}, {EdgeAgent: true}} {
		spec := setupAPIForSpecMode(cfg).OpenAPI()
		require.Equal(t, !cfg.AgentMode && !cfg.EdgeAgent, spec.Paths["/stack-import"] != nil)
		for _, path := range []string{"/settings/search", "/settings/categories", "/customize/search", "/customize/categories"} {
			require.Nil(t, spec.Paths[path])
		}
		for _, id := range []string{"search-settings", "get-settings-categories", "search-customize", "get-customize-categories"} {
			require.NotContains(t, access.Rules, id)
			for _, route := range access.Routes {
				require.NotEqual(t, id, route.Operation)
			}
		}
	}
}
