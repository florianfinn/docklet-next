// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Malformed remote point field validation.

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
