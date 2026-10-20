// Source: getarcaneapp/arcane/backend/pkg/libarcane/edge/client_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; relocated WebSocket origin/forwarding test to client_websocket_origin_test.go.

package edge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	tunnelpb "github.com/florianfinn/docklet-next/backend/proto/tunnel/v1"
)

func TestTunnelClient_HandleRequest(t *testing.T) {
	// 1. Setup Local Service (that agent proxies TO)
	localHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/local/api" {
			w.Header().Set("X-Local", "true")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("local response"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})

	// 2. Setup Mock Manager (that agent connects TO)
	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		_, registerData, _ := conn.Read(r.Context())
		var registerMsg TunnelMessage
		_ = json.Unmarshal(registerData, &registerMsg)
		assert.Equal(t, MessageTypeRegister, registerMsg.Type)
		registerResp, _ := json.Marshal(&TunnelMessage{
			Type:      MessageTypeRegisterResponse,
			Accepted:  true,
			SessionID: "session-1",
		})
		_ = conn.Write(r.Context(), websocket.MessageText, registerResp)

		// Send a request to the agent
		reqMsg := &TunnelMessage{
			ID:     "req-1",
			Type:   MessageTypeRequest,
			Method: "GET",
			Path:   "/local/api",
		}
		data, _ := json.Marshal(reqMsg)
		_ = conn.Write(r.Context(), websocket.MessageText, data)

		// Wait for response
		_, respData, _ := conn.Read(r.Context())
		var resp TunnelMessage
		_ = json.Unmarshal(respData, &resp)

		// Validate response from agent
		assert.Equal(t, "req-1", resp.ID)
		assert.Equal(t, MessageTypeResponse, resp.Type)
		assert.Equal(t, http.StatusOK, resp.Status)
		assert.Equal(t, "true", resp.Headers["X-Local"])
		assert.Equal(t, "local response", string(resp.Body))
	}))
	defer managerServer.Close()

	// 3. Configure and Start Agent Client
	cfg := &Config{
		EdgeTransport:         EdgeTransportWebSocket,
		ManagerApiUrl:         managerServer.URL,
		AgentToken:            "test-token",
		EdgeReconnectInterval: 1,
	}

	client := NewTunnelClient(cfg, localHandler)
	client.managerURL = "ws" + strings.TrimPrefix(managerServer.URL, "http")

	ctx := t.Context()

	// Run client in background
	go client.StartWithErrorChan(ctx, nil)

	// Wait for process to finish or timeout
	time.Sleep(100 * time.Millisecond)
}

func TestTunnelClient_WebSocketProxy(t *testing.T) {
	// 1. Setup Local Service with WS
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		for {
			mt, data, readErr := conn.Read(r.Context())
			if readErr != nil {
				return
			}
			// Echo
			_ = conn.Write(r.Context(), mt, append([]byte("local echo: "), data...))
		}
	}))
	defer localServer.Close()

	localPort := strings.Split(localServer.Listener.Addr().String(), ":")[1]

	// 2. Setup Mock Manager
	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		_, registerData, _ := conn.Read(r.Context())
		var registerMsg TunnelMessage
		_ = json.Unmarshal(registerData, &registerMsg)
		assert.Equal(t, MessageTypeRegister, registerMsg.Type)
		registerResp, _ := json.Marshal(&TunnelMessage{
			Type:      MessageTypeRegisterResponse,
			Accepted:  true,
			SessionID: "session-1",
		})
		_ = conn.Write(r.Context(), websocket.MessageText, registerResp)

		// Send WS Start
		startMsg := &TunnelMessage{
			ID:   "ws-1",
			Type: MessageTypeWebSocketStart,
			Path: "/", // Connect to root of local server
		}
		data, _ := json.Marshal(startMsg)
		_ = conn.Write(r.Context(), websocket.MessageText, data)

		// Send Data
		dataMsg := &TunnelMessage{
			ID:            "ws-1",
			Type:          MessageTypeWebSocketData,
			Body:          []byte("hello"),
			WSMessageType: int(websocket.MessageText),
		}
		data, _ = json.Marshal(dataMsg)
		_ = conn.Write(r.Context(), websocket.MessageText, data)

		// Read Echo
		_, respData, _ := conn.Read(r.Context())
		var resp TunnelMessage
		_ = json.Unmarshal(respData, &resp)

		assert.Equal(t, MessageTypeWebSocketData, resp.Type)
		assert.Equal(t, "local echo: hello", string(resp.Body))
	}))
	defer managerServer.Close()

	// 3. Configure Agent
	cfg := &Config{
		EdgeTransport: EdgeTransportWebSocket,
		ManagerApiUrl: managerServer.URL,
		AgentToken:    "test-token",
		Port:          localPort, // Tell agent where local service is
	}

	client := NewTunnelClient(cfg, http.NotFoundHandler()) // Handler ignored for WS
	client.managerURL = "ws" + strings.TrimPrefix(managerServer.URL, "http")

	ctx := t.Context()

	go client.StartWithErrorChan(ctx, nil)
	time.Sleep(100 * time.Millisecond)
}

// TestTunnelClient_WebSocket_ReconnectClosesStreams verifies that a reconnect
// reclaims the goroutines, local sockets, and activeStreams entries opened on
// the previous connection instead of leaking them. Run under -race it also
// exercises concurrent c.conn access across the reassignment on reconnect.
func TestTunnelClient_WebSocket_ReconnectClosesStreams(t *testing.T) {
	// Local WS echo server the agent proxies to.
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		for {
			mt, data, readErr := conn.Read(r.Context())
			if readErr != nil {
				return
			}
			_ = conn.Write(r.Context(), mt, append([]byte("local echo: "), data...))
		}
	}))
	defer localServer.Close()
	localPort := strings.Split(localServer.Listener.Addr().String(), ":")[1]

	var connCount atomic.Int32
	firstStreamLive := make(chan struct{})
	closeFirstConn := make(chan struct{})
	stopManager := make(chan struct{})

	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		// Every connection registers first.
		if _, _, readErr := conn.Read(r.Context()); readErr != nil {
			return
		}
		registerResp, _ := json.Marshal(&TunnelMessage{
			Type:      MessageTypeRegisterResponse,
			Accepted:  true,
			SessionID: "session-1",
		})
		_ = conn.Write(r.Context(), websocket.MessageText, registerResp)

		if connCount.Add(1) > 1 {
			// Post-reconnect connection: stay up so the agent stops reconnecting
			// and the test can assert a stable, empty stream set.
			<-stopManager
			return
		}

		// First connection: open a stream and prove it is live by round-tripping
		// data through the local echo server, then force a disconnect.
		startMsg, _ := json.Marshal(&TunnelMessage{ID: "ws-1", Type: MessageTypeWebSocketStart, Path: "/"})
		_ = conn.Write(r.Context(), websocket.MessageText, startMsg)
		dataMsg, _ := json.Marshal(&TunnelMessage{
			ID:            "ws-1",
			Type:          MessageTypeWebSocketData,
			Body:          []byte("hello"),
			WSMessageType: int(websocket.MessageText),
		})
		_ = conn.Write(r.Context(), websocket.MessageText, dataMsg)

		_, respData, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var resp TunnelMessage
		_ = json.Unmarshal(respData, &resp)
		assert.Equal(t, MessageTypeWebSocketData, resp.Type)
		assert.Equal(t, "local echo: hello", string(resp.Body))

		close(firstStreamLive)
		<-closeFirstConn // returning closes the conn, forcing the agent to reconnect
	}))
	defer managerServer.Close()

	cfg := &Config{
		EdgeTransport: EdgeTransportWebSocket,
		ManagerApiUrl: managerServer.URL,
		AgentToken:    "test-token",
		Port:          localPort,
	}
	client := NewTunnelClient(cfg, http.NotFoundHandler())
	client.managerURL = "ws" + strings.TrimPrefix(managerServer.URL, "http")
	client.reconnectInterval = 50 * time.Millisecond

	countStreams := func() int {
		n := 0
		client.activeStreams.Range(func(_, _ any) bool { n++; return true })
		return n
	}

	ctx := t.Context()
	defer close(stopManager)
	go client.StartWithErrorChan(ctx, nil)

	// The stream is registered while the first connection is live.
	select {
	case <-firstStreamLive:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for the first stream to come up")
	}
	assert.Equal(t, 1, countStreams(), "expected one active stream during the first connection")

	// Force the disconnect; the reconnect must reclaim the stream.
	close(closeFirstConn)
	require.Eventually(t, func() bool {
		return connCount.Load() >= 2 && countStreams() == 0
	}, 5*time.Second, 20*time.Millisecond, "stream leaked across reconnect")
}

func TestTunnelClient_HandleRequest_Errors(t *testing.T) {
	// Setup Mock Manager
	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		_, registerData, _ := conn.Read(r.Context())
		var registerMsg TunnelMessage
		_ = json.Unmarshal(registerData, &registerMsg)
		assert.Equal(t, MessageTypeRegister, registerMsg.Type)
		registerResp, _ := json.Marshal(&TunnelMessage{
			Type:      MessageTypeRegisterResponse,
			Accepted:  true,
			SessionID: "session-1",
		})
		_ = conn.Write(r.Context(), websocket.MessageText, registerResp)

		// 1. Send request with invalid URL to trigger error
		reqMsg := &TunnelMessage{
			ID:     "req-err",
			Type:   MessageTypeRequest,
			Method: "GET",
			Path:   "://invalid-url",
		}
		data, _ := json.Marshal(reqMsg)
		_ = conn.Write(r.Context(), websocket.MessageText, data)

		// Expect error response
		_, respData, _ := conn.Read(r.Context())
		var resp TunnelMessage
		_ = json.Unmarshal(respData, &resp)

		assert.Equal(t, "req-err", resp.ID)
		assert.Equal(t, 500, resp.Status)

		// 2. Send unknown message type
		unknownMsg := &TunnelMessage{
			ID:   "unknown",
			Type: "unknown_type",
		}
		data, _ = json.Marshal(unknownMsg)
		_ = conn.Write(r.Context(), websocket.MessageText, data)
	}))
	defer managerServer.Close()

	cfg := &Config{
		EdgeTransport: EdgeTransportWebSocket,
		ManagerApiUrl: managerServer.URL,
		AgentToken:    "test-token",
	}

	client := NewTunnelClient(cfg, http.NotFoundHandler())
	client.managerURL = "ws" + strings.TrimPrefix(managerServer.URL, "http")

	ctx := t.Context()

	go client.StartWithErrorChan(ctx, nil)
	time.Sleep(100 * time.Millisecond)
}

func TestTunnelClient_InternalHelpers(t *testing.T) {
	// Mock connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		for {
			_, _, readErr := conn.Read(r.Context())
			if readErr != nil {
				return
			}
		}
	}))
	defer server.Close()

	cfg := &Config{
		ManagerApiUrl: server.URL,
		AgentToken:    "test-token",
	}
	client := NewTunnelClient(cfg, nil)

	// Manually connect
	serverURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.Dial(t.Context(), serverURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()

	tunnelConn := NewTunnelConn(conn)
	client.conn.Store(&connBox{conn: tunnelConn})

	// Test sendWebSocketData
	err = client.sendWebSocketData(tunnelConn, "stream-1", int(websocket.MessageText), []byte("data"))
	require.NoError(t, err)

	// Test sendWebSocketClose
	client.sendWebSocketClose(tunnelConn, "stream-1")

	// Test sendErrorResponse
	client.sendErrorResponse(tunnelConn, "req-1", 500, "error")
}

func TestTunnelClient_BuildLocalWebSocketURL(t *testing.T) {
	tests := []struct {
		name     string
		listen   string
		port     string
		path     string
		query    string
		expected string
	}{
		{
			name:     "empty listen uses localhost",
			listen:   "",
			port:     "3553",
			path:     "/api",
			query:    "",
			expected: "ws://localhost:3553/api",
		},
		{
			name:     "wildcard ipv4 maps to localhost",
			listen:   "0.0.0.0",
			port:     "3553",
			path:     "/",
			query:    "",
			expected: "ws://localhost:3553/",
		},
		{
			name:     "wildcard ipv6 maps to localhost",
			listen:   "::",
			port:     "3553",
			path:     "/",
			query:    "",
			expected: "ws://localhost:3553/",
		},
		{
			name:     "explicit ipv4 listen",
			listen:   "127.0.0.1",
			port:     "3553",
			path:     "/",
			query:    "q=1",
			expected: "ws://127.0.0.1:3553/?q=1",
		},
		{
			name:     "explicit ipv6 listen",
			listen:   "2001:db8::1",
			port:     "3553",
			path:     "/ws",
			query:    "",
			expected: "ws://[2001:db8::1]:3553/ws",
		},
		{
			name:     "listen host and port wildcard maps to localhost",
			listen:   "0.0.0.0:3553",
			port:     "3553",
			path:     "/ws",
			query:    "",
			expected: "ws://localhost:3553/ws",
		},
		{
			name:     "listen with port only maps to localhost",
			listen:   ":3553",
			port:     "3553",
			path:     "/ws",
			query:    "",
			expected: "ws://localhost:3553/ws",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := &Config{
				Listen: testCase.listen,
				Port:   testCase.port,
			}
			client := NewTunnelClient(cfg, http.NotFoundHandler())
			msg := &TunnelMessage{
				Path:  testCase.path,
				Query: testCase.query,
			}
			assert.Equal(t, testCase.expected, client.buildLocalWebSocketURLInternal(msg))
		})
	}
}

func TestTunnelClient_GRPCConnectMethodInternal(t *testing.T) {
	client := NewTunnelClient(&Config{}, http.NotFoundHandler())
	assert.Equal(t, "/api/tunnel/connect", client.grpcConnectMethodInternal())
}

func TestTunnelClient_buildLocalWebSocketHeadersInternal(t *testing.T) {
	client := NewTunnelClient(&Config{
		AgentToken: "agent-token",
	}, http.NotFoundHandler())

	headers := client.buildLocalWebSocketHeadersInternal(&TunnelMessage{
		Headers: map[string]string{
			"sec-websocket-key":      "abc",
			"sec-websocket-version":  "13",
			"Sec-WebSocket-Protocol": "binary",
			"X-Custom":               "value",
			"X-API-Key":              "manager-token",
		},
	})

	assert.Empty(t, headers.Get("Sec-Websocket-Key"))
	assert.Empty(t, headers.Get("Sec-Websocket-Version"))
	assert.Equal(t, "binary", headers.Get("Sec-Websocket-Protocol"))
	assert.Equal(t, "value", headers.Get("X-Custom"))
	assert.Equal(t, "agent-token", headers.Get("X-API-Key"))
	assert.Equal(t, "agent-token", headers.Get("X-Arcane-Agent-Token"))
}

func TestTunnelClient_buildLocalWebSocketHeadersInternal_FiltersBrowserHeaders(t *testing.T) {
	client := NewTunnelClient(&Config{
		AgentToken: "agent-token",
	}, http.NotFoundHandler())

	headers := client.buildLocalWebSocketHeadersInternal(&TunnelMessage{
		Headers: map[string]string{
			// Browser headers that should be stripped
			"Origin":             "https://docker.example.com",
			"Cookie":             "session=abc123",
			"Authorization":      "Bearer browser-jwt",
			"Referer":            "https://docker.example.com/environments/123",
			"Sec-Fetch-Dest":     "websocket",
			"Sec-Fetch-Mode":     "websocket",
			"Sec-Fetch-Site":     "same-origin",
			"Sec-Fetch-User":     "?1",
			"Sec-Ch-Ua":          "\"Chromium\";v=\"130\"",
			"Sec-Ch-Ua-Mobile":   "?0",
			"Sec-Ch-Ua-Platform": "\"Linux\"",
			// Headers that should be preserved
			"X-Custom":               "value",
			"Sec-WebSocket-Protocol": "binary",
			"Accept-Language":        "en-US",
		},
	})

	// Browser headers must be stripped
	assert.Empty(t, headers.Get("Origin"), "Origin should be stripped")
	assert.Empty(t, headers.Get("Cookie"), "Cookie should be stripped")
	assert.Empty(t, headers.Get("Authorization"), "Authorization should be stripped")
	assert.Empty(t, headers.Get("Referer"), "Referer should be stripped")
	assert.Empty(t, headers.Get("Sec-Fetch-Dest"), "Sec-Fetch-Dest should be stripped")
	assert.Empty(t, headers.Get("Sec-Fetch-Mode"), "Sec-Fetch-Mode should be stripped")
	assert.Empty(t, headers.Get("Sec-Fetch-Site"), "Sec-Fetch-Site should be stripped")
	assert.Empty(t, headers.Get("Sec-Fetch-User"), "Sec-Fetch-User should be stripped")
	assert.Empty(t, headers.Get("Sec-Ch-Ua"), "Sec-Ch-Ua should be stripped")
	assert.Empty(t, headers.Get("Sec-Ch-Ua-Mobile"), "Sec-Ch-Ua-Mobile should be stripped")
	assert.Empty(t, headers.Get("Sec-Ch-Ua-Platform"), "Sec-Ch-Ua-Platform should be stripped")

	// Non-browser headers must be preserved
	assert.Equal(t, "value", headers.Get("X-Custom"))
	assert.Equal(t, "binary", headers.Get("Sec-Websocket-Protocol"))
	assert.Equal(t, "en-US", headers.Get("Accept-Language"))

	// Agent token must override any manager-forwarded auth
	assert.Equal(t, "agent-token", headers.Get("X-API-Key"))
	assert.Equal(t, "agent-token", headers.Get("X-Arcane-Agent-Token"))
}

func TestTunnelClient_IsGRPCConnectionInternal(t *testing.T) {
	t.Run("nil connection", func(t *testing.T) {
		assert.False(t, isGRPCConnection(nil))
	})

	t.Run("grpc connection", func(t *testing.T) {
		assert.True(t, isGRPCConnection(NewGRPCAgentTunnelConn(nil)))
	})

	t.Run("non-grpc connection", func(t *testing.T) {
		assert.False(t, isGRPCConnection(&fakeTunnelConnForTransportCheck{}))
	})
}

func TestTunnelClient_HandleRequest_GRPCConfigWithWebSocketConnUsesNonStreamingResponse(t *testing.T) {
	localHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.Equal(t, "/local/api", r.URL.Path) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportGRPC,
	}, localHandler)
	conn := &capturingTunnelConnForHandleRequest{}
	client.conn.Store(&connBox{conn: conn})

	client.handleRequest(t.Context(), conn, &TunnelMessage{
		ID:     "req-fallback-1",
		Type:   MessageTypeRequest,
		Method: http.MethodGet,
		Path:   "/local/api",
	})

	require.Len(t, conn.sent, 1)
	assert.Equal(t, MessageTypeResponse, conn.sent[0].Type)
	assert.Equal(t, http.StatusOK, conn.sent[0].Status)
	assert.Equal(t, `{"ok":true}`, string(conn.sent[0].Body))
}

func TestTunnelClient_HeartbeatLoop_ClosesConnectionOnSendFailure(t *testing.T) {
	conn := &failingHeartbeatConn{}
	client := &TunnelClient{
		heartbeatInterval: 5 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	client.heartbeatLoop(ctx, conn)
	assert.True(t, conn.closeCalled)
}

type fakeTunnelConnForTransportCheck struct{}

func (f *fakeTunnelConnForTransportCheck) Send(_ *TunnelMessage) error {
	return nil
}

func (f *fakeTunnelConnForTransportCheck) Receive() (*TunnelMessage, error) {
	return nil, nil
}

func (f *fakeTunnelConnForTransportCheck) IsExpectedReceiveError(error) bool {
	return false
}

func (f *fakeTunnelConnForTransportCheck) Close() error {
	return nil
}

func (f *fakeTunnelConnForTransportCheck) IsClosed() bool {
	return false
}

func (f *fakeTunnelConnForTransportCheck) Transport() string { return EdgeTransportWebSocket }

type capturingTunnelConnForHandleRequest struct {
	sent []*TunnelMessage
}

func (c *capturingTunnelConnForHandleRequest) Send(msg *TunnelMessage) error {
	cloned := *msg
	if msg.Headers != nil {
		cloned.Headers = make(map[string]string, len(msg.Headers))
		maps.Copy(cloned.Headers, msg.Headers)
	}
	if msg.Body != nil {
		cloned.Body = append([]byte(nil), msg.Body...)
	}
	c.sent = append(c.sent, &cloned)
	return nil
}

func (c *capturingTunnelConnForHandleRequest) Receive() (*TunnelMessage, error) {
	return nil, nil
}

func (c *capturingTunnelConnForHandleRequest) IsExpectedReceiveError(error) bool {
	return false
}

func (c *capturingTunnelConnForHandleRequest) Close() error {
	return nil
}

func (c *capturingTunnelConnForHandleRequest) IsClosed() bool {
	return false
}

func (c *capturingTunnelConnForHandleRequest) Transport() string { return EdgeTransportWebSocket }

type failingHeartbeatConn struct {
	closeCalled bool
}

func (f *failingHeartbeatConn) Send(*TunnelMessage) error {
	return errors.New("send failed")
}

func (f *failingHeartbeatConn) Receive() (*TunnelMessage, error) {
	return nil, errors.New("receive not implemented")
}

func (f *failingHeartbeatConn) IsExpectedReceiveError(error) bool {
	return false
}

func (f *failingHeartbeatConn) Close() error {
	f.closeCalled = true
	return nil
}

func (f *failingHeartbeatConn) IsClosed() bool {
	return false
}

func (f *failingHeartbeatConn) Transport() string { return EdgeTransportWebSocket }

type stallingTunnelService struct {
	tunnelpb.UnimplementedTunnelServiceServer
	connectCount atomic.Int32
}

type blockingRegistrationConn struct {
	closeCount       atomic.Int32
	closedCh         chan struct{}
	receiveStarted   chan struct{}
	receiveStartOnce sync.Once
}

func newBlockingRegistrationConnInternal() *blockingRegistrationConn {
	return &blockingRegistrationConn{
		closedCh:       make(chan struct{}),
		receiveStarted: make(chan struct{}),
	}
}

func (c *blockingRegistrationConn) Send(*TunnelMessage) error { return nil }

func (c *blockingRegistrationConn) Receive() (*TunnelMessage, error) {
	c.receiveStartOnce.Do(func() {
		close(c.receiveStarted)
	})
	<-c.closedCh
	return nil, io.EOF
}

func (c *blockingRegistrationConn) IsExpectedReceiveError(error) bool { return false }

func (c *blockingRegistrationConn) Close() error {
	if c.closeCount.Add(1) == 1 {
		close(c.closedCh)
	}
	return nil
}

func (c *blockingRegistrationConn) IsClosed() bool {
	select {
	case <-c.closedCh:
		return true
	default:
		return false
	}
}

func (c *blockingRegistrationConn) Transport() string { return EdgeTransportWebSocket }

func (s *stallingTunnelService) Connect(stream grpc.BidiStreamingServer[tunnelpb.AgentMessage, tunnelpb.ManagerMessage]) error {
	s.connectCount.Add(1)
	if _, err := stream.Recv(); err != nil {
		return err
	}

	<-stream.Context().Done()
	return stream.Context().Err()
}
