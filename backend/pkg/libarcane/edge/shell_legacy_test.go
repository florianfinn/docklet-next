// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original legacy stream allowlist test over the real tunnel wire.

package edge

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestLegacyShellStartRefusedOverRealTunnel(t *testing.T) {
	var dials atomic.Int32
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { dials.Add(1); w.WriteHeader(403) }))
	defer local.Close()
	address, err := url.Parse(local.URL)
	require.NoError(t, err)
	client := NewTunnelClient(&Config{Listen: address.Hostname(), Port: address.Port()}, http.NotFoundHandler())
	connected := make(chan *TunnelConn, 1)
	manager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		connected <- NewTunnelConn(conn)
	}))
	defer manager.Close()
	raw, _, err := websocket.Dial(t.Context(), strings.Replace(manager.URL, "http", "ws", 1), nil)
	require.NoError(t, err)
	agent := NewTunnelConn(raw)
	wire := <-connected
	var workers sync.WaitGroup
	joined := make(chan struct{})
	go func() { defer close(joined); _ = client.messageLoop(t.Context(), agent, &workers); workers.Wait() }()
	defer func() { _ = wire.Close(); _ = agent.Close(); <-joined }()
	for _, command := range []string{"", "container.exec.stream", "system.stats.stream"} {
		require.NoError(t, wire.Send(&TunnelMessage{ID: "example", Type: MessageTypeWebSocketStart, Path: "/forbidden", Command: command}))
		response, err := wire.Receive()
		require.NoError(t, err)
		require.Equal(t, MessageTypeStreamClose, response.Type)
		require.Equal(t, "unsupported edge stream", response.Error)
		require.Zero(t, dials.Load(), "a rejected command must never dial locally")
	}
}
