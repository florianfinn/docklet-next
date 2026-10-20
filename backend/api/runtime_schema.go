// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Optional paired knowledge on generic Activity; required on RuntimeSnapshot.

package api

import (
	"github.com/danielgtaylor/huma/v2"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"reflect"
	"slices"
)

func registerRuntimeSchemasInternal(api huma.API) {
	api.OpenAPI().OnAddOperation = append(api.OpenAPI().OnAddOperation, func(_ *huma.OpenAPI, _ *huma.Operation) {
		registry := api.OpenAPI().Components.Schemas
		for name, schema := range registry.Map() {
			if registry.TypeFromRef("#/components/schemas/"+name) != reflect.TypeFor[activitytypes.Activity]() {
				continue
			}
			schema.Required = slices.DeleteFunc(schema.Required, func(name string) bool { return name == "outcomeState" || name == "cancelRequested" })
			if schema.DependentRequired == nil {
				schema.DependentRequired = map[string][]string{}
			}
			schema.DependentRequired["outcomeState"] = []string{"cancelRequested"}
			schema.DependentRequired["cancelRequested"] = []string{"outcomeState"}
			schema.PrecomputeMessages()
		}
	})
}
