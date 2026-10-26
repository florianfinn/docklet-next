// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Advisory hub point holds and bounded agent-response validation for #229.

package activity

import (
	"github.com/danielgtaylor/huma/v2"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"reflect"
)

// JSON Schema's enum also constrains null, independently of its type union.
// Keep null as the field's absent-anchor value without adding a Go enum value.
func registerRestorePointSchema(api huma.API) {
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.RestorePoint](), false, "")
	property := schema.Properties["protectionReason"]
	property.Enum = append(property.Enum, nil)
	property.PrecomputeMessages()
}
