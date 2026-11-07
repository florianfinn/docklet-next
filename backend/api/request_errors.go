// Source: docklet-next/issues/358
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original M20 removal and refusal contracts.

package api

import (
	"github.com/danielgtaylor/huma/v2"
	requestDTO "github.com/florianfinn/docklet-next/types/request"
	"reflect"
	"strings"
)

// Environment requests can be refused by the shared proxy before any handler.
func registerRequestErrors(api huma.API) {
	typed := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[requestDTO.Problem](), true, "")
	for path, item := range api.OpenAPI().Paths {
		if !strings.HasPrefix(path, "/environments/") {
			continue
		}
		for _, op := range []*huma.Operation{item.Get, item.Head, item.Post, item.Put, item.Patch, item.Delete, item.Options, item.Trace} {
			if op == nil {
				continue
			}
			if op.Responses == nil {
				op.Responses = map[string]*huma.Response{}
			}
			response := op.Responses["400"]
			if response == nil {
				response = &huma.Response{Description: "Invalid request path"}
				op.Responses["400"] = response
			}
			if response.Content == nil {
				response.Content = map[string]*huma.MediaType{}
			}
			schema := typed
			if media := response.Content["application/json"]; media != nil && media.Schema != nil {
				schema = &huma.Schema{OneOf: []*huma.Schema{media.Schema, typed}}
			}
			response.Content["application/json"] = &huma.MediaType{Schema: schema}
		}
	}
}
