// Source: docklet-next/issues/375
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original browser frame limit tests for both hub bridges.

package ws

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// Both Direct bridges accept a browser frame of exactly BrowserFrameLimit bytes and close the
// browser leg with 1009 for one byte more; the oversized frame never reaches the agent.
func TestBridgesLimitBrowserFrames(t *testing.T) {
	for _, path := range []string{"/api/environments/1/ws/containers/c1/logs", "/api/environments/1/ws/containers/c1/terminal"} {
		t.Run(path, func(t *testing.T) {
			received := make(chan int, 4)
			agentDone, hubDone := make(chan struct{}), make(chan struct{})
			agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(agentDone)
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer func() { _ = conn.CloseNow() }()
				conn.SetReadLimit(-1)
				for {
					_, msg, err := conn.Read(r.Context())
					if err != nil {
						return
					}
					received <- len(msg)
					if len(msg) < 1024 {
						return // the terminal bridge's close frame: end like an exiting shell
					}
				}
			}))
			defer agent.Close()
			remote := "ws" + strings.TrimPrefix(agent.URL, "http")
			hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(hubDone)
				_ = ProxyHTTP(w, r, remote, nil, allowAnyOriginForTest)
			}))
			defer hub.Close()
			browser, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(hub.URL, "http")+path, nil)
			require.NoError(t, err)
			defer func() { _ = browser.CloseNow() }()

			require.NoError(t, browser.Write(t.Context(), websocket.MessageBinary, bytes.Repeat([]byte("a"), BrowserFrameLimit)))
			select {
			case n := <-received:
				require.Equal(t, BrowserFrameLimit, n)
			case <-time.After(5 * time.Second):
				t.Fatal("a frame at the limit must reach the agent")
			}

			require.NoError(t, browser.Write(t.Context(), websocket.MessageBinary, bytes.Repeat([]byte("a"), BrowserFrameLimit+1)))
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			_, _, err = browser.Read(ctx)
			require.Equal(t, websocket.StatusMessageTooBig, websocket.CloseStatus(err), "read error: %v", err)
			// The terminal bridge may still forward its own small close frame to the agent.
			deadline := time.After(200 * time.Millisecond)
			for done := false; !done; {
				select {
				case n := <-received:
					require.Less(t, n, BrowserFrameLimit, "the oversized frame reached the agent")
				case <-deadline:
					done = true
				}
			}
			// Join both bridge legs so no goroutine outlives this test (the package counts goroutines).
			_ = browser.CloseNow()
			for _, done := range []chan struct{}{hubDone, agentDone} {
				select {
				case <-done:
				case <-time.After(10 * time.Second):
					t.Fatal("bridge did not end after the browser closed")
				}
			}
		})
	}
}
