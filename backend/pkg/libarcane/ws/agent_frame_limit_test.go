// Source: docklet-next/issues/670
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original real-pair agent output frame bounds.

package ws

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/docker"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBridgesLimitAgentFrames(t *testing.T) {
	for _, kind := range []string{"logs", "terminal"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			agentDone, hubDone := make(chan struct{}), make(chan struct{})
			agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(agentDone)
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				conn.SetReadLimit(-1)
				// First prove an exact-limit frame survives, then send one byte too many.
				for _, size := range []int{AgentFrameLimit, AgentFrameLimit + 1} {
					if err = conn.Write(ctx, websocket.MessageBinary, bytes.Repeat([]byte("a"), size)); err != nil {
						return
					}
				}
				_, _, _ = conn.Read(ctx)
			}))
			defer agent.Close()
			hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(hubDone)
				_ = ProxyHTTP(w, r, "ws"+strings.TrimPrefix(agent.URL, "http"), nil, allowAnyOriginForTest)
			}))
			defer hub.Close()
			browser, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(hub.URL, "http")+"/api/environments/1/ws/containers/c1/"+kind, nil)
			require.NoError(t, err)
			defer browser.CloseNow()
			browser.SetReadLimit(-1)
			_, data, err := browser.Read(ctx)
			require.NoError(t, err)
			require.Len(t, data, AgentFrameLimit)
			for {
				_, data, err = browser.Read(ctx)
				if err != nil {
					break
				}
				require.Less(t, len(data), AgentFrameLimit, "oversized frame reached browser")
			}
			require.NoError(t, ctx.Err(), "refusal must precede deadline")
			require.Error(t, err, "oversized frame reached browser (%d bytes)", len(data))
			browser.CloseNow()
			for _, done := range []chan struct{}{hubDone, agentDone} {
				select {
				case <-done:
				case <-ctx.Done():
					t.Fatal("bridge did not settle")
				}
			}
		})
	}
}

// Log text is the largest bounded payload producer: 64 KiB before JSON escaping.
func TestLegitimateAgentLogFrameFits(t *testing.T) {
	payload, err := json.Marshal(LogMessage{Message: strings.Repeat("\x00", docker.MaxLogLineBytes), Service: "example", Stream: "stdout", Level: "info"})
	require.NoError(t, err)
	require.Less(t, len(payload), AgentFrameLimit)
	t.Logf("maximum escaped log-text fixture: %d bytes", len(payload))
}
