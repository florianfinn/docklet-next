// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Frozen unreleased v17 Compose self-update DTO and recovery contract. M23 #435: active summaries contract coverage.

package contract

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestSelfUpdateV17FrozenProducers(t *testing.T) {
	var fixture struct {
		SelfUpdate struct {
			Routes        map[string]json.RawMessage
			Schemas       map[string]json.RawMessage
			RetiredRoutes []string
			RunKinds      []string
		}
	}
	raw, err := os.ReadFile("testdata/agent-hub-v18.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &fixture))
	var spec struct {
		Paths      map[string]json.RawMessage
		Components struct{ Schemas map[string]json.RawMessage }
	}
	raw, err = os.ReadFile("../../api/openapi.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &spec))
	require.Len(t, fixture.SelfUpdate.Routes, 2)
	require.Len(t, fixture.SelfUpdate.Schemas, 6)
	for path, expected := range fixture.SelfUpdate.Routes {
		require.JSONEq(t, string(expected), string(spec.Paths[path]), path)
	}
	for name, expected := range fixture.SelfUpdate.Schemas {
		require.JSONEq(t, string(expected), string(spec.Components.Schemas[name]), name)
	}
	for _, path := range fixture.SelfUpdate.RetiredRoutes {
		require.NotContains(t, spec.Paths, path)
	}
	require.Equal(t, []string{"hub.update", "agent.update"}, fixture.SelfUpdate.RunKinds)
	require.Equal(t, 18, AgentHubVersion)
	require.Equal(t, 18, MinimumAgentVersion)
	require.False(t, AgentCompatible(16, AgentHubFingerprint))
}
