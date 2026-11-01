// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package user

import (
	"context"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/validation"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/user"
)

type ResetPasswordInput struct {
	UserID string `path:"userId"`
	Body   struct {
		Password string `json:"password" minLength:"8"`
	}
}
type SetAdminInput struct {
	UserID string `path:"userId"`
	Body   struct {
		IsAdmin bool `json:"isAdmin"`
	}
}
type SetAssignmentsInput struct {
	UserID string `path:"userId"`
	Body   struct {
		Assignments []dto.AppAssignment `json:"assignments"`
	}
}

func managementResult(err error) (*handlerutil.Out[base.MessageResponse], error) {
	if errors.Is(err, ErrCannotRemoveLastAdmin) {
		return nil, huma.Error409Conflict(err.Error())
	}
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	return &handlerutil.Out[base.MessageResponse]{Body: base.ApiResponse[base.MessageResponse]{Success: true, Data: base.MessageResponse{Message: "User updated"}}}, nil
}
func registerUserManagement(api huma.API, h *UserHandler) {
	op := func(id, method, path string) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: "/users/{userId}" + path, Security: handlerutil.DefaultOperationSecurity(), Middlewares: middleware.RequireGlobalAdmin(api), Tags: []string{"Users"}}
	}
	huma.Register(api, op("disable-user", "POST", "/disable"), func(ctx context.Context, in *GetUserInput) (*handlerutil.Out[base.MessageResponse], error) {
		return managementResult(h.userService.SetDisabled(ctx, in.UserID, true))
	})
	huma.Register(api, op("enable-user", "POST", "/enable"), func(ctx context.Context, in *GetUserInput) (*handlerutil.Out[base.MessageResponse], error) {
		return managementResult(h.userService.SetDisabled(ctx, in.UserID, false))
	})
	huma.Register(api, op("reset-user-password", "POST", "/reset-password"), func(ctx context.Context, in *ResetPasswordInput) (*handlerutil.Out[base.MessageResponse], error) {
		if h.cfg != nil && h.cfg.SignInMode() != "local" {
			return nil, huma.Error403Forbidden("Local password management is disabled")
		}
		if err := validation.ValidatePasswordPolicy(in.Body.Password, h.passwordPolicyInternal(ctx)); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return managementResult(h.userService.ResetInitialPassword(ctx, in.UserID, in.Body.Password))
	})
	huma.Register(api, op("set-user-admin", "PUT", "/administrator"), func(ctx context.Context, in *SetAdminInput) (*handlerutil.Out[base.MessageResponse], error) {
		id, _ := middleware.GetUserIDFromContext(ctx)
		return managementResult(h.userService.SetAdministrator(ctx, id, in.UserID, in.Body.IsAdmin))
	})
	huma.Register(api, op("set-user-app-assignments", "PUT", "/app-assignments"), func(ctx context.Context, in *SetAssignmentsInput) (*handlerutil.Out[base.MessageResponse], error) {
		return managementResult(h.userService.SetAppAssignments(ctx, in.UserID, in.Body.Assignments))
	})
}
