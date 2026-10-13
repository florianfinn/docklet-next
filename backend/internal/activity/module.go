// Source: getarcaneapp/arcane/backend/internal/activity/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

// Package activity owns background activity persistence, execution tracking,
// streaming, and the HTTP surface used to inspect and cancel work.
package activity

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

type Module struct {
	service *ActivityService
	handler *ActivityHandler
}

func New(service *ActivityService, environment EnvironmentDependencies) *Module {
	return &Module{service: service, handler: NewHandler(service, environment)}
}

func (m *Module) Service() *ActivityService {
	if m == nil {
		return nil
	}
	return m.service
}

func (m *Module) Handler() *ActivityHandler {
	if m == nil {
		return nil
	}
	return m.handler
}

func (m *Module) RegisterRoutes(api huma.API) {
	if m == nil {
		RegisterActivities(api, NewHandler(nil, EnvironmentDependencies{}))
		return
	}
	RegisterActivities(api, m.handler)
}

func RegisterActivities(api huma.API, h *ActivityHandler) {
	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "list-activities",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/activities",
		Summary:     "List background activities",
		Description: "Get current and recent background activities for an environment",
		Tags:        []string{"Activities"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermActivitiesRead, h.ListActivities)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "get-activity",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/activities/{activityId}",
		Summary:     "Get background activity",
		Description: "Get a background activity with its recent output messages",
		Tags:        []string{"Activities"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermActivitiesRead, h.GetActivity)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "cancel-activity",
		Method:      http.MethodPost,
		Path:        "/environments/{id}/activities/{activityId}/cancel",
		Summary:     "Cancel a background activity",
		Description: "Request cancellation of a running or queued background activity",
		Tags:        []string{"Activities"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermActivitiesCancel, h.CancelActivity)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "clear-activity-history",
		Method:      http.MethodDelete,
		Path:        "/environments/{id}/activities/history",
		Summary:     "Clear background activity history",
		Description: "Delete completed background activity history for an environment",
		Tags:        []string{"Activities"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermActivitiesDelete, h.ClearHistory)
}
