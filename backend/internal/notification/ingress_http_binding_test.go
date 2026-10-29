// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notification

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestHeldHTTPNotificationRechecksCapturedBinding(t *testing.T) {
	for _, change := range []string{"rotate", "repair", "remove"} {
		t.Run(change, func(t *testing.T) {
			db, s := setupNotificationTestServiceInternal(t)
			require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
			token, key := "example", "key"
			env := environment.Environment{ID: "remote", Name: "Example", Enabled: true, AccessToken: &token, ApiKeyID: &key}
			require.NoError(t, db.Create(&env).Error)
			cfg := huma.DefaultConfig("test", "1")
			cfg.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(t reflect.Type, h string) string { return t.PkgPath() + "." + huma.DefaultSchemaNamer(t, h) })
			_, api := humatest.New(t, cfg)
			RegisterNotifications(api, s, &config.Config{})
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			s.dispatchBarrier = func() { close(entered); <-release }
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				done <- api.Post("/notifications/dispatch", "X-API-Key: "+token, "Content-Type: application/json", strings.NewReader(`{"kind":"backup_failed","backupFailed":{"eventId":"one","operation":"backup","volume":"data","reason":"volume.backup.failed"}}`))
			}()
			<-entered
			switch change {
			case "rotate":
				require.NoError(t, db.Model(&env).Update("access_token", "replacement").Error)
			case "repair":
				require.NoError(t, db.Model(&env).Update("api_key_id", "replacement").Error)
			case "remove":
				require.NoError(t, db.Delete(&env).Error)
			}
			unblock()
			response := <-done
			require.Equal(t, 409, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "notification.dispatch-stale")
			var n int64
			require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
			require.Zero(t, n)
		})
	}
}
