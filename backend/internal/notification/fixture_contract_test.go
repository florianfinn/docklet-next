// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original producer proof for the web notification target fixture.

package notification

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	"github.com/stretchr/testify/require"
)

// The web parser test reads the same file, so a producer change fails here first.
func TestNotificationTargetListMatchesWebFixture(t *testing.T) {
	_, s := setupNotificationTestServiceInternal(t)
	for _, row := range []struct {
		provider notifications.NotificationProvider
		enabled  bool
		config   database.JSON
	}{
		{"email", true, database.JSON{"smtpHost": "mail.example.com", "smtpPort": 587, "smtpUsername": "sender", "smtpPassword": "example", "fromAddress": "sender@example.com", "toAddresses": []string{"reader@example.com"}, "tlsMode": "starttls", "events": map[string]bool{"deploy_failed": true, "host_offline": true, "image_update": false}}},
		{"ntfy", false, database.JSON{"host": "ntfy.example.com", "topic": "system", "events": map[string]bool{"backup_failed": true}}},
		{"generic", true, database.JSON{"webhookUrl": "https://notify.example.com/hook", "method": "POST", "customHeaders": map[string]string{"X-Example": "visible"}, "events": map[string]bool{"auto_heal": true, "vulnerability_found": false}}},
	} {
		_, err := s.CreateOrUpdateSettings(t.Context(), row.provider, row.enabled, row.config)
		require.NoError(t, err)
	}
	cfg := huma.DefaultConfig("test", "1")
	cfg.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(t reflect.Type, h string) string { return t.PkgPath() + "." + huma.DefaultSchemaNamer(t, h) })
	_, api := humatest.New(t, cfg)
	api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
		next(huma.WithContext(c, context.WithValue(c.Context(), middleware.ContextKeyUserPermissions, &authz.PermissionSet{Sudo: true})))
	})
	RegisterNotifications(api, s, &config.Config{})
	response := api.Get("/environments/0/notifications/settings")
	require.Equal(t, 200, response.Code, response.Body.String())
	fixture, err := os.ReadFile("../../../web/src/data/http/testdata/notification-targets.json")
	require.NoError(t, err)
	require.JSONEq(t, string(fixture), response.Body.String())
}
