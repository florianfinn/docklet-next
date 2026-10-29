// Source: getarcaneapp/arcane/backend/internal/notification/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

// Package notification owns notification settings, dispatch, and HTTP routes.
package notification

import (
	dto "github.com/florianfinn/docklet-next/types/notification"
	"net/http"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

type Module struct {
	service *NotificationService
	config  *config.Config
}

func New(service *NotificationService, cfg *config.Config) *Module {
	return &Module{service: service, config: cfg}
}

func (m *Module) Service() *NotificationService {
	if m == nil {
		return nil
	}
	return m.service
}

func (m *Module) RegisterRoutes(api huma.API) {
	if m == nil {
		RegisterNotifications(api, nil, nil)
		return
	}
	RegisterNotifications(api, m.service, m.config)
}

// RegisterNotifications registers notification endpoints.
func RegisterNotifications(api huma.API, notificationSvc *NotificationService, cfg *config.Config) {
	configRefs := []*huma.Schema{}
	for _, t := range []reflect.Type{reflect.TypeFor[dto.ConfigDiscord](), reflect.TypeFor[dto.ConfigEmail](), reflect.TypeFor[dto.ConfigTelegram](), reflect.TypeFor[dto.ConfigSignal](), reflect.TypeFor[dto.ConfigSlack](), reflect.TypeFor[dto.ConfigNtfy](), reflect.TypeFor[dto.ConfigPushover](), reflect.TypeFor[dto.ConfigGotify](), reflect.TypeFor[dto.ConfigMatrix](), reflect.TypeFor[dto.ConfigGoogleChat](), reflect.TypeFor[dto.ConfigGeneric](), reflect.TypeFor[dto.Problem]()} {
		ref := api.OpenAPI().Components.Schemas.Schema(t, true, "")
		if t != reflect.TypeFor[dto.Problem]() {
			configRefs = append(configRefs, ref)
		}
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[dto.Update](), reflect.TypeFor[dto.Response]()} {
		ref := api.OpenAPI().Components.Schemas.Schema(typ, true, "")
		name := strings.TrimPrefix(ref.Ref, "#/components/schemas/")
		api.OpenAPI().Components.Schemas.Map()[name].Properties["config"].AnyOf = configRefs
	}
	h := &NotificationHandler{
		notificationService: notificationSvc,
		config:              cfg,
	}

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "get-all-notification-settings",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/notifications/settings",
		Summary:     "Get all notification settings",
		Tags:        []string{"Notifications"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequireGlobalAdmin(api),
	}, authz.PermNotificationsManage, h.GetAllNotificationSettings)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "get-notification-settings",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/notifications/settings/{provider}",
		Summary:     "Get notification settings by provider",
		Tags:        []string{"Notifications"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequireGlobalAdmin(api),
	}, authz.PermNotificationsManage, h.GetNotificationSettings)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID:      "create-or-update-notification-settings",
		SkipValidateBody: true,
		Method:           http.MethodPost,
		Path:             "/environments/{id}/notifications/settings",
		Summary:          "Create or update notification settings",
		Tags:             []string{"Notifications"},
		Security:         handlerutil.DefaultOperationSecurity(),
		Middlewares:      middleware.RequireGlobalAdmin(api),
	}, authz.PermNotificationsManage, h.CreateOrUpdateNotificationSettings)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "delete-notification-settings",
		Method:      http.MethodDelete,
		Path:        "/environments/{id}/notifications/settings/{provider}",
		Summary:     "Delete notification settings",
		Tags:        []string{"Notifications"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequireGlobalAdmin(api),
	}, authz.PermNotificationsManage, h.DeleteNotificationSettings)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "test-notification",
		Method:      http.MethodPost,
		Path:        "/environments/{id}/notifications/test/{provider}",
		Summary:     "Test notification",
		Tags:        []string{"Notifications"},
		Security:    handlerutil.DefaultOperationSecurity(),
		Middlewares: middleware.RequireGlobalAdmin(api),
	}, authz.PermNotificationsManage, h.TestNotification)

	// Environment tokens are authenticated by ApiKeyAuth and revalidated by the
	// handler. RBAC middleware cannot scope this route because it has no environment ID.
	huma.Register(api, huma.Operation{
		OperationID:      "dispatch-notification",
		SkipValidateBody: true,
		Method:           http.MethodPost,
		Path:             "/notifications/dispatch",
		Summary:          "Dispatch notification from remote agent to manager",
		Tags:             []string{"Notifications"},
		Security:         []map[string][]string{{"ApiKeyAuth": {}}},
	}, h.DispatchNotification)
}
