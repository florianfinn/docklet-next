// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Review correction with real producer and behavioral guard proofs.

package testfixtures

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func Read(t *testing.T, key string, actual any) {
	t.Helper()
	body, err := json.Marshal(actual)
	require.NoError(t, err)
	raw, err := os.ReadFile("../../../web/src/data/http/testdata/unset-reads.json")
	require.NoError(t, err)
	var fixture map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fixture))
	require.JSONEq(t, string(fixture[key]), string(body))
}
