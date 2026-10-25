// Source: docklet-next/issues/108
// Revision: dd403606fbbf763798d43f4ee854adcb105e01b1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original activity list source-completeness contract and regression evidence.

package activity

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	"reflect"
	"slices"
)

// ListActivitiesOutput adds source completeness metadata only to activity lists.
type ListActivitiesOutput struct {
	HostReason string `header:"X-Docklet-Host-Reason" required:"false" enum:"agent.unavailable" doc:"Present only on HTTP 200 when remote transport is unavailable and the page contains only retained hub summaries (possibly empty). Any marked page makes the whole host read incomplete; retain previous agent observations until a complete successful host read. Omitted on complete local or remote reads and on errors. This is not an observation timestamp or execution outcome."`
	Body       base.Paginated[activity.ActivitySummary]
}

// Match the optional paired knowledge of the embedded generic Activity without
// changing the required runtime snapshot carrier or its strict wire schema.
func registerActivityListSchemaInternal(api huma.API) {
	api.OpenAPI().OnAddOperation = append(api.OpenAPI().OnAddOperation, func(_ *huma.OpenAPI, _ *huma.Operation) {
		registry := api.OpenAPI().Components.Schemas
		for name, schema := range registry.Map() {
			if registry.TypeFromRef("#/components/schemas/"+name) != reflect.TypeFor[activity.ActivitySummary]() {
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
