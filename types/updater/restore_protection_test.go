// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Producer and transport regression proofs for #229 point projection.

package updater

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRestorePointRequiredFieldsAndEnums(t *testing.T) {
	point := RestorePoint{ID: "point", RegistrationID: "registration", TargetID: "stack", TargetIdentity: "identity", SourceRunID: "run", CreatedAt: time.Now().UTC(), SourceState: "settled", Images: []PointImage{}, HoldState: HoldNone}
	body, err := json.Marshal(point)
	require.NoError(t, err)
	invalidUTF8 := bytes.Replace(body, []byte(`"id":"point"`), []byte{'"', 'i', 'd', '"', ':', '"', 0xff, '"'}, 1)
	var invalid RestorePoint
	require.Error(t, json.Unmarshal(invalidUTF8, &invalid))
	var decoded RestorePoint
	require.NoError(t, json.Unmarshal(body, &decoded))
	var fields map[string]any
	require.NoError(t, json.Unmarshal(body, &fields))
	for _, field := range []string{"protectionReason", "holdState", "failedAttempt"} {
		for _, value := range []any{"MISSING", "undeclared", 42} {
			copy := map[string]any{}
			for k, v := range fields {
				copy[k] = v
			}
			if value == "MISSING" {
				delete(copy, field)
			} else {
				copy[field] = value
			}
			bad, e := json.Marshal(copy)
			require.NoError(t, e)
			require.Error(t, json.Unmarshal(bad, &decoded), "%s=%v", field, value)
		}
	}
	for _, a := range []RestorePointHoldState{HoldNone, HoldActive, HoldUnknown} {
		for _, b := range []RestorePointHoldState{HoldNone, HoldActive, HoldUnknown} {
			merged := MergePointHold(a, b)
			if a == HoldUnknown || b == HoldUnknown {
				require.Equal(t, HoldUnknown, merged)
			} else if a == HoldActive || b == HoldActive {
				require.Equal(t, HoldActive, merged)
			} else {
				require.Equal(t, HoldNone, merged)
			}
		}
	}
}
