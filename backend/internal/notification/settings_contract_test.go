// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notification

import (
	"context"
	"encoding/json/v2"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/stretchr/testify/require"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestProviderSaveEffectiveConfigAndCredentials(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	fixtures := map[notifications.NotificationProvider]database.JSON{
		"discord":  {"webhookId": "123", "token": "example"},
		"email":    {"smtpHost": "mail.example.com", "smtpPort": 587, "smtpUsername": "sender", "smtpPassword": "example", "fromAddress": "sender@example.com", "toAddresses": []string{"reader@example.com"}, "tlsMode": "starttls"},
		"telegram": {"botToken": "example", "chatIds": []string{"123"}},
		"signal":   {"host": "signal.example.com", "port": 443, "token": "example", "source": "sender", "recipients": []string{"reader"}},
		"slack":    {"token": "example"}, "ntfy": {"host": "notify.example.com", "topic": "system", "username": "sender", "password": "example"},
		"pushover": {"token": "example", "user": "reader"}, "gotify": {"host": "notify.example.com", "token": "example"},
		"matrix":     {"host": "matrix.example.com", "rooms": "room", "username": "sender", "password": "example"},
		"googlechat": {"webhookUrl": "https://chat.example.com/message"}, "generic": {"webhookUrl": "https://notify.example.com/message", "customHeaders": map[string]string{"X-Example": "visible"}},
	}
	for p, c := range fixtures {
		t.Run(string(p), func(t *testing.T) {
			row, err := s.CreateOrUpdateSettings(t.Context(), p, true, c)
			require.NoError(t, err)
			fields := credentialsSet(*row)
			redacted := RedactNotificationConfigCredentials(p, row.Config)
			for _, f := range fields {
				require.Empty(t, redacted[f])
				c[f] = ""
			}
			edited, err := s.CreateOrUpdateSettings(t.Context(), p, true, c)
			require.NoError(t, err)
			require.Equal(t, fields, credentialsSet(*edited))
			require.Equal(t, row.ID, edited.ID)
			for _, f := range fields {
				require.Equal(t, row.Config[f], edited.Config[f])
			}
			if p == "generic" {
				require.Equal(t, c["webhookUrl"], redacted["webhookUrl"])
				require.Contains(t, redacted, "customHeaders")
			}
			c["unexpected"] = true
			_, err = s.CreateOrUpdateSettings(t.Context(), p, true, c)
			require.Error(t, err)
			delete(c, "unexpected")
		})
	}
	// The real sender accepts bare hosts, default ntfy host and optional scheme ports.
	for _, host := range []string{"", "notify.example.com", "2001:db8::1"} {
		_, err := s.CreateOrUpdateSettings(t.Context(), "ntfy", true, database.JSON{"host": host, "topic": "system", "username": "sender", "password": "example"})
		require.NoError(t, err)
		u, err := notifications.BuildNtfyURL(notifications.NtfyConfig{Host: host, Topic: "system"})
		require.NoError(t, err)
		require.Contains(t, u, "system")
	}
	for _, c := range []database.JSON{{"host": "https://notify.example.com", "topic": "system"}, {"host": "notify.example.com", "topic": "system", "port": -1}, {"topic": "system", "events": map[string]bool{"unknown": true}}} {
		_, err := s.CreateOrUpdateSettings(t.Context(), "ntfy", true, c)
		require.Error(t, err)
	}
	// Concurrent first saves still have exactly one channel row.
	require.NoError(t, s.DeleteSettings(t.Context(), "ntfy"))
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			_, err := s.CreateOrUpdateSettings(t.Context(), "ntfy", true, database.JSON{"topic": "system"})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, db.Model(&NotificationSettings{}).Where("provider='ntfy'").Count(&count).Error)
	require.EqualValues(t, 1, count)
}
func TestDeliveryErrorsNeverPersistProviderText(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.Create(&NotificationSettings{Provider: "generic", Enabled: true, Config: database.JSON{}}).Error)
	raw := errors.New("HTTP status 401 at https://notify.example.com/credential-marker")
	_, err := s.notifyEnabledProvidersInternal(t.Context(), NotificationTarget{EnvironmentID: "0"}, "image_update", "image_update", database.JSON{}, func(context.Context, notifications.NotificationProvider, database.JSON) (bool, error) {
		return true, raw
	})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "credential-marker")
	var events []event.Event
	require.NoError(t, db.Find(&events).Error)
	require.Len(t, events, 1)
	require.Equal(t, "notification.delivery-failed", string(events[0].Type))
	require.Equal(t, "delivery-auth", events[0].Metadata["reason"])
	b, err := json.Marshal(events)
	require.NoError(t, err)
	require.NotContains(t, string(b), "credential-marker")
	require.NotContains(t, notifications.FormatDeliveryError(raw), "credential-marker")
}
func TestNotificationRoutesRequireGlobalAdminAndLocalID(t *testing.T) {
	_, s := setupNotificationTestServiceInternal(t)
	cfg := huma.DefaultConfig("test", "1")
	cfg.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(t reflect.Type, h string) string { return t.PkgPath() + "." + huma.DefaultSchemaNamer(t, h) })
	_, api := humatest.New(t, cfg)
	api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
		ps := authz.NewPermissionSet()
		ps.Global[authz.PermNotificationsManage] = struct{}{}
		next(huma.WithContext(c, context.WithValue(c.Context(), middleware.ContextKeyUserPermissions, ps)))
	})
	RegisterNotifications(api, s, &config.Config{})
	for _, path := range []string{"/environments/0/notifications/settings", "/environments/0/notifications/test/generic"} {
		var status int
		if strings.Contains(path, "test") {
			status = api.Post(path).Code
		} else {
			status = api.Get(path).Code
		}
		require.Equal(t, http.StatusForbidden, status)
	}
	h := &NotificationHandler{notificationService: s, config: &config.Config{}}
	_, err := h.GetAllNotificationSettings(t.Context(), &GetAllNotificationSettingsInput{EnvironmentID: "remote"})
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, "notification.hub-only", p.Code)
}
