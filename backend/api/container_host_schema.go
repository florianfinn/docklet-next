// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M14 container and host operations foundation.

package api

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/types/container"
	"github.com/florianfinn/docklet-next/types/environment"
	"reflect"
)

// Register the complete shared v11 vocabulary before the dependent modules land.
// This adds schemas only; the agent restart route is reserved and remains unregistered.
func registerContainerHostSchemasInternal(api huma.API) {
	refs := []*huma.Schema{}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[container.OriginAssignment](), reflect.TypeFor[container.RecreateRequest](),
		reflect.TypeFor[container.Problem](), reflect.TypeFor[container.ShellQuery](),
		reflect.TypeFor[container.ShellClose](), reflect.TypeFor[container.ShellResize](), reflect.TypeFor[container.ShellExit](),
		reflect.TypeFor[environment.AgentRestartResponse](), reflect.TypeFor[environment.AgentRestartRun](),
		reflect.TypeFor[environment.HostProblem](),
	} {
		refs = append(refs, api.OpenAPI().Components.Schemas.Schema(typ, true, ""))
	}
	if api.OpenAPI().Extensions == nil {
		api.OpenAPI().Extensions = map[string]any{}
	}
	api.OpenAPI().Extensions["x-docklet-container-host-v11"] = refs
}
