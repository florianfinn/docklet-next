// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Advisory hub point holds and bounded agent-response validation for #229.

package activity

import (
	"github.com/danielgtaylor/huma/v2/humatest"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

func TestPointProjectionSchemaNullAnchor(t *testing.T) {
	_, api := humatest.New(t)
	registerRestorePointSchema(api)
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.RestorePoint](), false, "")
	require.Contains(t, schema.Properties["protectionReason"].Enum, nil)
	require.False(t, schema.Properties["images"].Nullable)
	require.Equal(t, "array", schema.Properties["images"].Type)
	require.Equal(t, []any{"restore-target", "healthy-baseline", nil}, schema.Properties["protectionReason"].Enum)
}
