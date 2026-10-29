// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Scoped update capability and restore-point projection implementation for #229.

package contract

import (
	"encoding/json"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestRestorePointProjectionContract(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi.json")
	require.NoError(t, err)
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string
				Properties map[string]struct {
					Type any
					Enum []any
				}
			}
		}
	}
	require.NoError(t, json.Unmarshal(body, &spec))
	schema := spec.Components.Schemas["UpdaterRestorePoint"]
	require.NotEmpty(t, schema.Properties)
	require.Equal(t, "array", schema.Properties["images"].Type)
	require.Contains(t, schema.Required, "images")
	require.Contains(t, schema.Properties["protectionReason"].Type, "null")
	require.Equal(t, []any{string(dto.RestoreTarget), string(dto.HealthyBaseline), nil}, schema.Properties["protectionReason"].Enum)
	require.Equal(t, []any{string(dto.HoldNone), string(dto.HoldActive), string(dto.HoldUnknown)}, schema.Properties["holdState"].Enum)
	typ := reflect.TypeFor[dto.RestorePoint]()
	for _, name := range []string{"ProtectionReason", "HoldState", "FailedAttempt"} {
		field, ok := typ.FieldByName(name)
		require.True(t, ok)
		wire := field.Tag.Get("json")
		require.NotContains(t, wire, "omitempty")
		require.Contains(t, schema.Required, wire)
	}
	reasonField, _ := typ.FieldByName("ProtectionReason")
	holdField, _ := typ.FieldByName("HoldState")
	require.Equal(t, []string{string(dto.RestoreTarget), string(dto.HealthyBaseline)}, strings.Split(reasonField.Tag.Get("enum"), ","))
	require.Equal(t, []string{string(dto.HoldNone), string(dto.HoldActive), string(dto.HoldUnknown)}, strings.Split(holdField.Tag.Get("enum"), ","))
	require.Equal(t, "true", reasonField.Tag.Get("nullable"))
	body, err = os.ReadFile("testdata/agent-hub-v10.json")
	require.NoError(t, err)
	var fixture struct {
		ImageLifecycle struct{ RetentionProtections, HoldStates, PointRequiredFields []string }
	}
	require.NoError(t, json.Unmarshal(body, &fixture))
	require.Equal(t, strings.Split(reasonField.Tag.Get("enum"), ","), fixture.ImageLifecycle.RetentionProtections)
	require.Equal(t, strings.Split(holdField.Tag.Get("enum"), ","), fixture.ImageLifecycle.HoldStates)
	require.Equal(t, schema.Required, fixture.ImageLifecycle.PointRequiredFields)
	for _, field := range []string{"protectionReason", "holdState", "failedAttempt"} {
		require.Contains(t, fixture.ImageLifecycle.PointRequiredFields, field)
	}
	for _, locale := range []string{"en", "de"} {
		catalogue, e := os.ReadFile("../../web/src/shared/messages/" + locale + ".ts")
		require.NoError(t, e)
		for _, key := range []string{"restorePoints:", "noRetentionAnchor:", "failedAttempt:", "restore-target", "healthy-baseline", "none:", "active:", "unknown:"} {
			require.Contains(t, string(catalogue), key)
		}
	}
	// The normal contract drift check binds this schema to the generated TS DTO;
	// also assert the actual consumer DTO exposes each new required property.
	ts, err := os.ReadFile("../../web/src/api/generated/schema.ts")
	require.NoError(t, err)
	segment := strings.SplitN(strings.SplitN(string(ts), "\n        UpdaterRestorePoint: {", 2)[1], "};", 2)[0]
	require.Contains(t, segment, "images: components[\"schemas\"][\"UpdaterPointImage\"][];")
	for _, field := range []string{"protectionReason:", "holdState:", "failedAttempt:"} {
		require.Contains(t, segment, field)
	}
}
