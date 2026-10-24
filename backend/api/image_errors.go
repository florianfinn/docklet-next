// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package api

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"reflect"
	"strconv"
	"strings"
)

func imageSafetyPath(path string) bool {
	if strings.Contains(path, "/vulnerabilities") {
		return false
	}
	return strings.Contains(path, "/images") || strings.Contains(path, "/image-updates") || strings.HasPrefix(path, "/container-registries") || strings.HasPrefix(path, "/api/container-registries")
}

// Huma validation failures can otherwise echo a write-only credential value.
func imageErrorTransformer(ctx huma.Context, status string, value any) (any, error) {
	if !imageSafetyPath(ctx.URL().Path) {
		return value, nil
	}
	code, err := strconv.Atoi(status)
	if err != nil || code < 400 {
		return value, nil
	}
	if _, ok := value.(*containerregistry.RegistryTestResult); ok {
		return value, nil
	}
	if _, ok := value.(*imageDTO.OperationError); ok {
		return value, nil
	}
	problem := "image.usage-unavailable"
	if strings.Contains(ctx.URL().Path, "container-registries") {
		problem = "registry.unavailable"
	}
	if strings.Contains(ctx.URL().Path, "image-updates") {
		problem = "update.check-unavailable"
	}
	if code == 422 || code == 400 {
		problem = "image.invalid-reference"
		if strings.Contains(ctx.URL().Path, "container-registries") {
			problem = "registry.invalid-config"
		}
		if code == 422 {
			code = 400
			ctx.SetStatus(code)
		}
	}
	if code == 401 || code == 403 {
		return value, nil
	}
	return imageDTO.Failure(problem, code), nil
}
func registerImageSafetyErrors(api huma.API) {
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[imageDTO.OperationError](), true, "")
	for path, item := range api.OpenAPI().Paths {
		upgrade := strings.HasSuffix(path, "/system/upgrade")
		if !imageSafetyPath(path) && !upgrade {
			continue
		}
		for _, op := range []*huma.Operation{item.Get, item.Post, item.Put, item.Delete} {
			if op == nil {
				continue
			}
			statuses := []string{"400", "404", "409", "429", "500", "502", "503"}
			registryTest := strings.Contains(path, "container-registries") && strings.HasSuffix(path, "/test")
			if registryTest {
				statuses = append(statuses, "401")
			}
			if upgrade {
				statuses = []string{"400", "409", "503"}
			}
			for _, status := range statuses {
				errorSchema := schema
				if strings.Contains(path, "container-registries") && strings.HasSuffix(path, "/test") {
					errorSchema = api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[containerregistry.RegistryTestResult](), true, "")
				}
				if upgrade || (registryTest && status == "401") {
					errorSchema = &huma.Schema{OneOf: []*huma.Schema{errorSchema, api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[huma.ErrorModel](), true, "")}}
				}
				op.Responses[status] = &huma.Response{Description: "Typed neutral image or registry failure", Content: map[string]*huma.MediaType{"application/json": {Schema: errorSchema}}}
			}
		}
	}
}
