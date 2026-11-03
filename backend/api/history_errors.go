// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original history and preference HTTP failure contracts.

package api

import (
	"github.com/danielgtaylor/huma/v2"
	historyDTO "github.com/florianfinn/docklet-next/types/history"
	preferencesDTO "github.com/florianfinn/docklet-next/types/preferences"
	"reflect"
	"strconv"
	"strings"
)

func personalHistoryErrorTransformer(ctx huma.Context, status string, value any) (any, error) {
	code, _ := strconv.Atoi(status)
	if code < 400 {
		return value, nil
	}
	path := ctx.URL().Path
	preferences := strings.Contains(path, "/auth/me/ui-preferences")
	history := strings.Contains(path, "/history/") || strings.HasSuffix(path, "/history") || strings.HasSuffix(path, "/runtime-conditions")
	if !preferences && !history {
		return value, nil
	}
	model, ok := value.(*huma.ErrorModel)
	if !ok {
		return value, nil
	}
	if strings.HasPrefix(model.Detail, "preferences.") {
		return &preferencesDTO.Problem{Code: model.Detail, Status: code}, nil
	}
	if strings.HasPrefix(model.Detail, "history.") {
		return &historyDTO.Problem{Code: model.Detail, Status: code}, nil
	}
	if preferences && (code == 400 || code == 422 || code == 413) {
		problem := "preferences.invalid"
		if code == 413 {
			problem = "preferences.too-large"
		} else {
			code = 400
			ctx.SetStatus(code)
		}
		return &preferencesDTO.Problem{Code: problem, Status: code}, nil
	}
	return value, nil
}

func registerHistoryErrors(api huma.API) {
	for path, item := range api.OpenAPI().Paths {
		preferences := strings.HasPrefix(path, "/auth/me/ui-preferences")
		history := strings.Contains(path, "/history/") || strings.HasSuffix(path, "/history") || strings.HasSuffix(path, "/runtime-conditions")
		if !preferences && !history {
			continue
		}
		typ := reflect.TypeFor[historyDTO.Problem]()
		if preferences {
			typ = reflect.TypeFor[preferencesDTO.Problem]()
		}
		schema := api.OpenAPI().Components.Schemas.Schema(typ, true, "")
		for _, op := range []*huma.Operation{item.Get, item.Put} {
			if op == nil {
				continue
			}
			for _, status := range []string{"400", "403", "413", "502", "503"} {
				previous := op.Responses[status]
				response := &huma.Response{Description: "Typed history or personal preference failure", Content: map[string]*huma.MediaType{"application/json": {Schema: schema}}}
				if previous != nil {
					for kind, media := range previous.Content {
						if kind != "application/json" {
							response.Content[kind] = media
						}
					}
				}
				op.Responses[status] = response
			}
		}
	}
}
