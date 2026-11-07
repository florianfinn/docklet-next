// Source: docklet-next/issues/415
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original agent stream back-pressure proofs over the real WebSocket tunnel.

package edge

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

const backpressureStreamID = "ws-backpressure"

// Frames large enough that a local consumer which does not read fills the
// socket buffers, so the agent's local writer and then dataCh really back up.
const backpressureFrameSize = 64 * 1024

type backpressureHarness struct {
	client   *TunnelClient
	gate     chan struct{}
	kill     chan struct{}
	stop     chan struct{}
	received chan uint32
	closes   chan *TunnelMessage
	sent     atomic.Int64
	writer   chan struct{}
	once     struct{ gate, kill, stop sync.Once }
}

func (h *backpressureHarness) releaseGate() { h.once.gate.Do(func() { close(h.gate) }) }
func (h *backpressureHarness) killLocal()   { h.once.kill.Do(func() { close(h.kill) }) }
func (h *backpressureHarness) stopWriter()  { h.once.stop.Do(func() { close(h.stop) }) }

// newBackpressureHarness wires a manager, the real agent TunnelClient and a
// local WebSocket service that reads only after gate opens.
func newBackpressureHarness(t *testing.T) *backpressureHarness {
	t.Helper()
	h := &backpressureHarness{
		gate:     make(chan struct{}),
		kill:     make(chan struct{}),
		stop:     make(chan struct{}),
		received: make(chan uint32, 1<<16),
		closes:   make(chan *TunnelMessage, 8),
		writer:   make(chan struct{}),
	}

	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		conn.SetReadLimit(1 << 20)
		select {
		case <-h.gate:
		case <-h.kill:
			return
		case <-r.Context().Done():
			return
		}
		for {
			_, data, readErr := conn.Read(r.Context())
			if readErr != nil {
				return
			}
			h.received <- binary.BigEndian.Uint32(data)
		}
	}))

	manager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		conn.SetReadLimit(maxWebSocketTunnelMessageSize)
		ctx := r.Context()
		if _, _, readErr := conn.Read(ctx); readErr != nil {
			return
		}
		writeJSON := func(msg *TunnelMessage) error {
			data, marshalErr := json.Marshal(msg)
			if marshalErr != nil {
				return marshalErr
			}
			return conn.Write(ctx, websocket.MessageText, data)
		}
		if writeJSON(&TunnelMessage{Type: MessageTypeRegisterResponse, Accepted: true, SessionID: "session-1"}) != nil {
			return
		}
		if writeJSON(&TunnelMessage{ID: backpressureStreamID, Type: MessageTypeWebSocketStart, Path: "/api/environments/0/ws/containers/example/terminal", Command: "container.exec.stream"}) != nil {
			return
		}
		go func() {
			defer close(h.writer)
			for i := uint32(0); ; i++ {
				select {
				case <-h.stop:
					return
				default:
				}
				body := make([]byte, backpressureFrameSize)
				binary.BigEndian.PutUint32(body, i)
				if writeJSON(&TunnelMessage{ID: backpressureStreamID, Type: MessageTypeWebSocketData, Body: body, WSMessageType: int(websocket.MessageBinary)}) != nil {
					return
				}
				h.sent.Add(1)
			}
		}()
		for {
			_, data, readErr := conn.Read(ctx)
			if readErr != nil {
				return
			}
			var msg TunnelMessage
			if json.Unmarshal(data, &msg) == nil && msg.ID == backpressureStreamID && msg.Type == MessageTypeStreamClose {
				select {
				case h.closes <- &msg:
				default:
				}
			}
		}
	}))

	cfg := &Config{
		EdgeTransport: EdgeTransportWebSocket,
		ManagerApiUrl: manager.URL,
		Port:          strings.Split(local.Listener.Addr().String(), ":")[1],
	}
	h.client = NewTunnelClient(cfg, http.NotFoundHandler())
	h.client.managerURL = "ws" + strings.TrimPrefix(manager.URL, "http")
	ctx, cancel := context.WithCancel(context.Background())
	agentDone := make(chan struct{})
	go func() {
		defer close(agentDone)
		h.client.StartWithErrorChan(ctx, nil)
	}()
	t.Cleanup(func() {
		h.stopWriter()
		h.killLocal()
		cancel()
		<-agentDone
		manager.Close()
		local.Close()
	})
	return h
}

func (h *backpressureHarness) stream() *activeWSStream {
	raw, ok := h.client.activeStreams.Load(backpressureStreamID)
	if !ok {
		return nil
	}
	stream, _ := raw.(*activeWSStream)
	return stream
}

// waitForBackedUpStream returns once dataCh is full, so the agent's receive
// loop waits on a full buffer (or, without back-pressure, drops the next frame).
func (h *backpressureHarness) waitForBackedUpStream(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		stream := h.stream()
		return stream != nil && len(stream.dataCh) == cap(stream.dataCh)
	}, 30*time.Second, 5*time.Millisecond, "the local writer backs up and dataCh fills")
	// Let at least one frame meet the full buffer; the manager may already be
	// blocked by the waiting agent, which is why this wait is not asserted.
	atFull := h.sent.Load()
	deadline := time.Now().Add(time.Second)
	for h.sent.Load() == atFull && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
}

// A local consumer that falls behind for less than the delivery timeout loses
// no input: every frame arrives in order once it reads again.
func TestAgentStreamWaitsForASlowLocalConsumer(t *testing.T) {
	h := newBackpressureHarness(t)
	h.waitForBackedUpStream(t)
	h.stopWriter()
	h.releaseGate()
	<-h.writer
	sent := h.sent.Load()

	for want := range uint32(sent) {
		select {
		case got := <-h.received:
			require.Equal(t, want, got, "frames arrive complete and in order")
		case closeMsg := <-h.closes:
			t.Fatalf("stream closed after %d of %d frames: %q", want, sent, closeMsg.Error)
		case <-time.After(30 * time.Second):
			t.Fatalf("received %d of %d frames", want, sent)
		}
	}
}

// A local consumer that stays stuck ends the stream with an error to the hub
// instead of the agent dropping input silently.
func TestAgentStreamClosesAStuckLocalConsumer(t *testing.T) {
	h := newBackpressureHarness(t)
	h.waitForBackedUpStream(t)

	select {
	case closeMsg := <-h.closes:
		require.Contains(t, closeMsg.Error, "timed out")
	case <-time.After(streamDeliveryTimeout + 20*time.Second):
		t.Fatal("the hub never saw the stuck stream close")
	}
	require.Eventually(t, func() bool { return h.stream() == nil }, 10*time.Second, 5*time.Millisecond,
		"the stuck stream is removed")
}

// Closing the stream while delivery waits on a full buffer ends the wait
// through the stream's done channel; the close reason is the local one.
func TestAgentStreamCloseEndsAWaitingDelivery(t *testing.T) {
	h := newBackpressureHarness(t)
	h.waitForBackedUpStream(t)
	h.killLocal()

	select {
	case closeMsg := <-h.closes:
		require.NotContains(t, closeMsg.Error, "timed out", "the close wins over the delivery timeout")
	case <-time.After(streamDeliveryTimeout):
		t.Fatal("the hub never saw the local close")
	}
	require.Eventually(t, func() bool { return h.stream() == nil }, 10*time.Second, 5*time.Millisecond)
	// A delivery that kept waiting would report its timeout after the close.
	select {
	case late := <-h.closes:
		require.NotContains(t, late.Error, "timed out", "the waiting delivery ended with the close")
	case <-time.After(streamDeliveryTimeout + time.Second):
	}
}
