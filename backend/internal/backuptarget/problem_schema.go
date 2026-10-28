// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original shared advertised backup problem vocabulary.

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
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.Problem](), true, "BackupProblem")
	for path, item := range api.OpenAPI().Paths {
		if !strings.HasPrefix(path, "/backup-targets") && dto.VolumeRouteResponseLimit("GET", path) == 0 {
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
				op.Responses[strconv.Itoa(status)] = &huma.Response{Description: "Volume backup operation refused", Content: map[string]*huma.MediaType{"application/problem+json": {Schema: schema}}}
			}
		}
	}
}
