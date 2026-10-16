// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original detached WebSocket maintenance regression test for issue #32.

package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	systemtypes "github.com/florianfinn/docklet-next/types/system"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	hubbackup "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
)

func TestMaintenanceClosesDetachedWebSocket(t *testing.T) {
	gate := hubbackup.NewMaintenanceGate()
	handler := &WebSocketHandler{
		maintenance:   gate,
		wsMetrics:     wshub.NewWebSocketMetrics(),
		checkWSOrigin: func(*http.Request) bool { return true },
	}
	hub := wshub.NewHub(4)
	hubCtx, stopHub := context.WithCancel(t.Context())
	defer stopHub()
	go hub.Run(hubCtx)
	router := echo.New()
	router.GET("/ws", func(c *echo.Context) error {
		conn, unregister, accepted := handler.acceptWSInternal(c, systemtypes.WSKindContainerStats, "example")
		if accepted {
			if !wshub.ServeClientWithOnRemove(context.WithoutCancel(c.Request().Context()), hub, conn, unregister) {
				_ = conn.CloseNow()
				unregister()
			}
		}
		return nil
	})
	server := httptest.NewServer(router)
	defer server.Close()
	conn, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	require.NoError(t, err)
	defer conn.CloseNow()
	require.NoError(t, gate.Begin("activity-1"))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, gate.Wait(ctx))
	_, _, err = conn.Read(ctx)
	require.Error(t, err)
}
