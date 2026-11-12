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

// typedFailures lists the statuses at which each M17 operation returns its own problem
// document. Routes are selected by operation id so older routes that share a path
// word keep their contract; a generic error schema stays a oneOf alternative.
var typedFailures = map[string][]string{
	"get-history-sample":     {"502", "503"},
	"get-container-history":  {"400", "503"},
	"get-project-history":    {"400", "503"},
	"get-host-history":       {"400", "503"},
	"get-runtime-conditions": {"503"},
	"get-history-capacity":   {"503"},
	"get-my-ui-preferences":  {"403"},
	"put-my-ui-preferences":  {"400", "403", "413"},
}

func registerHistoryErrors(api huma.API) {
	for _, item := range api.OpenAPI().Paths {
		for _, op := range []*huma.Operation{item.Get, item.Put} {
			if op == nil {
				continue
			}
			statuses, ok := typedFailures[op.OperationID]
			if !ok {
				continue
			}
			typ := reflect.TypeFor[historyDTO.Problem]()
			if strings.Contains(op.OperationID, "ui-preferences") {
				typ = reflect.TypeFor[preferencesDTO.Problem]()
			}
			typed := api.OpenAPI().Components.Schemas.Schema(typ, true, "")
			for _, status := range statuses {
				schema := typed
				response := &huma.Response{Description: "Typed history or personal preference failure", Content: map[string]*huma.MediaType{}}
				if previous := op.Responses[status]; previous != nil {
					response.Description = previous.Description
					for kind, media := range previous.Content {
						response.Content[kind] = media
					}
					if media := previous.Content["application/json"]; media != nil && media.Schema != nil {
						schema = &huma.Schema{OneOf: []*huma.Schema{typed, media.Schema}}
					}
				}
				response.Content["application/json"] = &huma.MediaType{Schema: schema}
				op.Responses[status] = response
			}
		}
	}
}
