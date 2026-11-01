// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original shared advertised backup problem vocabulary; M16 bind mounts and backup completion.

package backuptarget

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	dto "github.com/florianfinn/docklet-next/types/backup"
)

// RegisterProblemSchemas describes the stable fields actually returned by
// backup handlers; authentication/authorization keep their platform schemas.
func RegisterProblemSchemas(api huma.API) {
	request := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.TargetSet](), true, "BackupTargetSet")
	receipt := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.TargetSetReceipt](), true, "BackupTargetSetReceipt")
	if api.OpenAPI().Extensions == nil {
		api.OpenAPI().Extensions = map[string]any{}
	}
	api.OpenAPI().Extensions["x-agent-volume-target-sync"] = map[string]any{"method": "PUT", "path": "/environments/{id}/volumes/backups/targets", "request": request, "receipt": receipt}
	probeRequest := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.TargetProbeRequest](), true, "BackupTargetProbeRequest")
	probeResult := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.TargetProbeResult](), true, "BackupTargetProbeResult")
	api.OpenAPI().Extensions["x-agent-volume-target-probe"] = map[string]any{"method": "POST", "path": "/environments/{id}/volumes/backups/targets/probe", "request": probeRequest, "response": probeResult}
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.Problem](), true, "BackupProblem")
	for path, item := range api.OpenAPI().Paths {
		if !strings.HasPrefix(path, "/backup-targets") && !strings.Contains(path, "/volumes") {
			continue
		}
		for _, op := range []*huma.Operation{item.Get, item.Post, item.Put, item.Delete, item.Patch} {
			if op == nil {
				continue
			}
			if op.Responses == nil {
				op.Responses = map[string]*huma.Response{}
			}
			for _, status := range []int{400, 404, 409, 413, 422, 500, 502, 503, 504} {
				responseSchema := schema
				if existing := op.Responses[strconv.Itoa(status)]; existing != nil && existing.Content["application/problem+json"] != nil {
					old := existing.Content["application/problem+json"].Schema
					if old != nil {
						responseSchema = old
						if !containsProblemSchema(old, schema.Ref) {
							responseSchema = &huma.Schema{OneOf: []*huma.Schema{old, schema}}
						}
					}
				}
				op.Responses[strconv.Itoa(status)] = &huma.Response{Description: "Volume backup operation refused", Content: map[string]*huma.MediaType{"application/problem+json": {Schema: responseSchema}}}
			}
		}
	}
}

func containsProblemSchema(schema *huma.Schema, ref string) bool {
	if schema.Ref == ref {
		return true
	}
	for _, child := range schema.OneOf {
		if child != nil && containsProblemSchema(child, ref) {
			return true
		}
	}
	return false
}
