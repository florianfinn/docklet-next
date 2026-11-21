// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Malformed remote point field validation. M35 #555: revisionless unavailable status.

package updater

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestIdentityRemotePointDecode(t *testing.T) {
	valid := map[string]any{"service": "app", "reference": "registry.example.com/app:1", "imageId": "sha256:" + strings.Repeat("a", 64), "present": true, "running": false}
	for _, field := range []string{"service", "reference", "imageId", "present", "running", "digest"} {
		for _, value := range []any{nil, 42, "", "bad", "MISSING"} {
			if value == "bad" && (field == "service" || field == "reference") {
				continue
			}
			if field == "digest" && (value == "" || value == "MISSING") {
				continue
			}
			t.Run(field+"/"+stringValue(value), func(t *testing.T) {
				copy := map[string]any{}
				for k, v := range valid {
					copy[k] = v
				}
				if value == "MISSING" {
					delete(copy, field)
				} else {
					copy[field] = value
				}
				b, e := json.Marshal(copy)
				require.NoError(t, e)
				var image PointImage
				require.Error(t, json.Unmarshal(b, &image), "%s=%v", field, value)
			})
		}
	}
	b, e := json.Marshal(valid)
	require.NoError(t, e)
	var image PointImage
	require.NoError(t, json.Unmarshal(b, &image))
}
func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return "null"
	}
	return "number"
}

func TestBaselineStatusValidAcceptsRevisionlessUnavailableOnly(t *testing.T) {
	id, other := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, tc := range []struct {
		name   string
		status BaselineStatus
		valid  bool
	}{
		{"available complete", BaselineStatus{TargetIdentity: id, SavedRevision: id, ObservedRevision: other, Available: true}, true},
		{"available without revisions", BaselineStatus{TargetIdentity: id, Available: true}, false},
		{"available one revision", BaselineStatus{TargetIdentity: id, SavedRevision: id, Available: true}, false},
		{"legacy drift without revisions", BaselineStatus{TargetIdentity: id, Code: "update.config-drift"}, true},
		{"baseline unavailable without revisions", BaselineStatus{TargetIdentity: id, Code: "update.baseline-unavailable"}, true},
		{"drift with current revisions", BaselineStatus{TargetIdentity: id, SavedRevision: id, ObservedRevision: other, Code: "update.config-drift"}, true},
		{"unavailable without code", BaselineStatus{TargetIdentity: id}, false},
		{"unknown code without revisions", BaselineStatus{TargetIdentity: id, Code: "update.check-unavailable"}, false},
		{"unavailable malformed revision", BaselineStatus{TargetIdentity: id, SavedRevision: "bad", Code: "update.config-drift"}, false},
		{"missing target", BaselineStatus{Code: "update.config-drift"}, false},
	} {
		require.Equal(t, tc.valid, tc.status.Valid(), tc.name)
	}
}
