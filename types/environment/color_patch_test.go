// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original absent/null/value colour patch transport contract proof.
package environment

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestColorPatchPreservesAbsentNullAndValue(t *testing.T) {
	for _, decoder := range []struct {
		name   string
		decode func([]byte, any) error
	}{{"json", json.Unmarshal}, {"json-v2", func(body []byte, value any) error { return jsonv2.Unmarshal(body, value) }}} {
		t.Run(decoder.name, func(t *testing.T) {
			var absent, reset, value Update
			require.NoError(t, decoder.decode([]byte(`{}`), &absent))
			require.Nil(t, absent.Color)
			require.NoError(t, decoder.decode([]byte(`{"color":null}`), &reset))
			require.NotNil(t, reset.Color)
			require.Nil(t, *reset.Color)
			require.NoError(t, decoder.decode([]byte(`{"color":"#aabbcc"}`), &value))
			require.NotNil(t, value.Color)
			require.Equal(t, "#aabbcc", **value.Color)
			var combined Update
			require.NoError(t, decoder.decode([]byte(`{"color":null,"name":"Example","enabled":false,"accessToken":"","regenerateApiKey":false}`), &combined))
			require.Equal(t, new("Example"), combined.Name)
			require.Equal(t, new(false), combined.Enabled)
			require.Equal(t, new(""), combined.AccessToken)
			require.Equal(t, new(false), combined.RegenerateApiKey)
			var invalid Update
			require.Error(t, decoder.decode([]byte(`{"color":42}`), &invalid))
			body, err := jsonv2.Marshal(reset)
			require.NoError(t, err)
			require.JSONEq(t, `{"color":null}`, string(body))
		})
	}
}
