// Source: docklet-next/issues/339
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original self-only UI preference routes.

package auth

import (
	"context"
	"errors"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/preferences"
)

type PutUIPreferencesInput struct {
	Section string `path:"section"`
	Body    dto.Update
}

func (h *AuthHandler) registerUIPreferences(api huma.API) {
	api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[dto.Problem](), true, "PreferencesProblem")
	huma.Register(api, huma.Operation{OperationID: "get-my-ui-preferences", Method: http.MethodGet, Path: "/auth/me/ui-preferences", Summary: "Read personal UI preferences", Tags: []string{"Auth"}, Security: handlerutil.DefaultOperationSecurity()}, h.GetUIPreferences)
	huma.Register(api, huma.Operation{OperationID: "put-my-ui-preferences", Method: http.MethodPut, Path: "/auth/me/ui-preferences/{section}", Summary: "Replace a personal UI preference section", Tags: []string{"Auth"}, Security: handlerutil.DefaultOperationSecurity(), MaxBodyBytes: 128 * 1024, Errors: []int{400, 403, 413}}, h.PutUIPreferences)
}
func preferenceResult(document dto.Document, err error) (*handlerutil.Out[dto.Document], error) {
	if err != nil {
		if p, ok := errors.AsType[*dto.Problem](err); ok {
			return nil, huma.NewError(p.Status, p.Code)
		}
		return nil, huma.Error500InternalServerError("Could not persist UI preferences")
	}
	return &handlerutil.Out[dto.Document]{Body: base.ApiResponse[dto.Document]{Success: true, Data: document}}, nil
}
func (h *AuthHandler) GetUIPreferences(ctx context.Context, _ *struct{}) (*handlerutil.Out[dto.Document], error) {
	id, _ := middleware.GetUserIDFromContext(ctx)
	return preferenceResult(h.userService.UIPreferences(ctx, id))
}
func (h *AuthHandler) PutUIPreferences(ctx context.Context, input *PutUIPreferencesInput) (*handlerutil.Out[dto.Document], error) {
	id, _ := middleware.GetUserIDFromContext(ctx)
	// Check the principal before validating the section or value.
	if _, err := h.userService.UIPreferences(ctx, id); err != nil {
		return preferenceResult(dto.Document{}, err)
	}
	return preferenceResult(h.userService.PutUIPreferences(ctx, id, input.Section, input.Body.Value))
}
