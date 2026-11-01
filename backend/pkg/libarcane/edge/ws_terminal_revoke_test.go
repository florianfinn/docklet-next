// Source: docklet-next/docs/design/resource-rights.md
// Revision: 8968e3d22a56e0296739d497e1d557e7d3d0a57b
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original Edge terminal revocation input guard.

package edge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

// The agent delays its close so input sent after a revoke would still reach it without the guard.
func TestRevokedEdgeTerminalViewerSendsNoNewInput(t *testing.T) {
	var mu sync.Mutex
	received := []string{}
	agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		send := func(msg *TunnelMessage) {
			data, _ := json.Marshal(msg)
			_ = conn.Write(r.Context(), websocket.MessageText, data)
		}
		for {
			_, data, readErr := conn.Read(r.Context())
			if readErr != nil {
				return
			}
			var msg TunnelMessage
			_ = json.Unmarshal(data, &msg)
			switch msg.Type {
			case MessageTypeStreamOpen:
				send(&TunnelMessage{ID: msg.ID, Type: MessageTypeStreamData, Body: []byte("ready"), WSMessageType: int(websocket.MessageText)})
			case MessageTypeStreamData:
				mu.Lock()
				received = append(received, string(msg.Body))
				mu.Unlock()
				if wshub.IsTerminalClose(websocket.MessageType(msg.WSMessageType), msg.Body) {
					time.Sleep(300 * time.Millisecond)
					send(&TunnelMessage{ID: msg.ID, Type: MessageTypeStreamClose})
				}
			}
		}
	}))
	defer agentServer.Close()
	agentConn, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(agentServer.URL, "http"), nil)
	require.NoError(t, err)
	tunnel := newWebSocketAgentTunnel("env-ws-revoke", agentConn)
	defer func() { _ = tunnel.CloseWithReason("") }()
	go func() {
		for {
			msg, receiveErr := tunnel.Conn.Receive()
			if receiveErr != nil {
				return
			}
			if req, ok := tunnel.Pending.Load(msg.ID); ok {
				select {
				case req.(*PendingRequest).ResponseCh <- msg:
				default:
				}
			}
		}
	}()
	router := echo.New()
	router.GET("/ws/containers/example/terminal", func(c *echo.Context) error {
		ctx := sessionlife.WithSession(c.Request().Context(), "", "example-edge-terminal-user", func(context.Context) error { return nil })
		c.SetRequest(c.Request().WithContext(ctx))
		return ProxyWebSocketRequest(c, tunnel, "/api/environments/0/ws/containers/example/terminal", func(*http.Request) bool { return true })
	})
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	clientConn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(proxyServer.URL, "http")+"/ws/containers/example/terminal", nil)
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()
	_, body, err := clientConn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "ready", string(body))

	sessionlife.Revoke("", "example-edge-terminal-user", "")
	require.NoError(t, clientConn.Write(ctx, websocket.MessageText, []byte("after revoke")))
	for {
		if _, _, err := clientConn.Read(ctx); err != nil {
			break
		}
	}
	require.NoError(t, ctx.Err(), "revocation must close rather than expire")
	mu.Lock()
	defer mu.Unlock()
	require.NotContains(t, received, "after revoke")
}
