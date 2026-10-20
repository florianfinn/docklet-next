// Source: getarcaneapp/arcane/backend/pkg/libarcane/edge/client_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; relocated WebSocket origin/forwarding test to client_websocket_origin_test.go.

package edge

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTunnelClient_GRPC_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)

	envID := "env-e2e-grpc-1"
	GetRegistry().Unregister(envID)
	defer GetRegistry().Unregister(envID)

	resolver := func(ctx context.Context, token string) (string, error) {
		if token != "valid-token" {
			return "", errors.New("invalid token")
		}
		return envID, nil
	}

	tunnelServer := NewTunnelServerWithRegistry(GetRegistry(), resolver, nil)
	go tunnelServer.StartCleanupLoop(ctx)
	defer func() {
		cancel()
		tunnelServer.WaitForCleanupDone()
	}()

	managerURL, stopManager := startTestGRPCTunnelServerOnAPIPathInternal(t, ctx, tunnelServer)
	defer stopManager()

	localHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok-from-agent"))
	})

	cfg := &Config{
		EdgeTransport:         EdgeTransportGRPC,
		ManagerApiUrl:         managerURL,
		AgentToken:            "valid-token",
		EdgeReconnectInterval: 1,
		Port:                  "3552",
	}

	client := NewTunnelClient(cfg, localHandler)
	errCh := make(chan error, 4)
	go client.StartWithErrorChan(ctx, errCh)

	var tunnel *AgentTunnel
	require.Eventually(t, func() bool {
		var ok bool
		tunnel, ok = GetRegistry().Get(envID).Get()
		return ok && tunnel != nil && !tunnel.Conn.IsClosed()
	}, 5*time.Second, 20*time.Millisecond)

	proxyCtx, proxyCancel := context.WithTimeout(ctx, 5*time.Second)
	defer proxyCancel()

	localStatus, headers, body, err := ProxyRequest(proxyCtx, tunnel, http.MethodGet, "/api/health", "", map[string]string{"Accept": "text/plain"}, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, localStatus)
	assert.Equal(t, "text/plain", headers["Content-Type"])
	assert.Equal(t, "ok-from-agent", string(body))

	select {
	case clientErr := <-errCh:
		require.NoError(t, clientErr)
	default:
	}
}

func TestTunnelClient_GRPC_ChunkedRequestBodyEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)

	envID := "env-e2e-grpc-chunked-body"
	GetRegistry().Unregister(envID)
	defer GetRegistry().Unregister(envID)

	tunnelServer := NewTunnelServerWithRegistry(GetRegistry(), func(_ context.Context, token string) (string, error) {
		if token != "valid-token" {
			return "", errors.New("invalid token")
		}
		return envID, nil
	}, nil)
	go tunnelServer.StartCleanupLoop(ctx)
	defer func() {
		cancel()
		tunnelServer.WaitForCleanupDone()
	}()

	managerURL, stopManager := startTestGRPCTunnelServerOnAPIPathInternal(t, ctx, tunnelServer)
	defer stopManager()

	wantBody := bytes.Repeat([]byte("arcane-edge-chunk"), 320*1024)
	localHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || r.URL.Path != "/api/environments/0/images/upload" || !bytes.Equal(body, wantBody) {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("uploaded"))
	})

	client := NewTunnelClient(&Config{
		EdgeTransport:         EdgeTransportGRPC,
		ManagerApiUrl:         managerURL,
		AgentToken:            "valid-token",
		EdgeReconnectInterval: 1,
	}, localHandler)
	go client.StartWithErrorChan(ctx, nil)

	var tunnel *AgentTunnel
	require.Eventually(t, func() bool {
		var ok bool
		tunnel, ok = GetRegistry().Get(envID).Get()
		return ok && tunnel != nil && !tunnel.Conn.IsClosed()
	}, 5*time.Second, 20*time.Millisecond)
	require.Contains(t, tunnel.Capabilities, tunnelCapabilityChunkedRequest)

	localStatus, _, body, err := ProxyRequest(ctx, tunnel, http.MethodPost, "/api/environments/0/images/upload", "", nil, wantBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, localStatus)
	assert.Equal(t, "uploaded", string(body))
}

func TestTunnelClient_useTLSForManagerGRPC(t *testing.T) {
	tests := []struct {
		name       string
		managerURL string
		expected   bool
	}{
		{name: "https manager url", managerURL: "https://manager.example.com/api", expected: true},
		{name: "https manager url with reverse proxy path", managerURL: "https://manager.example.com/arcane/api", expected: true},
		{name: "http manager url", managerURL: "http://manager.example.com/api", expected: false},
		{name: "invalid manager url", managerURL: "://bad-url", expected: false},
		{name: "empty manager url", managerURL: "", expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := NewTunnelClient(&Config{ManagerApiUrl: tc.managerURL}, http.NotFoundHandler())
			assert.Equal(t, tc.expected, client.useTLSForManagerGRPC())
		})
	}
}

func TestStartTunnelClient_GRPCValidation(t *testing.T) {
	ctx := t.Context()

	t.Run("edge mode required", func(t *testing.T) {
		_, err := StartTunnelClient(ctx, &Config{}, http.NotFoundHandler())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "edge tunnel disabled")
	})

	t.Run("manager url required for grpc transport", func(t *testing.T) {
		_, err := StartTunnelClient(ctx, &Config{
			EdgeAgent:     true,
			EdgeTransport: EdgeTransportGRPC,
			AgentToken:    "token",
		}, http.NotFoundHandler())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MANAGER_API_URL")
	})

	t.Run("agent token required", func(t *testing.T) {
		_, err := StartTunnelClient(ctx, &Config{
			EdgeAgent:     true,
			EdgeTransport: EdgeTransportGRPC,
			ManagerApiUrl: "https://manager.example.com/arcane/api",
		}, http.NotFoundHandler())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AGENT_TOKEN is required")
	})

	t.Run("mtls requires https manager url", func(t *testing.T) {
		_, err := StartTunnelClient(ctx, &Config{
			EdgeAgent:     true,
			EdgeTransport: EdgeTransportGRPC,
			ManagerApiUrl: "http://manager.example.com/api",
			AgentToken:    "token",
			EdgeMTLSMode:  EdgeMTLSModeRequired,
		}, http.NotFoundHandler())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MANAGER_API_URL to use https")
	})

	t.Run("required mtls auto-enrollment failure surfaces", func(t *testing.T) {
		_, err := StartTunnelClient(ctx, &Config{
			EdgeAgent:     true,
			EdgeTransport: EdgeTransportGRPC,
			ManagerApiUrl: "https://127.0.0.1:1/api",
			AgentToken:    "token",
			EdgeMTLSMode:  EdgeMTLSModeRequired,
		}, http.NotFoundHandler())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "edge mTLS enrollment request failed")
	})
}

func TestTunnelClient_connectAndServeGRPC_EmptyManagerAddress(t *testing.T) {
	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportGRPC,
		AgentToken:    "valid-token",
	}, http.NotFoundHandler())

	err := client.connectAndServeGRPC(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manager gRPC address is empty")
}

func TestTunnelClient_connectAndServeGRPC_RegistrationRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)

	envID := "env-e2e-grpc-reject-1"
	GetRegistry().Unregister(envID)
	defer GetRegistry().Unregister(envID)

	resolver := func(ctx context.Context, token string) (string, error) {
		if token != "valid-token" {
			return "", errors.New("invalid token")
		}
		return envID, nil
	}

	tunnelServer := NewTunnelServerWithRegistry(GetRegistry(), resolver, nil)
	go tunnelServer.StartCleanupLoop(ctx)
	defer func() {
		cancel()
		tunnelServer.WaitForCleanupDone()
	}()

	managerURL, stopManager := startTestGRPCTunnelServerOnAPIPathInternal(t, ctx, tunnelServer)
	defer stopManager()

	client := NewTunnelClient(&Config{
		EdgeTransport:         EdgeTransportGRPC,
		ManagerApiUrl:         managerURL,
		AgentToken:            "invalid-token",
		EdgeReconnectInterval: 1,
		Port:                  "3552",
	}, http.NotFoundHandler())

	err := client.connectAndServeGRPC(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to receive tunnel registration response")
	assert.Contains(t, err.Error(), "invalid agent token")
}

func TestTunnelClient_connectAndServeGRPC_TimesOutWithoutRegisterResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	service := &stallingTunnelService{}
	managerURL, stopManager := startTestTunnelServiceOnAPIPathInternal(t, ctx, service)
	defer stopManager()

	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportGRPC,
		ManagerApiUrl: managerURL,
		AgentToken:    "valid-token",
	}, http.NotFoundHandler())
	client.registrationTimeout = 100 * time.Millisecond

	err := client.connectAndServeGRPC(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, errTunnelRegistrationTimeout)
	assert.Contains(t, err.Error(), "timed out waiting for tunnel registration response")
	assert.Contains(t, err.Error(), "not forwarding gRPC")
	assert.EqualValues(t, 1, service.connectCount.Load())
	active := client.conn.Load()
	assert.True(t, active == nil || active.conn.IsClosed())
}

func TestTunnelClient_awaitRegistrationInternal_ClosesConnOnContextDone(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	conn := newBlockingRegistrationConnInternal()
	client := &TunnelClient{
		registrationTimeout: time.Second,
	}
	client.conn.Store(&connBox{conn: conn})

	msg, err := client.awaitRegistrationInternal(ctx, conn)
	require.Nil(t, msg)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.True(t, conn.IsClosed())
	assert.EqualValues(t, 1, conn.closeCount.Load())
}

func TestTunnelClient_awaitRegistrationInternal_ClosesAttemptConnWhenClientConnChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	firstConn := newBlockingRegistrationConnInternal()
	secondConn := newBlockingRegistrationConnInternal()
	t.Cleanup(func() {
		_ = firstConn.Close()
		_ = secondConn.Close()
	})

	client := &TunnelClient{
		registrationTimeout: time.Second,
	}
	client.conn.Store(&connBox{conn: firstConn})

	type registrationResult struct {
		msg *TunnelMessage
		err error
	}
	resultCh := make(chan registrationResult, 1)
	go func() {
		msg, err := client.awaitRegistrationInternal(ctx, firstConn)
		resultCh <- registrationResult{msg: msg, err: err}
	}()

	select {
	case <-firstConn.receiveStarted:
	case <-time.After(time.Second):
		require.FailNow(t, "registration receive did not start")
	}

	client.conn.Store(&connBox{conn: secondConn})
	cancel()

	select {
	case result := <-resultCh:
		require.Nil(t, result.msg)
		require.ErrorIs(t, result.err, context.Canceled)
	case <-time.After(time.Second):
		require.FailNow(t, "registration did not exit after cancellation")
	}

	assert.True(t, firstConn.IsClosed())
	assert.EqualValues(t, 1, firstConn.closeCount.Load())
	assert.False(t, secondConn.IsClosed())
	assert.EqualValues(t, 0, secondConn.closeCount.Load())
}

func TestTunnelClient_GRPC_WebSocketProxyEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)

	envID := "env-e2e-grpc-ws-1"
	GetRegistry().Unregister(envID)
	defer GetRegistry().Unregister(envID)

	resolver := func(ctx context.Context, token string) (string, error) {
		if token != "valid-token" {
			return "", errors.New("invalid token")
		}
		return envID, nil
	}

	tunnelServer := NewTunnelServerWithRegistry(GetRegistry(), resolver, nil)
	go tunnelServer.StartCleanupLoop(ctx)
	defer func() {
		cancel()
		tunnelServer.WaitForCleanupDone()
	}()

	managerURL, stopManager := startTestGRPCTunnelServerOnAPIPathInternal(t, ctx, tunnelServer)
	defer stopManager()

	headerTokenCh := make(chan string, 1)
	queryCh := make(chan string, 1)
	pathCh := make(chan string, 1)
	receivedMsgCh := make(chan string, 1)
	upgradeErrCh := make(chan string, 1)

	localWSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case pathCh <- r.URL.Path:
		default:
		}
		select {
		case headerTokenCh <- r.Header.Get(HeaderAPIKey):
		default:
		}
		select {
		case queryCh <- r.URL.RawQuery:
		default:
		}

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			select {
			case upgradeErrCh <- err.Error():
			default:
			}
			return
		}
		defer func() { _ = conn.CloseNow() }()

		for {
			mt, data, readErr := conn.Read(r.Context())
			if readErr != nil {
				return
			}
			select {
			case receivedMsgCh <- string(data):
			default:
			}
			if writeErr := conn.Write(r.Context(), mt, append([]byte("local echo: "), data...)); writeErr != nil {
				return
			}
		}
	}))
	defer localWSServer.Close()

	localHost, localPort, err := net.SplitHostPort(localWSServer.Listener.Addr().String())
	require.NoError(t, err)
	localHost = strings.Trim(localHost, "[]")

	cfg := &Config{
		EdgeTransport:         EdgeTransportGRPC,
		ManagerApiUrl:         managerURL,
		AgentToken:            "valid-token",
		EdgeReconnectInterval: 1,
		Listen:                localHost,
		Port:                  localPort,
	}

	client := NewTunnelClient(cfg, http.NotFoundHandler())
	errCh := make(chan error, 4)
	go client.StartWithErrorChan(ctx, errCh)

	require.Eventually(t, func() bool {
		tunnel, ok := GetRegistry().Get(envID).Get()
		return ok && tunnel != nil && !tunnel.Conn.IsClosed()
	}, 5*time.Second, 20*time.Millisecond)

	router := echo.New()
	router.GET("/proxy-ws", func(c *echo.Context) error {
		tunnel, ok := GetRegistry().Get(envID).Get()
		if !ok || tunnel == nil {
			return c.NoContent(http.StatusServiceUnavailable)
		}
		return ProxyWebSocketRequest(c, tunnel, "/api/environments/0/ws/system/stats", func(*http.Request) bool { return true })
	})

	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	proxyURL := "ws" + strings.TrimPrefix(proxyServer.URL, "http") + "/proxy-ws?tail=100"
	proxyConn, _, err := websocket.Dial(ctx, proxyURL, nil)
	require.NoError(t, err)
	defer func() { _ = proxyConn.CloseNow() }()

	require.NoError(t, proxyConn.Write(ctx, websocket.MessageText, []byte("hello-grpc-ws")))

	msgType, payload, err := proxyConn.Read(ctx)
	select {
	case upgradeErr := <-upgradeErrCh:
		require.FailNowf(t, "unexpected failure", "local websocket upgrade failed: %s", upgradeErr)
	default:
	}
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, msgType)
	assert.Equal(t, "local echo: hello-grpc-ws", string(payload))

	select {
	case got := <-pathCh:
		assert.Equal(t, "/api/environments/0/ws/system/stats", got)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout waiting for forwarded websocket path")
	}

	select {
	case got := <-headerTokenCh:
		assert.Equal(t, "valid-token", got)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout waiting for local websocket auth header")
	}

	select {
	case got := <-queryCh:
		assert.Equal(t, "tail=100", got)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout waiting for forwarded query")
	}

	select {
	case got := <-receivedMsgCh:
		assert.Equal(t, "hello-grpc-ws", got)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout waiting for forwarded websocket payload")
	}

	select {
	case clientErr := <-errCh:
		require.NoError(t, clientErr)
	default:
	}
}
