// Source: docklet-next/issues/416
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original raw-socket stalled log consumer delivery-bound proof.

package edge

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type deliveryLogBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *deliveryLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}
func (b *deliveryLogBuffer) text() string { b.mu.Lock(); defer b.mu.Unlock(); return b.Buffer.String() }

func TestDeliveryRawSocketLogBound(t *testing.T) {
	var logs deliveryLogBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	defer func() {
		if t.Failed() {
			t.Log(logs.text())
		}
	}()
	h := newDeliveryHarness(t, EdgeTransportWebSocket)
	router := echo.New()
	consumerDone := make(chan struct{})
	router.GET("/logs", func(c *echo.Context) error {
		defer close(consumerDone)
		return ProxyWebSocketRequest(c, h.tunnel, "/api/environments/0/ws/containers/example/logs", func(*http.Request) bool { return true })
	})
	browserServer := httptest.NewUnstartedServer(router)
	sendBuffer := make(chan error, 1)
	browserServer.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateNew {
			sendBuffer <- conn.(*net.TCPConn).SetWriteBuffer(1024)
		}
	}
	browserServer.Start()
	defer browserServer.Close()
	raw, err := net.Dial("tcp", browserServer.Listener.Addr().String())
	require.NoError(t, err)
	defer func() { _ = raw.Close() }()
	require.NoError(t, <-sendBuffer)
	require.NoError(t, raw.(*net.TCPConn).SetReadBuffer(1024))
	require.NoError(t, raw.SetDeadline(time.Now().Add(15*time.Second)))
	_, err = fmt.Fprintf(raw, "GET /logs HTTP/1.1\r\nHost: example.com\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: MDEyMzQ1Njc4OWFiY2RlZg==\r\n\r\n")
	require.NoError(t, err)
	resp, err := http.ReadResponse(bufio.NewReader(raw), &http.Request{Method: "GET"})
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	// No socket reads occur after the upgrade; the bounded TCP receive buffer
	// forces the real browser writer to stall rather than buffering in a WS client.
	var id string
	var p *PendingRequest
	require.Eventually(t, func() bool {
		h.tunnel.Pending.Range(func(k, v any) bool { id = k.(string); p = v.(*PendingRequest); return false })
		return p != nil
	}, time.Second, time.Millisecond)
	defer func() {
		if t.Failed() {
			p.deliveryMu.Lock()
			t.Logf("pending: channel=%d queue=%d charged=%d draining=%t released=%t failed=%t", len(p.ResponseCh), len(p.queue), p.charged, p.draining, p.released, p.failed)
			p.deliveryMu.Unlock()
		}
	}()
	payload := bytes.Repeat([]byte("x"), 64<<10)
	started := time.Now()
	producerDone := make(chan error, 1)
	overflowGo := make(chan struct{})
	var overflowOnce sync.Once
	allowOverflow := func() { overflowOnce.Do(func() { close(overflowGo) }) }
	defer allowOverflow()
	go func() {
		for i := range 850 {
			if i == 650 {
				<-overflowGo
			}
			if err := h.agent.Send(&TunnelMessage{ID: id, Type: MessageTypeStreamData, Body: payload, WSMessageType: int(websocket.MessageBinary)}); err != nil {
				producerDone <- err
				return
			}
		}
		producerDone <- nil
	}()
	t.Cleanup(func() {
		_ = h.agent.Close()
		select {
		case <-producerDone:
		case <-time.After(time.Second):
			t.Error("socket fixture producer must join")
		}
	})
	require.Eventually(t, func() bool {
		return len(p.ResponseCh) == cap(p.ResponseCh)
	}, time.Second, time.Millisecond, "pending delivery saturation witnessed")
	h.heartbeat(t, "socket-stall-heartbeat")
	probeCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	_, err = DefaultCommandClient.Execute(probeCtx, h.tunnel, &CommandRequest{Method: "GET", Path: "/api/health"})
	cancel()
	require.NoError(t, err, "concurrent request completes while raw log socket stalls")
	allowOverflow()
	require.Eventually(t, p.deliveryFailed, time.Second, time.Millisecond, "slow stream ends at delivery overflow bound")
	require.Contains(t, logs.text(), "requestId="+id, "failure log names request")
	require.Contains(t, logs.text(), `cause=overflow`, "close cause is delivery overflow, not browser write deadline")
	require.Less(t, time.Since(started), streamDeliveryTimeout, "overflow wins before the relay five-second write deadline")
	select {
	case <-consumerDone:
	case <-time.After(time.Second):
		t.Fatal("delivery failure must interrupt browser write and join proxy")
	}
	count := 0
	h.heartbeat(t, "socket-close-barrier")
	for {
		select {
		case msg := <-h.agent.controls:
			if msg.ID == id && msg.Type == MessageTypeStreamClose {
				count++
			}
		default:
			goto counted
		}
	}
counted:
	require.Equal(t, 1, count, "agent receives exactly one stream close")
	require.Zero(t, h.tunnel.overflowBytes.Load())
	// Advance the production liveness clock beyond the stale window, with real
	// heartbeats and a real registry sweep at each step; no 120s wall-clock sleep.
	for range 4 {
		h.clock.Add(int64(30 * time.Second))
		h.heartbeat(t, "long-lived")
		require.Empty(t, h.registry.CleanupStaleAt(t.Context(), TunnelStaleTimeout, time.Unix(0, h.clock.Load())))
	}
	require.True(t, TunnelLiveAt(h.tunnel, time.Unix(0, h.clock.Load())), "tunnel stays live past 90 seconds")
	require.NotContains(t, strings.ToLower(logs.text()), "failed to write", "delivery failure supplies the close cause")
}
