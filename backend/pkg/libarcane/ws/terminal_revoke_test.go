// Source: docklet-next/docs/design/resource-rights.md
// Revision: 8968e3d22a56e0296739d497e1d557e7d3d0a57b
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original Direct terminal revocation input guard.

package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/stretchr/testify/require"
)

// The agent delays its close so input sent after a revoke would still reach it without the guard.
func TestRevokedTerminalViewerSendsNoNewInput(t *testing.T) {
	var mu sync.Mutex
	received := []string{}
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_ = conn.Write(r.Context(), websocket.MessageText, []byte("ready"))
		for {
			kind, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			mu.Lock()
			received = append(received, string(data))
			mu.Unlock()
			if IsTerminalClose(kind, data) {
				time.Sleep(300 * time.Millisecond)
				_ = conn.Close(websocket.StatusNormalClosure, "")
				return
			}
		}
	}))
	defer agent.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := sessionlife.WithSession(r.Context(), "", "example-terminal-user", func(context.Context) error { return nil })
		_ = ProxyHTTP(w, r.WithContext(ctx), "ws"+strings.TrimPrefix(agent.URL, "http"), nil, func(*http.Request) bool { return true })
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/containers/example/terminal", nil)
	require.NoError(t, err)
	defer conn.CloseNow()
	_, body, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "ready", string(body))

	sessionlife.Revoke("", "example-terminal-user", "")
	require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte("after revoke")))
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			break
		}
	}
	require.NoError(t, ctx.Err(), "revocation must close rather than expire")
	mu.Lock()
	defer mu.Unlock()
	require.NotContains(t, received, "after revoke")
}
