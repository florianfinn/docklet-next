// Source: getarcaneapp/arcane/backend/internal/user/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; M15 simple-user rights and identity contracts.

// Package user owns user persistence, password hashing, authorization guards,
// and the user HTTP surface.
package user

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/config"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

type Module struct {
	Config                   *config.Config
	service                  *UserService
	invalidateUserTokenCache func(string)
	settings                 *settings.SettingsService
}

func New(service *UserService, invalidateUserTokenCache func(string), settingsService *settings.SettingsService) *Module {
	return &Module{service: service, invalidateUserTokenCache: invalidateUserTokenCache, settings: settingsService}
}

func (m *Module) Service() *UserService {
	if m == nil {
		return nil
	}
	return m.service
}

func (m *Module) RegisterRoutes(api huma.API) {
	if m == nil {
		RegisterUsers(api, nil, nil, nil)
		return
	}
	registerUsersConfigured(api, m.service, m.invalidateUserTokenCache, m.settings, m.Config)
}

// RegisterUsers registers all user management endpoints.
func RegisterUsers(api huma.API, userService *UserService, invalidateUserTokenCache func(string), settingsService *settings.SettingsService) {
	registerUsersConfigured(api, userService, invalidateUserTokenCache, settingsService, nil)
}
func registerUsersConfigured(api huma.API, userService *UserService, invalidateUserTokenCache func(string), settingsService *settings.SettingsService, cfg *config.Config) {
	h := &UserHandler{userService: userService, invalidateUserTokenCache: invalidateUserTokenCache, settingsService: settingsService, cfg: cfg}
	registerUserManagement(api, h)

	huma.Register(api, huma.Operation{
		OperationID: "listUsers",
		Method:      "GET",
		Path:        "/users",
		Summary:     "List users",
		Description: "Get a paginated list of all users",
		Tags:        []string{"Users"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequirePermission(api, authz.PermUsersList),
	}, h.ListUsers)

	huma.Register(api, huma.Operation{
		OperationID: "createUser",
		Method:      "POST",
		Path:        "/users",
		Summary:     "Create a user",
		Description: "Create a new user account",
		Tags:        []string{"Users"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequirePermission(api, authz.PermUsersCreate),
	}, h.CreateUser)

	huma.Register(api, huma.Operation{
		OperationID: "getUser",
		Method:      "GET",
		Path:        "/users/{userId}",
		Summary:     "Get a user",
		Description: "Get a user by ID",
		Tags:        []string{"Users"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequirePermission(api, authz.PermUsersRead),
	}, h.GetUser)

	huma.Register(api, huma.Operation{
		OperationID: "updateUser",
		Method:      "PUT",
		Path:        "/users/{userId}",
		Summary:     "Update a user",
		Description: "Update an existing user's information",
		Tags:        []string{"Users"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequirePermission(api, authz.PermUsersUpdate),
	}, h.UpdateUser)

	huma.Register(api, huma.Operation{
		OperationID: "deleteUser",
		Method:      "DELETE",
		Path:        "/users/{userId}",
		Summary:     "Delete a user",
		Description: "Delete a user by ID",
		Tags:        []string{"Users"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequirePermission(api, authz.PermUsersDelete),
	}, h.DeleteUser)

	// Unauthenticated by design: profile pictures are publicly visible
	// so they can be displayed without requiring a session token.
	huma.Register(api, huma.Operation{
		OperationID: "getUserAvatar",
		Method:      "GET",
		Path:        "/users/{userId}/avatar",
		Summary:     "Get user avatar",
		Description: "Get the custom profile picture for a user",
		Tags:        []string{"Users"},
		Security:    []map[string][]string{},
	}, h.GetUserAvatar)
}
