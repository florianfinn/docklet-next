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
	"sync"
	"testing"
	"time"
)

func TestHeldEdgeNotificationRechecksCapturedBinding(t *testing.T) {
	for _, change := range []string{"rotate", "repair", "remove", "reconnect"} {
		t.Run(change, func(t *testing.T) {
			g, e := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
			require.NoError(t, e)
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
			entered, release := make(chan dto.DispatchOrigin, 1), make(chan struct{})
			results := make(chan error, 1)
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			server.SetEventCallback(func(ctx context.Context, o dto.DispatchOrigin, event *edge.TunnelEvent) error {
				entered <- o
				<-release
				e := handleEdgeOriginEventInternal(ctx, o, event, nil, service)
				results <- e
				return e
			})
			router := echo.New()
			router.GET("/connect", server.HandleConnect)
			host := httptest.NewServer(router)
			defer host.Close()
			connect := func() *websocket.Conn {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				conn, _, e := websocket.Dial(ctx, "ws"+strings.TrimPrefix(host.URL, "http")+"/connect", &websocket.DialOptions{HTTPHeader: http.Header{edge.HeaderAgentToken: []string{token}}})
				require.NoError(t, e)
				t.Cleanup(func() { _ = conn.CloseNow() })
				require.NoError(t, wsjson.Write(ctx, conn, &edge.TunnelMessage{Type: edge.MessageTypeRegister, AgentToken: token, AgentInstance: "same-agent"}))
				var registration edge.TunnelMessage
				require.NoError(t, wsjson.Read(ctx, conn, &registration))
				require.True(t, registration.Accepted)
				conn.CloseRead(t.Context())
				return conn
			}
			conn := connect()
			body := json.RawMessage(`{"kind":"auto_heal","autoHeal":{"eventId":"one","outcome":"restarted","container":"web","restarts":1,"windowMinutes":30}}`)
			require.NoError(t, wsjson.Write(t.Context(), conn, &edge.TunnelMessage{Type: edge.MessageTypeEvent, Event: &edge.TunnelEvent{Type: edge.TunnelEventTypeNotificationDispatch, MetadataJSON: body}}))
			var origin dto.DispatchOrigin
			select {
			case origin = <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("event did not reach callback")
			}
			require.Equal(t, edge.AgentTokenHash(token), origin.TokenHash)
			require.Equal(t, key, origin.APIKeyID)
			switch change {
			case "rotate":
				require.NoError(t, db.Model(&env).Update("access_token", "replacement").Error)
			case "repair":
				require.NoError(t, db.Model(&env).Update("api_key_id", "replacement").Error)
			case "remove":
				require.NoError(t, db.Delete(&env).Error)
			case "reconnect":
				_ = connect()
				current, ok := registry.Get(env.ID).Get()
				require.True(t, ok)
				require.NotEqual(t, origin.SessionID, current.SessionID)
			}
			unblock()
			select {
			case e := <-results:
				var p *dto.Problem
				require.ErrorAs(t, e, &p)
				require.Equal(t, "notification.dispatch-stale", p.Code)
			case <-time.After(3 * time.Second):
				t.Fatal("held callback did not settle")
			}
			var n int64
			require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
			require.Zero(t, n)
		})
	}
}
