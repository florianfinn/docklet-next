// Source: docklet-next/docs/design/updates-ui.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #555 test: the policy consumer accepts both shared drift bodies.

package updater

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

// fixtureProxy decodes a fixture body as the environment proxy decodes the agent's answer
// (bootstrap TestDriftBaselineCrossesRealDirectAndEdge proves that equivalence for Direct and Edge).
type fixtureProxy []byte

func (f fixtureProxy) ProxyJSONRequest(_ context.Context, _, _, _ string, _ []byte, out any) error {
	return json.Unmarshal(f, out)
}

func TestDriftFixturePolicyConsumer(t *testing.T) {
	body, err := os.ReadFile("../../../types/updater/testdata/drift-baseline.json")
	require.NoError(t, err)
	var bodies map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &bodies))
	require.Len(t, bodies, 2)
	for name, raw := range bodies {
		t.Run(name, func(t *testing.T) {
			var sent base.ApiResponse[dto.BaselineStatus]
			require.NoError(t, json.Unmarshal(raw, &sent))
			got, err := (&UpdaterService{}).imagePolicyBaseline(fixtureProxy(raw))(t.Context(), "example", "project-uuid")
			require.NoError(t, err)
			require.Equal(t, sent.Data, got)
			require.False(t, got.Available)
			require.Equal(t, "update.config-drift", got.Code)
		})
	}
}
