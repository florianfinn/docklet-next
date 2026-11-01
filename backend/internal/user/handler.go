// Source: getarcaneapp/arcane/backend/internal/user/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; M15 simple-user rights and identity contracts.

package user

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/user"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/validation"
)

// UserHandler handles user management endpoints.
type UserHandler struct {
	cfg                      *config.Config
	userService              *UserService
	invalidateUserTokenCache func(string)
	settingsService          *settings.SettingsService
}

type ListUsersInput struct {
	Search string `query:"search" doc:"Search query"`
	Sort   string `query:"sort" doc:"Column to sort by"`
	Order  string `query:"order" default:"asc" doc:"Sort direction"`
	Start  int    `query:"start" default:"0" doc:"Start index"`
	Limit  int    `query:"limit" default:"20" doc:"Items per page"`
}

type CreateUserInput struct {
	Body user.CreateUser
}

type GetUserInput struct {
	UserID string `path:"userId" doc:"User ID"`
}

type UpdateUserInput struct {
	UserID string `path:"userId" doc:"User ID"`
	Body   user.UpdateUser
}

type DeleteUserInput struct {
	UserID string `path:"userId" doc:"User ID"`
}

type GetUserAvatarInput struct {
	UserID string `path:"userId" doc:"User ID"`
}

type GetUserAvatarOutput struct {
	ContentType         string `header:"Content-Type"`
	CacheControl        string `header:"Cache-Control"`
	XContentTypeOptions string `header:"X-Content-Type-Options"`
	Body                []byte
}

// ListUsers returns a paginated list of users.
func (h *UserHandler) ListUsers(ctx context.Context, input *ListUsersInput) (*handlerutil.Page[user.User], error) {
	params := handlerutil.PaginationParams(input.Start, input.Limit, input.Sort, input.Order, input.Search)

	users, paginationResp, err := h.userService.ListUsersPaginated(ctx, params)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list users: " + err.Error())
	}

	return &handlerutil.Page[user.User]{
		Body: base.Paginated[user.User]{
			Success:    true,
			Data:       users,
			Pagination: handlerutil.PaginationResponse(paginationResp),
		},
	}, nil
}

// CreateUser creates a new usertypes.
func (h *UserHandler) CreateUser(ctx context.Context, input *CreateUserInput) (*handlerutil.Out[user.User], error) {
	normalizedUsername, err := normalizeUsernameInternal(input.Body.Username)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	input.Body.Username = normalizedUsername

	normalizedEmail, err := NormalizeOptionalEmail(input.Body.Email)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	input.Body.Email = normalizedEmail

	proxy := h.cfg != nil && h.cfg.SignInMode() == "proxy"
	if !proxy && strings.Contains(input.Body.Username, "@") {
		return nil, huma.Error400BadRequest("Local usernames cannot contain @")
	}
	if proxy && input.Body.Password != "" {
		return nil, huma.Error400BadRequest("Proxy users have no password")
	}
	passwordPolicy := h.passwordPolicyInternal(ctx)
	if validatePasswordPolicyErr := validation.ValidatePasswordPolicy(input.Body.Password, passwordPolicy); !proxy && validatePasswordPolicyErr != nil {
		return nil, handlerutil.Error400BadRequestWithType(validatePasswordPolicyErr.Error(), validation.PasswordPolicyProblemType(passwordPolicy))
	}

	hashedPassword := ""
	if !proxy {
		hashedPassword, err = h.userService.HashPassword(input.Body.Password)
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to hash password")
	}

	userModel := &User{
		Username:               input.Body.Username,
		PasswordHash:           hashedPassword,
		RequiresPasswordChange: !proxy,
		DisplayName:            input.Body.DisplayName,
		Email:                  input.Body.Email,
		Locale:                 input.Body.Locale,
		TimeFormat:             user.TimeFormatAuto,
		CreatedAt:              time.Now(),
	}
	if input.Body.TimeFormat != nil {
		userModel.TimeFormat = *input.Body.TimeFormat
	}

	createdUser, err := h.userService.CreateUser(ctx, userModel)
	if err != nil {
		if errors.Is(err, ErrUsernameTaken) {
			return nil, huma.Error409Conflict(ErrUsernameTaken.Error())
		}
		return nil, huma.Error500InternalServerError("Failed to create user")
	}

	out, err := h.userService.ToUserResponseDto(ctx, *createdUser)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to map user")
	}

	return &handlerutil.Out[user.User]{
		Body: base.ApiResponse[user.User]{
			Success: true,
			Data:    out,
		},
	}, nil
}

// GetUser returns a user by ID.
func (h *UserHandler) GetUser(ctx context.Context, input *GetUserInput) (*handlerutil.Out[user.User], error) {
	userModel, err := h.userService.GetUserByID(ctx, input.UserID)
	if err != nil {
		return nil, huma.Error404NotFound("User not found")
	}

	rows, err := h.userService.AppAssignments(ctx, input.UserID)
	if err != nil {
		return nil, access.Problem(503, "access.unavailable")
	}
	if clean, changed := access.CleanAssignments(ctx, rows); changed {
		if err := h.userService.SetAppAssignments(ctx, input.UserID, clean); err != nil {
			return nil, access.Problem(503, "access.unavailable")
		}
	}
	out, err := h.userService.ToUserResponseDto(ctx, *userModel)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to map user")
	}

	return &handlerutil.Out[user.User]{
		Body: base.ApiResponse[user.User]{
			Success: true,
			Data:    out,
		},
	}, nil
}

// UpdateUser updates a usertypes.
func (h *UserHandler) UpdateUser(ctx context.Context, input *UpdateUserInput) (*handlerutil.Out[user.User], error) {
	userModel, err := h.userService.GetUserByID(ctx, input.UserID)
	if err != nil {
		return nil, huma.Error404NotFound("User not found")
	}

	normalizedEmail, err := NormalizeOptionalEmail(input.Body.Email)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	input.Body.Email = normalizedEmail

	if input.Body.Username != nil {
		normalizedUsername, normalizeUsernameErr := normalizeUsernameInternal(*input.Body.Username)
		if normalizeUsernameErr != nil {
			return nil, huma.Error400BadRequest(normalizeUsernameErr.Error())
		}
		if (h.cfg == nil || h.cfg.SignInMode() != "proxy") && strings.Contains(normalizedUsername, "@") {
			return nil, huma.Error400BadRequest("Local usernames cannot contain @")
		}
		userModel.Username = normalizedUsername
	}
	if input.Body.DisplayName != nil {
		userModel.DisplayName = input.Body.DisplayName
	}
	if input.Body.Email != nil {
		userModel.Email = input.Body.Email
	}
	if input.Body.Locale != nil {
		userModel.Locale = input.Body.Locale
	}
	if input.Body.TimeFormat != nil {
		userModel.TimeFormat = *input.Body.TimeFormat
	}

	userModel.UpdatedAt = new(time.Now())

	callerPerms, err := h.checkUpdateUserPrivilegesInternal(ctx, input, userModel.ID)
	if err != nil {
		return nil, err
	}

	if input.Body.Password != nil && *input.Body.Password != "" {
		if h.cfg != nil && h.cfg.SignInMode() != "local" {
			return nil, huma.Error403Forbidden("Local password management is disabled")
		}
		passwordPolicy := h.passwordPolicyInternal(ctx)
		if validatePasswordPolicyErr := validation.ValidatePasswordPolicy(*input.Body.Password, passwordPolicy); validatePasswordPolicyErr != nil {
			return nil, handlerutil.Error400BadRequestWithType(validatePasswordPolicyErr.Error(), validation.PasswordPolicyProblemType(passwordPolicy))
		}
		hashedPassword, hashPasswordErr := h.userService.HashPassword(*input.Body.Password)
		if hashPasswordErr != nil {
			return nil, huma.Error500InternalServerError("Failed to hash password")
		}
		userModel.PasswordHash = hashedPassword
		userModel.RequiresPasswordChange = true
	}

	updatedUser, err := h.userService.UpdateUser(ctx, userModel, callerPerms)
	if err != nil {
		if errors.Is(err, ErrInsufficientPrivilege) {
			return nil, huma.Error403Forbidden(ErrInsufficientPrivilege.Error())
		}
		if errors.Is(err, ErrCannotRemoveLastAdmin) {
			return nil, huma.Error409Conflict(ErrCannotRemoveLastAdmin.Error())
		}
		if errors.Is(err, ErrUsernameTaken) {
			return nil, huma.Error409Conflict(ErrUsernameTaken.Error())
		}
		if errors.Is(err, common.ErrUserNotFound) {
			return nil, huma.Error404NotFound("User not found")
		}
		return nil, huma.Error500InternalServerError("Failed to update user")
	}

	if h.invalidateUserTokenCache != nil {
		h.invalidateUserTokenCache(updatedUser.ID)
	}

	out, err := h.userService.ToUserResponseDto(ctx, *updatedUser)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to map user")
	}

	return &handlerutil.Out[user.User]{
		Body: base.ApiResponse[user.User]{
			Success: true,
			Data:    out,
		},
	}, nil
}

// DeleteUser deletes a usertypes.
func (h *UserHandler) DeleteUser(ctx context.Context, input *DeleteUserInput) (*handlerutil.Out[base.MessageResponse], error) {
	// Privilege ordering: a non-admin caller may not delete a global admin
	// target. The service enforces the same check; this pre-check produces a
	// clean 403 without entering the delete path.
	callerPerms, _ := middleware.PermissionsFromContext(ctx)
	caller, _ := userctx.CurrentUserFromContext(ctx)
	if callerPerms != nil && !callerPerms.IsGlobalAdmin() && caller != nil && caller.ID != input.UserID {
		targetPerms, err := h.userService.ResolveUserPermissions(ctx, input.UserID)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to resolve target permissions")
		}
		if targetPerms != nil && targetPerms.IsGlobalAdmin() {
			return nil, huma.Error403Forbidden(ErrInsufficientPrivilege.Error())
		}
	}

	if err := h.userService.DeleteUser(ctx, input.UserID, callerPerms); err != nil {
		if errors.Is(err, ErrInsufficientPrivilege) {
			return nil, huma.Error403Forbidden(ErrInsufficientPrivilege.Error())
		}
		if errors.Is(err, ErrCannotRemoveLastAdmin) {
			return nil, huma.Error409Conflict(ErrCannotRemoveLastAdmin.Error())
		}
		if errors.Is(err, common.ErrUserNotFound) {
			return nil, huma.Error404NotFound("User not found")
		}
		return nil, huma.Error500InternalServerError("Failed to delete user")
	}

	if h.invalidateUserTokenCache != nil {
		h.invalidateUserTokenCache(input.UserID)
	}

	return &handlerutil.Out[base.MessageResponse]{
		Body: base.ApiResponse[base.MessageResponse]{
			Success: true,
			Data: base.MessageResponse{
				Message: "User deleted successfully",
			},
		},
	}, nil
}

// GetUserAvatar returns the custom profile picture for a usertypes.
func (h *UserHandler) GetUserAvatar(ctx context.Context, input *GetUserAvatarInput) (*GetUserAvatarOutput, error) {
	data, mimeType, err := h.userService.GetAvatar(ctx, input.UserID)
	if err != nil {
		if errors.Is(err, common.ErrUserNotFound) {
			return nil, huma.Error404NotFound("User not found")
		}
		return nil, huma.Error500InternalServerError("failed to retrieve avatar")
	}

	if len(data) == 0 {
		return nil, huma.Error404NotFound("user has no custom avatar")
	}

	return &GetUserAvatarOutput{
		ContentType:         mimeType,
		CacheControl:        "public, max-age=3600, stale-while-revalidate=86400",
		XContentTypeOptions: "nosniff",
		Body:                data,
	}, nil
}

func NormalizeOptionalEmail(email *string) (*string, error) {
	if email == nil {
		return nil, nil
	}

	trimmedEmail := strings.TrimSpace(*email)
	if trimmedEmail == "" {
		return nil, nil
	}

	if !validation.IsValidUserEmail(trimmedEmail) {
		return nil, errors.New("must be a valid email")
	}

	return &trimmedEmail, nil
}

// checkUpdateUserPrivilegesInternal enforces that a non-admin caller may not
// modify a global admin target, nor set another user's password. The service
// re-enforces the target-admin check.
func (h *UserHandler) checkUpdateUserPrivilegesInternal(ctx context.Context, input *UpdateUserInput, targetID string) (*authz.PermissionSet, error) {
	callerPerms, _ := middleware.PermissionsFromContext(ctx)
	caller, _ := userctx.CurrentUserFromContext(ctx)
	if callerPerms != nil && !callerPerms.IsGlobalAdmin() {
		if input.Body.Password != nil && *input.Body.Password != "" && caller != nil && caller.ID != targetID {
			return nil, huma.Error403Forbidden(ErrInsufficientPrivilege.Error())
		}
		targetPerms, err := h.userService.ResolveUserPermissions(ctx, targetID)
		if err != nil {
			return nil, huma.Error500InternalServerError("Failed to resolve target permissions")
		}
		if targetPerms != nil && targetPerms.IsGlobalAdmin() {
			return nil, huma.Error403Forbidden(ErrInsufficientPrivilege.Error())
		}
	}
	return callerPerms, nil
}

func (h *UserHandler) passwordPolicyInternal(ctx context.Context) string {
	if h.settingsService == nil {
		return validation.PasswordPolicyStrong
	}
	return h.settingsService.GetStringSetting(ctx, "authPasswordPolicy", validation.PasswordPolicyStrong)
}
