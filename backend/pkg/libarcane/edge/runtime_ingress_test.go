// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package edge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type runtimeEdgeFixture struct {
	client   *TunnelClient
	tunnel   *AgentTunnel
	observed chan string
}

func runtimeEdgeActual(t *testing.T, handler http.Handler) *runtimeEdgeFixture {
	t.Helper()
	token := uuid.New().String()
	registry := NewTunnelRegistry()
	connected := make(chan struct{})
	var once sync.Once
	server := NewTunnelServerWithRegistry(registry, func(_ context.Context, credential string) (string, error) {
		if credential != token {
			return "", errors.New("invalid synthetic credential")
		}
		return "example-runtime-edge", nil
	}, func(_ context.Context, _ string, online bool) {
		if online {
			once.Do(func() { close(connected) })
		}
	})
	router := echo.New()
	router.GET("/api/tunnel/connect", server.HandleConnect)
	manager := httptest.NewServer(router)
	t.Cleanup(manager.Close)
	observed := make(chan string, 1)
	local := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/observe/") {
			observed <- strings.TrimPrefix(r.URL.Path, "/observe/")
			w.WriteHeader(204)
			return
		}
		handler.ServeHTTP(w, r)
	})
	client := NewRuntimeTunnelClient(&Config{EdgeTransport: EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: token, EdgeReconnectInterval: 1}, local)
	life, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); client.StartWithErrorChan(life, nil) }()
	t.Cleanup(func() {
		cancel()
		registry.Unregister("example-runtime-edge")
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("runtime tunnel did not join")
		}
	})
	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("registration barrier not reached")
	}
	tunnel, ok := registry.Get("example-runtime-edge").Get()
	require.True(t, ok)
	return &runtimeEdgeFixture{client, tunnel, observed}
}
func (f *runtimeEdgeFixture) barrier(t *testing.T) {
	t.Helper()
	id := uuid.New().String()
	require.NoError(t, f.tunnel.Conn.Send(&TunnelMessage{ID: id, Type: MessageTypeRequest, Method: "GET", Path: "/observe/" + id}))
	select {
	case got := <-f.observed:
		require.Equal(t, id, got)
	case <-time.After(5 * time.Second):
		t.Fatal("message-loop barrier not reached")
	}
}
func (f *runtimeEdgeFixture) entry(id string) *runtimeRequestIngressInternal {
	f.client.runtimeRequestMu.Lock()
	defer f.client.runtimeRequestMu.Unlock()
	return f.client.runtimeRequests[id]
}
func (f *runtimeEdgeFixture) counts(t *testing.T, want int) {
	t.Helper()
	f.client.runtimeRequestMu.Lock()
	count := len(f.client.runtimeRequests)
	f.client.runtimeRequestMu.Unlock()
	require.Equal(t, want, count)
	transfers := 0
	f.client.requestTransfers.Range(func(_, _ any) bool { transfers++; return true })
	require.Equal(t, want, transfers)
}
func runtimeEdgeAwait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("runtime Edge cleanup did not join")
	}
}
func runtimeEdgeMetadata(id, transfer string) *TunnelMessage {
	return &TunnelMessage{ID: id, Type: MessageTypeCommandRequest, Command: "runtime.execute", Method: "POST", Path: "/api/environments/0/runtime-runs", Metadata: map[string]string{bodyTransferMetadataKey: transfer}}
}
func TestRuntimeEdgeActualAssemblyCancelAndDisconnect(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel-request", true: "disconnect"}[disconnect], func(t *testing.T) {
			var calls atomic.Int32
			f := runtimeEdgeActual(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			id, transfer := uuid.New().String(), uuid.New().String()
			require.NoError(t, f.tunnel.Conn.Send(runtimeEdgeMetadata(id, transfer)))
			f.barrier(t)
			entry := f.entry(id)
			require.NotNil(t, entry)
			deadline, ok := entry.ctx.Deadline()
			require.True(t, ok)
			require.LessOrEqual(t, time.Until(deadline), 5*time.Minute)
			require.NoError(t, f.tunnel.Conn.Send(&TunnelMessage{ID: transfer, Type: MessageTypeFileChunk, Sequence: 0, Body: []byte("partial")}))
			f.barrier(t)
			f.counts(t, 1)
			if disconnect {
				require.NoError(t, f.tunnel.Conn.Close())
			} else {
				require.NoError(t, f.tunnel.Conn.Send(&TunnelMessage{ID: id, Type: MessageTypeCancelRequest}))
				f.barrier(t)
			}
			runtimeEdgeAwait(t, entry.done)
			f.counts(t, 0)
			require.Zero(t, calls.Load())
		})
	}
}
func TestRuntimeEdgeActualAssembledByteAndPendingLimits(t *testing.T) {
	t.Run("bytes", func(t *testing.T) {
		var calls atomic.Int32
		f := runtimeEdgeActual(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
		id, transfer := uuid.New().String(), uuid.New().String()
		require.NoError(t, f.tunnel.Conn.Send(runtimeEdgeMetadata(id, transfer)))
		f.barrier(t)
		entry := f.entry(id)
		require.NotNil(t, entry)
		for i := range 5 {
			require.NoError(t, f.tunnel.Conn.Send(&TunnelMessage{ID: transfer, Type: MessageTypeFileChunk, Sequence: int64(i), Body: make([]byte, 256<<10), EOF: i == 4}))
		}
		f.barrier(t)
		runtimeEdgeAwait(t, entry.done)
		f.counts(t, 0)
		require.Zero(t, calls.Load())
	})
	t.Run("pending", func(t *testing.T) {
		var calls atomic.Int32
		f := runtimeEdgeActual(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
		var ids []string
		for range runtimePendingLimit + 1 {
			id := uuid.New().String()
			ids = append(ids, id)
			require.NoError(t, f.tunnel.Conn.Send(runtimeEdgeMetadata(id, uuid.New().String())))
		}
		f.barrier(t)
		f.counts(t, runtimePendingLimit)
		require.Nil(t, f.entry(ids[len(ids)-1]))
		var entries []*runtimeRequestIngressInternal
		for _, id := range ids[:runtimePendingLimit] {
			entries = append(entries, f.entry(id))
			require.NoError(t, f.tunnel.Conn.Send(&TunnelMessage{ID: id, Type: MessageTypeCancelRequest}))
		}
		f.barrier(t)
		for _, entry := range entries {
			runtimeEdgeAwait(t, entry.done)
		}
		f.counts(t, 0)
		require.Zero(t, calls.Load())
	})
}
func TestRuntimeEdgeActualDeadlineAndHandlerJoin(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "explicit-cancel", true: "deadline"}[timeout], func(t *testing.T) {
			entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			f := runtimeEdgeActual(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				close(entered)
				<-r.Context().Done()
				close(cancelled)
				<-release
			}))
			msg := runtimeEdgeMetadata(uuid.New().String(), "")
			msg.Metadata = nil
			msg.Body = []byte("{}")
			if timeout {
				msg.TimeoutMillis = 200
			}
			require.NoError(t, f.tunnel.Conn.Send(msg))
			runtimeEdgeAwait(t, entered)
			entry := f.entry(msg.ID)
			require.NotNil(t, entry)
			if !timeout {
				require.NoError(t, f.tunnel.Conn.Send(&TunnelMessage{ID: msg.ID, Type: MessageTypeCancelRequest}))
			}
			runtimeEdgeAwait(t, cancelled)
			require.NotNil(t, f.entry(msg.ID), "timeout/cancel alone must not claim actual handler join")
			select {
			case <-entry.done:
				t.Fatal("handler released before its join barrier")
			default:
			}
			once.Do(func() { close(release) })
			runtimeEdgeAwait(t, entry.done)
			require.Nil(t, f.entry(msg.ID))
		})
	}
}
func TestRuntimeEdgeFiniteResponseBoundSurvivesCloseDrain(t *testing.T) {
	tunnel := NewAgentTunnelWithConn("example-runtime-edge", &fakeTunnelConn{})
	pending := &PendingRequest{ResponseCh: make(chan *TunnelMessage, 8), failureCh: make(chan error, 1)}
	pending.ResponseCh <- &TunnelMessage{Type: MessageTypeCommandAck}
	for range 5 {
		pending.ResponseCh <- &TunnelMessage{Type: MessageTypeCommandOutput, Body: make([]byte, 256<<10)}
	}
	pending.ResponseCh <- &TunnelMessage{Type: MessageTypeCommandComplete, Status: 200}
	close(tunnel.done)
	status, _, body, err := collectCommandResponseBoundedInternal(t.Context(), tunnel, pending, "POST", runtimeRequestBodyLimit)
	require.Error(t, err)
	require.Zero(t, status)
	require.Empty(t, body)
}
