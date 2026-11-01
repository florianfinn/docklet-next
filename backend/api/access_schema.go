// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package api

import (
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	userDTO "github.com/florianfinn/docklet-next/types/user"
)

// registerAccessSchemaInternal keeps the access refusal shape and its six codes in
// the exported contract; refusals are written by middleware, not declared per operation.
func registerAccessSchemaInternal(api huma.API) {
	ref := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[userDTO.AccessProblem](), true, "")
	if api.OpenAPI().Extensions == nil {
		api.OpenAPI().Extensions = map[string]any{}
	}
	api.OpenAPI().Extensions["x-docklet-access-v1"] = []*huma.Schema{ref}
}
