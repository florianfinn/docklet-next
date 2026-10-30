// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original operation-level host problem declarations.

package handlerutil

import (
	"github.com/danielgtaylor/huma/v2"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"reflect"
)

// HostProblemResponses retains existing errors alongside the typed host refusal.
func HostProblemResponses(api huma.API, op *huma.Operation, statuses ...string) {
	host := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.HostProblem](), true, "")
	generic := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[huma.ErrorModel](), true, "")
	if op.Responses == nil {
		op.Responses = map[string]*huma.Response{}
	}
	for _, status := range statuses {
		response := op.Responses[status]
		if response == nil {
			response = &huma.Response{Description: "Host or framework refusal"}
			op.Responses[status] = response
		}
		if response.Content == nil {
			response.Content = map[string]*huma.MediaType{}
		}
		media := response.Content["application/problem+json"]
		if media == nil {
			media = &huma.MediaType{Schema: generic}
		}
		media.Schema = &huma.Schema{AnyOf: []*huma.Schema{media.Schema, host}}
		response.Content["application/problem+json"] = media
	}
}
