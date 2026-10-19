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
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v5"
	slogecho "github.com/samber/slog-echo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tunnelpb "github.com/florianfinn/docklet-next/backend/proto/tunnel/v1"
)

func TestTunnelClient_connectAndServe_AutoDoesNotFallbackWhenEstablishedGRPCSessionDrops(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	envID := "env-auto-grpc-drop"
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
	defer tunnelServer.WaitForCleanupDone()

	grpcManagerURL, stopGRPCManager := startTestGRPCTunnelServerOnAPIPathInternal(t, ctx, tunnelServer)
	defer stopGRPCManager()

	wsManagerURL, wsConnectedCh, stopWSManager := startTestWebSocketTunnelManagerInternal(t, ctx)
	defer stopWSManager()

	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportAuto,
		ManagerApiUrl: wsManagerURL,
		AgentToken:    "valid-token",
	}, http.NotFoundHandler())
	grpcAddr, err := url.Parse(grpcManagerURL)
	require.NoError(t, err)
	client.managerGRPCAddr = grpcAddr.Host
	client.healthySessionDuration = time.Millisecond

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.connectAndServe(ctx)
	}()

	var tunnel *AgentTunnel
	require.Eventually(t, func() bool {
		registered, ok := GetRegistry().Get(envID).Get()
		if !ok || registered == nil || registered.Conn == nil || registered.Conn.IsClosed() {
			return false
		}
		tunnel = registered
		return true
	}, 3*time.Second, 20*time.Millisecond)
	require.Equal(t, EdgeTransportGRPC, tunnel.Conn.Transport())

	time.Sleep(5 * time.Millisecond)
	require.NoError(t, tunnel.CloseWithReason("manager restart"))

	select {
	case operationErr := <-errCh:
		require.ErrorIs(t, operationErr, errEstablishedTunnelSessionEnded)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "expected dropped gRPC session to return instead of falling back to websocket")
	}

	select {
	case <-wsConnectedCh:
		require.FailNow(t, "dropped gRPC session must not fall back to websocket")
	default:
	}
	assert.Zero(t, client.grpcFailureStreakInternal())
	cancel()
}

func TestTunnelClient_connectAndServePoll_OpensGRPCWhenRequired(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	envID := "env-poll-grpc"
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

	managerURL, stopManager := startTestPollAndGRPCManagerInternal(t, ctx, tunnelServer, TunnelPollResponse{
		Status:              TunnelStatusRequired,
		PollIntervalSeconds: 1,
	})
	defer stopManager()

	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportPoll,
		ManagerApiUrl: managerURL,
		AgentToken:    "valid-token",
	}, http.NotFoundHandler())

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.connectAndServe(ctx)
	}()

	require.Eventually(t, func() bool {
		tunnel, ok := GetRegistry().Get(envID).Get()
		if !ok || tunnel == nil || tunnel.Conn == nil || tunnel.Conn.IsClosed() {
			return false
		}
		_, isGRPC := tunnel.Conn.(*GRPCManagerTunnelConn)
		return isGRPC
	}, 3*time.Second, 20*time.Millisecond)

	cancel()
	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "expected poll-managed gRPC session to stop after cancellation")
	}
}

func TestTunnelClient_connectAndServePoll_OpensWebSocketWhenRequired(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	var pollCount atomic.Int32
	wsConnectedCh := make(chan struct{}, 1)

	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tunnel/poll":
			pollCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(TunnelPollResponse{
				Status:              TunnelStatusRequired,
				PollIntervalSeconds: 1,
			})) {
				return
			}
		case "/api/tunnel/connect":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer func() { _ = conn.CloseNow() }()

			if _, _, readErr := conn.Read(r.Context()); readErr != nil {
				return
			}
			registerResp, err := json.Marshal(&TunnelMessage{
				Type:          MessageTypeRegisterResponse,
				Accepted:      true,
				EnvironmentID: "env-poll-ws",
				SessionID:     "session-poll-ws",
			})
			if err != nil {
				return
			}
			if writeErr := conn.Write(r.Context(), websocket.MessageText, registerResp); writeErr != nil {
				return
			}

			select {
			case wsConnectedCh <- struct{}{}:
			default:
			}

			<-ctx.Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer managerServer.Close()

	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportPoll,
		ManagerApiUrl: managerServer.URL,
		AgentToken:    "valid-token",
	}, http.NotFoundHandler())
	client.registrationTimeout = 100 * time.Millisecond

	err := client.connectAndServe(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	select {
	case <-wsConnectedCh:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "expected poll transport to open websocket tunnel when required")
	}

	assert.GreaterOrEqual(t, pollCount.Load(), int32(1))
}

func TestTunnelClient_pollTunnelControlInternal_UsesConfiguredHTTPClient(t *testing.T) {
	t.Parallel()

	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !assert.NoError(t, json.NewEncoder(w).Encode(TunnelPollResponse{
			Status:              TunnelStatusIdle,
			PollIntervalSeconds: 1,
		})) {
			return
		}
	}))
	defer managerServer.Close()

	baseClient := managerServer.Client()
	rewriteTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = "http"
		clone.URL.Host = strings.TrimPrefix(managerServer.URL, "http://")
		return baseClient.Transport.RoundTrip(clone)
	})

	client := NewTunnelClient(&Config{AgentToken: "valid-token"}, http.NotFoundHandler())
	httpClient := &http.Client{Transport: rewriteTransport}

	resp, err := client.pollTunnelControlInternal(t.Context(), httpClient, "http://127.0.0.1:1/api/tunnel/poll", false)
	require.NoError(t, err)
	assert.Equal(t, TunnelStatusIdle, resp.Status)
	assert.Equal(t, 1, resp.PollIntervalSeconds)
	assert.NotSame(t, http.DefaultClient, httpClient)

	defaultReq, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://127.0.0.1:1/api/tunnel/poll", http.NoBody)
	require.NoError(t, err)
	_, err = http.DefaultClient.Do(defaultReq)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "127.0.0.1:1")
}

func TestTunnelClient_connectAndServePoll_DoesNotOpenWebSocketWhenIdle(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	wsConnectedCh := make(chan struct{}, 1)

	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tunnel/poll":
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(TunnelPollResponse{
				Status:              TunnelStatusIdle,
				PollIntervalSeconds: 1,
			})) {
				return
			}
		case "/api/tunnel/connect":
			select {
			case wsConnectedCh <- struct{}{}:
			default:
			}
			http.Error(w, "unexpected websocket connect", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer managerServer.Close()

	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportPoll,
		ManagerApiUrl: managerServer.URL,
		AgentToken:    "valid-token",
	}, http.NotFoundHandler())

	err := client.connectAndServe(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	select {
	case <-wsConnectedCh:
		require.FailNow(t, "did not expect idle poll transport to open websocket tunnel")
	default:
	}
}

func TestTunnelClient_connectAndServePoll_RetriesAfterTransientPollError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	var pollCount atomic.Int32
	wsConnectedCh := make(chan struct{}, 1)

	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tunnel/poll":
			currentPoll := pollCount.Add(1)
			if currentPoll == 1 {
				http.Error(w, "temporary failure", http.StatusBadGateway)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(TunnelPollResponse{
				Status:              TunnelStatusRequired,
				PollIntervalSeconds: 1,
			})) {
				return
			}
		case "/api/tunnel/connect":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer func() { _ = conn.CloseNow() }()

			if _, _, readErr := conn.Read(r.Context()); readErr != nil {
				return
			}
			registerResp, err := json.Marshal(&TunnelMessage{
				Type:          MessageTypeRegisterResponse,
				Accepted:      true,
				EnvironmentID: "env-poll-retry-ws",
				SessionID:     "session-poll-retry-ws",
			})
			if err != nil {
				return
			}
			if writeErr := conn.Write(r.Context(), websocket.MessageText, registerResp); writeErr != nil {
				return
			}

			select {
			case wsConnectedCh <- struct{}{}:
			default:
			}

			<-ctx.Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer managerServer.Close()

	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportPoll,
		ManagerApiUrl: managerServer.URL,
		AgentToken:    "valid-token",
	}, http.NotFoundHandler())
	client.registrationTimeout = 100 * time.Millisecond

	err := client.connectAndServe(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	select {
	case <-wsConnectedCh:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "expected poll transport to recover after transient poll error")
	}

	assert.GreaterOrEqual(t, pollCount.Load(), int32(2))
}

func TestTunnelClient_stopPollManagedSessionInternal_DeadlineExceededReturnsTimeoutMessage(t *testing.T) {
	t.Parallel()

	session := &pollManagedTunnelSession{
		cancel: func() {},
		done:   make(chan error),
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	err := (&TunnelClient{}).stopPollManagedSessionInternal(ctx, session)
	require.Error(t, err)
	assert.EqualError(t, err, "timed out waiting for poll-managed websocket session to stop")
}

func TestTunnelClient_syncPollManagedSessionInternal_IdleUsesBoundedStopTimeout(t *testing.T) {
	previousTimeout := defaultPollManagedSessionStopTimeout
	defaultPollManagedSessionStopTimeout = 20 * time.Millisecond
	defer func() {
		defaultPollManagedSessionStopTimeout = previousTimeout
	}()

	session := &pollManagedTunnelSession{
		cancel: func() {},
		done:   make(chan error),
	}

	nextSession, err := (&TunnelClient{}).syncPollManagedSessionInternal(t.Context(), session, TunnelStatusIdle)
	require.Same(t, session, nextSession)
	require.Error(t, err)
	require.EqualError(t, err, "timed out waiting for poll-managed websocket session to stop")

	fencedSession, err := (&TunnelClient{}).syncPollManagedSessionInternal(t.Context(), nextSession, TunnelStatusRequired)
	require.NoError(t, err)
	require.Same(t, session, fencedSession)

	started := time.Now()
	stillDraining, err := (&TunnelClient{}).syncPollManagedSessionInternal(t.Context(), nextSession, TunnelStatusIdle)
	require.NoError(t, err)
	require.Same(t, session, stillDraining)
	require.Less(t, time.Since(started), defaultPollManagedSessionStopTimeout)
}

func startTestPollAndGRPCManagerInternal(t *testing.T, ctx context.Context, service tunnelpb.TunnelServiceServer, pollResp TunnelPollResponse) (string, func()) {
	t.Helper()

	var serverOptions []grpc.ServerOption
	if tunnelServer, ok := service.(*TunnelServer); ok {
		serverOptions = tunnelServer.GRPCServerOptions()
	}
	grpcServer := grpc.NewServer(serverOptions...)
	tunnelpb.RegisterTunnelServiceServer(grpcServer, service)

	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tunnel/poll" {
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(pollResp)) {
				return
			}
			return
		}

		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		clone := r.Clone(r.Context())
		if r.URL.Path == "/api/tunnel/connect" {
			clone.URL.Path = tunnelpb.TunnelService_Connect_FullMethodName
		} else {
			clone.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
		}
		clone.RequestURI = clone.URL.Path
		grpcServer.ServeHTTP(w, clone)
	})

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	httpServer := &http.Server{
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		_ = httpServer.Serve(lis)
	}()

	cleanup := func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer shutdownCancel()
		_ = httpServer.Shutdown(shutdownCtx)
		grpcServer.Stop()
		_ = lis.Close()
	}

	return "http://" + lis.Addr().String(), cleanup
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	if f == nil {
		return nil, errors.New("round tripper is nil")
	}
	return f(req)
}

func TestTunnelClient_InternalRequestSkipsSlogEcho(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, client *TunnelClient)
	}{
		{
			name: "legacy response",
			run: func(t *testing.T, client *TunnelClient) {
				conn := &capturingTunnelConnForHandleRequest{}
				client.conn.Store(&connBox{conn: conn})

				client.handleRequest(t.Context(), conn, &TunnelMessage{
					ID:     "req-legacy",
					Type:   MessageTypeRequest,
					Method: http.MethodGet,
					Path:   "/local/api",
				})

				require.Len(t, conn.sent, 1)
				assert.Equal(t, MessageTypeResponse, conn.sent[0].Type)
				assert.Equal(t, http.StatusOK, conn.sent[0].Status)
				assert.Equal(t, "local response", string(conn.sent[0].Body))
			},
		},
		{
			name: "streaming response",
			run: func(t *testing.T, client *TunnelClient) {
				conn := &fakeTunnelConn{}
				client.conn.Store(&connBox{conn: conn})

				client.handleRequestStreaming(t.Context(), conn, &TunnelMessage{
					ID:     "req-stream",
					Type:   MessageTypeRequest,
					Method: http.MethodGet,
					Path:   "/local/api",
				})

				require.Len(t, conn.msgs, 3)
				assert.Equal(t, MessageTypeResponse, conn.msgs[0].Type)
				assert.Equal(t, http.StatusOK, conn.msgs[0].Status)
				assert.Equal(t, MessageTypeStreamData, conn.msgs[1].Type)
				assert.Equal(t, "local response", string(conn.msgs[1].Body))
				assert.Equal(t, MessageTypeStreamEnd, conn.msgs[2].Type)
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var sawInternalTunnelRequest bool
			loggerMiddleware := slogecho.New(slog.Default())

			router := echo.New()
			router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c *echo.Context) error {
					if IsInternalTunnelRequest(c.Request().Context()) {
						sawInternalTunnelRequest = true
						return next(c)
					}
					return loggerMiddleware(next)(c)
				}
			})
			router.GET("/local/api", func(c *echo.Context) error {
				return c.String(http.StatusOK, "local response")
			})

			client := NewTunnelClient(&Config{}, router)
			testCase.run(t, client)

			assert.True(t, sawInternalTunnelRequest)
		})
	}
}

type fakeTunnelConn struct {
	mu         sync.Mutex
	msgs       []*TunnelMessage
	closed     bool
	sendErr    error
	receiveErr error
	transport  string
}

func (f *fakeTunnelConn) Send(msg *TunnelMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	copyMsg := *msg
	if msg.Headers != nil {
		copyMsg.Headers = cloneHeaderMap(msg.Headers)
	}
	if msg.Body != nil {
		copyMsg.Body = append([]byte(nil), msg.Body...)
	}
	f.msgs = append(f.msgs, &copyMsg)
	return nil
}

func (f *fakeTunnelConn) Receive() (*TunnelMessage, error) {
	if f.receiveErr == nil {
		return nil, errors.New("not implemented")
	}
	return nil, f.receiveErr
}

func (f *fakeTunnelConn) IsExpectedReceiveError(error) bool {
	return false
}

func (f *fakeTunnelConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeTunnelConn) IsClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *fakeTunnelConn) Transport() string {
	if f.transport == "" {
		return EdgeTransportWebSocket
	}
	return f.transport
}

func TestTunnelClient_serveTunnelSessionInternal_GRPCSendEOFSurfacesRegistrationError(t *testing.T) {
	conn := &fakeTunnelConn{
		sendErr:    io.EOF,
		receiveErr: status.Error(codes.Unauthenticated, "invalid agent token"),
		transport:  EdgeTransportGRPC,
	}
	client := NewTunnelClient(&Config{}, http.NotFoundHandler())

	err := client.serveTunnelSessionInternal(t.Context(), conn, "manager.test")
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Contains(t, err.Error(), "invalid agent token")
}

func TestStreamingResponseRecorder_Sequence(t *testing.T) {
	conn := &fakeTunnelConn{}
	r := newStreamingResponseRecorder("req-1", conn)

	r.Header().Set("Content-Type", "text/plain")

	n, err := r.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	n, err = r.Write([]byte(" world"))
	require.NoError(t, err)
	assert.Equal(t, 6, n)

	require.NoError(t, r.Close())

	require.Len(t, conn.msgs, 4)
	assert.Equal(t, MessageTypeResponse, conn.msgs[0].Type)
	assert.Equal(t, "req-1", conn.msgs[0].ID)
	assert.Equal(t, "text/plain", conn.msgs[0].Headers["Content-Type"])
	assert.Equal(t, "1", conn.msgs[0].Headers["X-Arcane-Tunnel-Stream"])

	assert.Equal(t, MessageTypeStreamData, conn.msgs[1].Type)
	assert.Equal(t, "hello", string(conn.msgs[1].Body))

	assert.Equal(t, MessageTypeStreamData, conn.msgs[2].Type)
	assert.Equal(t, " world", string(conn.msgs[2].Body))

	assert.Equal(t, MessageTypeStreamEnd, conn.msgs[3].Type)
}

func TestStreamingResponseRecorder_WriteHeaderAndClose(t *testing.T) {
	conn := &fakeTunnelConn{}
	r := newStreamingResponseRecorder("req-2", conn)

	r.WriteHeader(http.StatusCreated)
	require.NoError(t, r.Close())

	require.Len(t, conn.msgs, 2)
	assert.Equal(t, MessageTypeResponse, conn.msgs[0].Type)
	assert.Equal(t, http.StatusCreated, conn.msgs[0].Status)
	assert.Equal(t, MessageTypeStreamEnd, conn.msgs[1].Type)
}

func TestConnectAndServeWebSocket_CancelUnblocksMessageLoop(t *testing.T) {
	registered := make(chan struct{})
	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		_, _, err = conn.Read(r.Context())
		if err != nil {
			return
		}
		registerResp, _ := json.Marshal(&TunnelMessage{
			Type:      MessageTypeRegisterResponse,
			Accepted:  true,
			SessionID: "session-cancel",
		})
		_ = conn.Write(r.Context(), websocket.MessageText, registerResp)
		close(registered)
		// Go silent: keep reading so the socket stays open.
		for {
			if _, _, readErr := conn.Read(r.Context()); readErr != nil {
				return
			}
		}
	}))
	defer managerServer.Close()

	cfg := &Config{
		EdgeTransport: EdgeTransportWebSocket,
		ManagerApiUrl: managerServer.URL,
		AgentToken:    "test-token",
	}
	client := NewTunnelClient(cfg, http.NotFoundHandler())
	client.managerURL = "ws" + strings.TrimPrefix(managerServer.URL, "http")

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- client.connectAndServeWebSocketInternal(ctx) }()

	select {
	case <-registered:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for registration")
	}
	cancel()

	select {
	case <-done:
		// Cancellation must close the socket and unblock the message loop well
		// under the poll-managed teardown timeout.
	case <-time.After(defaultPollManagedSessionStopTimeout):
		require.FailNow(t, "messageLoop did not unblock after context cancellation")
	}
}
