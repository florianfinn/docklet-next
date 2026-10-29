// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package bootstrap

import (
	"context"
	"encoding/json"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotificationDispatchThroughRealEdgeEventPath(t *testing.T) {
	g, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	db := &database.DB{DB: g}
	require.NoError(t, db.AutoMigrate(&environment.Environment{}, &notificationoutbox.Row{}, &notification.NotificationSettings{}))
	token, key := "example", "key"
	env := environment.Environment{ID: "remote", Name: "Example", Enabled: true, IsEdge: true, AccessToken: &token, ApiKeyID: &key}
	require.NoError(t, db.Create(&env).Error)
	service := notification.NewNotificationService(db, &config.Config{}, nil, nil)
	registry := edge.NewTunnelRegistry()
	service.SetDispatchSessionCurrent(func(o dto.DispatchOrigin) bool {
		current, ok := registry.Get(o.EnvironmentID).Get()
		return ok && current.SessionID == o.SessionID
	})
	server := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return env.ID, nil }, nil)
	server.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return key, nil })
	results := make(chan error, 8)
	origins := make(chan dto.DispatchOrigin, 8)
	server.SetEventCallback(func(ctx context.Context, o dto.DispatchOrigin, e *edge.TunnelEvent) error {
		err := handleEdgeOriginEventInternal(ctx, o, e, nil, service)
		origins <- o
		results <- err
		return err
	})
	router := echo.New()
	router.GET("/connect", server.HandleConnect)
	httpServer := httptest.NewServer(router)
	defer httpServer.Close()
	conn, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(httpServer.URL, "http")+"/connect", &websocket.DialOptions{HTTPHeader: http.Header{edge.HeaderAgentToken: []string{token}}})
	require.NoError(t, err)
	defer conn.CloseNow()
	require.NoError(t, wsjson.Write(t.Context(), conn, &edge.TunnelMessage{Type: edge.MessageTypeRegister, AgentToken: token}))
	var registration edge.TunnelMessage
	require.NoError(t, wsjson.Read(t.Context(), conn, &registration))
	require.True(t, registration.Accepted)
	for _, body := range []string{`{"kind":"backup_failed","backupFailed":{"eventId":"bad space","operation":"backup","volume":"data","reason":"volume.backup.failed"}}`, `{"kind":"backup_failed","backupFailed":{"eventId":"one","operation":"backup","volume":"data","reason":"volume.backup.failed"}}`, `{"kind":"backup_failed","backupFailed":{"eventId":"one","operation":"backup","volume":"data","reason":"volume.backup.failed"}}`, `{"kind":"auto_heal","autoHeal":{"containerName":"old"}}`, `{"kind":"backup_failed","backupFailed":{"extra":true}}`} {
		require.NoError(t, wsjson.Write(t.Context(), conn, &edge.TunnelMessage{Type: edge.MessageTypeEvent, Event: &edge.TunnelEvent{Type: edge.TunnelEventTypeNotificationDispatch, MetadataJSON: json.RawMessage(body)}}))
		err := <-results
		o := <-origins
		require.Equal(t, key, o.APIKeyID)
		require.Equal(t, edge.AgentTokenHash(token), o.TokenHash)
		require.NotEmpty(t, o.SessionID)
		if strings.Contains(body, "extra") || strings.Contains(body, "old") || strings.Contains(body, "bad space") {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
	var n int64
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
}
