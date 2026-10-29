// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notification

import (
	"bytes"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestLegacyAgentProducersStillUseFanoutWithoutOutboxIdentity(t *testing.T) {
	db, hub := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
	token, key := "example", "key"
	require.NoError(t, db.Create(&environment.Environment{ID: "remote", Name: "Example", Enabled: true, AccessToken: &token, ApiKeyID: &key}).Error)
	cfg := huma.DefaultConfig("test", "1")
	cfg.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(t reflect.Type, h string) string { return t.PkgPath() + "." + huma.DefaultSchemaNamer(t, h) })
	_, api := humatest.New(t, cfg)
	RegisterNotifications(api, hub, &config.Config{})
	calls := 0
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		b, e := io.ReadAll(r.Body)
		require.NoError(t, e)
		require.NotContains(t, string(b), "eventId")
		response := api.Post("/notifications/dispatch", "X-API-Key: "+r.Header.Get("X-API-Key"), "Content-Type: application/json", bytes.NewReader(b))
		require.Equal(t, 200, response.Code, response.Body.String())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	}))
	defer host.Close()
	agent := NewNotificationService(setupNotificationTestDB(t), &config.Config{AgentMode: true, AgentToken: token, ManagerApiUrl: host.URL}, nil, nil)
	_, e := agent.SendImageUpdateNotification(t.Context(), "app:1", newNotificationTestUpdateInfoInternal(), notifications.NotificationEventImageUpdate)
	require.NoError(t, e)
	require.NoError(t, agent.SendPruneReportNotification(t.Context(), &system.PruneAllResult{ContainersPruned: []string{"example-container"}}))
	require.Equal(t, 2, calls)
	var n int64
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.Zero(t, n)
}
