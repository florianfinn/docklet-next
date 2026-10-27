// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Pull layer progress preservation and private-field refusal coverage.

package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"uuid"
)

func TestPullProgressRetainsSafeLayerEffects(t *testing.T) {
	secretValue := uuid.New().String()
	input := fmt.Sprintf(`{"status":"Downloading","id":"aabbccddeeff","progress":"synthetic-private-progress","progressDetail":{"current":12,"total":20},"aux":{"secret":%q}}
{"status":"Extracting","id":"aabbccddeeff","progressDetail":{"current":20,"total":20}}
{"status":"synthetic-private-status","id":"synthetic-private-id","progressDetail":{"current":-1,"total":20}}`, secretValue)
	var output bytes.Buffer
	require.NoError(t, WriteSafePullProgress(strings.NewReader(input), &output, false))
	require.NotContains(t, output.String(), "synthetic-private")
	require.NotContains(t, output.String(), secretValue)
	decoder := json.NewDecoder(&output)
	var first, second, third safePullMessage
	require.NoError(t, decoder.Decode(&first))
	require.NoError(t, decoder.Decode(&second))
	require.NoError(t, decoder.Decode(&third))
	require.Equal(t, "Downloading", first.Status)
	require.Equal(t, "aabbccddeeff", first.ID)
	require.Equal(t, &pullProgressCounts{Current: 12, Total: 20}, first.ProgressDetail)
	require.Equal(t, "Extracting", second.Status)
	require.Equal(t, int64(20), second.ProgressDetail.Current)
	require.Equal(t, "Pulling image", third.Status)
	require.Empty(t, third.ID)
	require.Nil(t, third.ProgressDetail)
}
