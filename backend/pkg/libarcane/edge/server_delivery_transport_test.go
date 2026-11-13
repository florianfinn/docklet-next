// Source: docklet-next/issues/416
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original real WS/gRPC delivery isolation and terminal-close proofs.

package edge

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type deliveryObservedConn struct {
	TunnelConnection
	controls chan *TunnelMessage
}

func (c *deliveryObservedConn) Receive() (*TunnelMessage, error) {
	msg, err := c.TunnelConnection.Receive()
	if err == nil {
		c.controls <- msg
	}
	return msg, err
}

type deliveryHarness struct {
	tunnel   *AgentTunnel
	agent    *deliveryObservedConn
	registry *TunnelRegistry
	clock    atomic.Int64
}

func newDeliveryHarness(t *testing.T, transport string) *deliveryHarness {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	h := &deliveryHarness{registry: NewTunnelRegistry()}
	h.clock.Store(time.Now().UnixNano())
	server := NewTunnelServerWithRegistry(h.registry, func(context.Context, string) (string, error) { return "delivery-example", nil }, nil)
	server.SetCleanupSchedule(&CleanupSchedule{Now: func() time.Time { return time.Unix(0, h.clock.Load()) }})
	router := echo.New()
	router.GET("/api/tunnel/connect", server.HandleConnect)
	manager := httptest.NewServer(router)
	var stopGRPC func()
	managerURL := manager.URL
	if transport == EdgeTransportGRPC {
		managerURL, stopGRPC = startTestGRPCTunnelServerOnAPIPathInternal(t, ctx, server)
	}
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.CloseNow() }()
		for {
			if _, _, err := ws.Read(ctx); err != nil {
				return
			}
		}
	}))
	_, port, err := net.SplitHostPort(local.Listener.Addr().String())
	require.NoError(t, err)
	client := NewTunnelClient(&Config{AgentToken: "example", ManagerApiUrl: managerURL, EdgeTransport: transport, Port: port}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("hold") == "1" {
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte("complete"))
	}))
	client.heartbeatInterval = time.Hour
	var connection TunnelConnection
	if transport == EdgeTransportWebSocket {
		ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(manager.URL, "http")+"/api/tunnel/connect", nil)
		require.NoError(t, err)
		connection = NewTunnelConn(ws)
	} else {
		cc, err := grpc.NewClient(strings.TrimPrefix(managerURL, "http://"), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = cc.Close() })
		stream, err := client.openTunnelConnectStreamInternal(testGRPCOutgoingContextInternal(ctx, "example"), cc, client.grpcConnectMethodInternal())
		require.NoError(t, err)
		connection = NewGRPCAgentTunnelConn(stream, cancel)
	}
	h.agent = &deliveryObservedConn{TunnelConnection: connection, controls: make(chan *TunnelMessage, 4096)}
	finished := make(chan error, 1)
	go func() { finished <- client.serveTunnelSessionInternal(ctx, h.agent, "example") }()
	require.Eventually(t, func() bool {
		var ok bool
		h.tunnel, ok = h.registry.Get("delivery-example").Get()
		return ok
	}, 3*time.Second, time.Millisecond, "real tunnel registers")
	t.Cleanup(func() {
		cancel()
		_ = h.agent.Close()
		_ = h.tunnel.CloseWithReason("")
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Error("agent session must join")
		}
		if stopGRPC != nil {
			stopGRPC()
		}
		manager.Close()
		local.Close()
	})
	return h
}

func (h *deliveryHarness) send(t *testing.T, msg *TunnelMessage) {
	t.Helper()
	require.NoError(t, h.agent.Send(msg))
}
func (h *deliveryHarness) heartbeat(t *testing.T, id string) {
	t.Helper()
	h.clock.Add(int64(time.Second))
	h.send(t, &TunnelMessage{ID: id, Type: MessageTypeHeartbeat})
	require.Eventually(t, func() bool { return h.tunnel.GetLastHeartbeat().UnixNano() == h.clock.Load() }, time.Second, time.Millisecond, "heartbeat handled before stalled request deadline")
}

// A context barrier holds the actual command collector before its select. This
// models an in-process consumer stall without changing the sender or receiver.
type deliveryGateContext struct {
	context.Context
	entered chan struct{}
	gate    chan struct{}
	once    sync.Once
}

func (c *deliveryGateContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered); <-c.gate })
	return c.Context.Done()
}
func newDeliveryGate(ctx context.Context) *deliveryGateContext {
	return &deliveryGateContext{Context: ctx, entered: make(chan struct{}), gate: make(chan struct{})}
}

func TestDeliveryRealTunnelIsolation(t *testing.T) {
	for _, transport := range []string{EdgeTransportWebSocket, EdgeTransportGRPC} {
		t.Run(transport, func(t *testing.T) {
			h := newDeliveryHarness(t, transport)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			gate := newDeliveryGate(ctx)
			var open sync.Once
			unblock := func() { open.Do(func() { close(gate.gate) }) }
			defer unblock()
			commandDone := make(chan error, 1)
			go func() {
				_, err := DefaultCommandClient.Execute(gate, h.tunnel, &CommandRequest{ID: "finite", Method: "GET", Path: "/api/health", Query: "hold=1"})
				commandDone <- err
			}()
			<-gate.entered
			entry, _ := h.tunnel.Pending.Load("finite")
			finite := entry.(*PendingRequest)
			// Hold the automatic HTTP reply while the fixture sends ordered finite
			// command chunks and a terminal frame through the real agent connection.
			for range cap(finite.ResponseCh) + 1 {
				h.send(t, &TunnelMessage{ID: "finite", Type: MessageTypeCommandOutput, Body: []byte("x")})
			}
			h.send(t, &TunnelMessage{ID: "finite", Type: MessageTypeCommandComplete, Status: 200})
			stream := blockedDelivery(t, h.tunnel, "stream")
			stream.deliveryMu.Lock()
			stream.onFailure = func() { stream.closeAgent(h.tunnel, "stream", MessageTypeStreamClose) }
			stream.deliveryMu.Unlock()
			h.send(t, &TunnelMessage{ID: "stream", Type: MessageTypeStreamData})
			h.heartbeat(t, "before-deadline")
			require.Greater(t, h.tunnel.overflowBytes.Load(), int64(0), "stalled delivery witnessed")
			probeCtx, probeCancel := context.WithTimeout(t.Context(), time.Second)
			result, err := DefaultCommandClient.Execute(probeCtx, h.tunnel, &CommandRequest{ID: "probe", Method: "GET", Path: "/api/health"})
			probeCancel()
			require.NoError(t, err, "B completes before A's delivery deadline")
			require.Equal(t, "complete", string(result.Body))
			require.False(t, finite.deliveryFailed(), "finite deadline has not elapsed")
			require.False(t, stream.deliveryFailed(), "stream deadline has not elapsed")
			require.Eventually(t, func() bool { return finite.deliveryFailed() && stream.deliveryFailed() }, 6*time.Second, time.Millisecond, "both stalled consumers fail at delivery bound")
			counts := map[TunnelMessageType]int{}
			require.Eventually(t, func() bool {
				for {
					select {
					case msg := <-h.agent.controls:
						if msg.ID == "finite" && msg.Type == MessageTypeCancelRequest {
							counts[msg.Type]++
						}
						if msg.ID == "stream" && msg.Type == MessageTypeStreamClose {
							counts[msg.Type]++
						}
					default:
						return counts[MessageTypeCancelRequest] == 1 && counts[MessageTypeStreamClose] == 1
					}
				}
			}, time.Second, time.Millisecond, "agent receives one cancel and one close")
			unblock()
			require.Error(t, <-commandDone, "timed-out finite collector returns its delivery failure")
			h.heartbeat(t, "after-cleanup")
			for {
				select {
				case msg := <-h.agent.controls:
					if msg.ID == "finite" && msg.Type == MessageTypeCancelRequest {
						counts[msg.Type]++
					}
					if msg.ID == "stream" && msg.Type == MessageTypeStreamClose {
						counts[msg.Type]++
					}
				default:
					goto counted
				}
			}
		counted:
			require.Equal(t, 1, counts[MessageTypeCancelRequest], "cancel emitted exactly once")
			require.Equal(t, 1, counts[MessageTypeStreamClose], "close emitted exactly once")
			require.Zero(t, h.tunnel.overflowBytes.Load())
		})
	}
}

func TestDeliveryTerminalOverflowSurvivesTunnelClose(t *testing.T) {
	h := newDeliveryHarness(t, EdgeTransportWebSocket)
	gate := newDeliveryGate(t.Context())
	var open sync.Once
	unblock := func() { open.Do(func() { close(gate.gate) }) }
	defer unblock()
	type response struct {
		result *CommandResult
		err    error
	}
	done := make(chan response, 1)
	p, err := registerPendingRequestInternal(h.tunnel, "terminal")
	require.NoError(t, err)
	headReady, sendHead := make(chan struct{}), make(chan struct{})
	var headOnce sync.Once
	allowHead := func() { headOnce.Do(func() { close(sendHead) }) }
	defer allowHead()
	p.deliveryMu.Lock()
	p.beforeDelivery = func() { close(headReady); <-sendHead }
	p.deliveryMu.Unlock()
	go func() {
		status, headers, body, err := collectCommandResponseInternal(gate, h.tunnel, p, "GET")
		p.Release()
		done <- response{&CommandResult{Status: status, Headers: headers, Body: body}, err}
	}()
	<-gate.entered
	for range cap(p.ResponseCh) {
		h.send(t, &TunnelMessage{ID: "terminal", Type: MessageTypeCommandOutput, Body: []byte("x")})
	}
	h.send(t, &TunnelMessage{ID: "terminal", Type: MessageTypeCommandComplete, Status: 201, Body: []byte("end")})
	h.heartbeat(t, "terminal-received")
	<-headReady
	require.Greater(t, h.tunnel.overflowBytes.Load(), int64(0), "terminal held in overflow")
	require.NoError(t, h.tunnel.CloseWithReason(""))
	unblock()
	require.Eventually(t, func() bool { return len(p.ResponseCh) == 0 }, time.Second, time.Millisecond, "collector drains channel before overflow terminal can send")
	allowHead()
	got := <-done
	require.NoError(t, got.err, "received overflow terminal survives transport close")
	require.Equal(t, 201, got.result.Status)
	require.Equal(t, strings.Repeat("x", cap(p.ResponseCh))+"end", string(got.result.Body))
	require.Eventually(t, func() bool { return h.tunnel.overflowBytes.Load() == 0 }, time.Second, time.Millisecond)
}

func TestDeliveryRealConsumerRelease(t *testing.T) {
	for _, kind := range []string{"command", "ws-proxy", "terminal"} {
		t.Run(kind, func(t *testing.T) {
			h := newDeliveryHarness(t, EdgeTransportWebSocket)
			ctx, cancel := context.WithCancel(t.Context())
			gate := newDeliveryGate(ctx)
			var open sync.Once
			unblock := func() { open.Do(func() { close(gate.gate) }) }
			defer unblock()
			defer cancel()
			done := make(chan struct{})
			var p *PendingRequest
			if kind == "command" {
				go func() {
					defer close(done)
					_, _ = DefaultCommandClient.Execute(gate, h.tunnel, &CommandRequest{ID: kind, Method: "GET", Path: "/api/health", Query: "hold=1"})
				}()
				<-gate.entered
				entry, _ := h.tunnel.Pending.Load(kind)
				p = entry.(*PendingRequest)
				for range cap(p.ResponseCh) + 1 {
					h.send(t, &TunnelMessage{ID: kind, Type: MessageTypeCommandOutput, Body: []byte("x")})
				}
			} else {
				p = blockedDelivery(t, h.tunnel, kind)
				go func() {
					defer close(done)
					if kind == "terminal" {
						forwardTerminalAgentToClient(ctx, gate, nil, h.tunnel, p, kind, nil)
					} else {
						forwardAgentToClient(ctx, gate, nil, h.tunnel, p, kind, nil)
					}
				}()
				<-gate.entered
				h.send(t, &TunnelMessage{ID: kind, Type: MessageTypeCommandAck})
			}
			h.heartbeat(t, "release-overflow")
			require.Greater(t, h.tunnel.overflowBytes.Load(), int64(0), "consumer has a waiting drainer")
			cancel()
			unblock()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("real consumer must exit")
			}
			// Execute and both forwarding functions are tested directly: no outer
			// request wrapper can hide a missing Release in this consumer.
			p.deliveryMu.Lock()
			p.failLocked(t.Context(), h.tunnel, kind, "delivery timeout")
			p.deliveryMu.Unlock()
			require.False(t, p.deliveryFailed(), "released consumer has no late delivery failure")
			require.Zero(t, h.tunnel.overflowBytes.Load(), "consumer release reclaims charges")
			require.Eventually(t, func() bool {
				p.deliveryMu.Lock()
				joined := p.drainerDone
				p.deliveryMu.Unlock()
				if joined == nil {
					return true
				}
				select {
				case <-joined:
					return true
				default:
					return false
				}
			}, time.Second, time.Millisecond, "released drainer joined")
		})
	}
}
