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
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	tunnelpb "github.com/florianfinn/docklet-next/backend/proto/tunnel/v1"
)

func TestTunnelClient_connectAndServe_WebSocketConfigFallsBackToWebSocket(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	wsConnectedCh := make(chan struct{}, 1)
	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tunnel/connect" {
			http.NotFound(w, r)
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()

		select {
		case wsConnectedCh <- struct{}{}:
		default:
		}

		time.Sleep(100 * time.Millisecond)
	}))
	defer managerServer.Close()

	cfg := &Config{
		EdgeTransport: EdgeTransportWebSocket,
		ManagerApiUrl: managerServer.URL,
		AgentToken:    "valid-token",
	}

	client := NewTunnelClient(cfg, http.NotFoundHandler())
	err := client.connectAndServe(ctx)
	require.Error(t, err)

	select {
	case <-wsConnectedCh:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "expected websocket fallback connection to manager")
	}
}

func TestTunnelClient_managedTunnelTransports_AutoEnablesGRPCAndWebSocket(t *testing.T) {
	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportAuto,
		ManagerApiUrl: "http://manager.example.com",
		AgentToken:    "valid-token",
	}, http.NotFoundHandler())

	transports := client.managedTunnelTransportsInternal()
	assert.True(t, transports.grpc)
	assert.True(t, transports.websocket)
}

func TestTunnelClient_connectAndServe_AutoFallsBackToWebSocketWhenGRPCUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	managerURL, wsConnectedCh, stopManager := startTestWebSocketTunnelManagerInternal(t, ctx)
	defer stopManager()

	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportAuto,
		ManagerApiUrl: managerURL,
		AgentToken:    "valid-token",
	}, http.NotFoundHandler())
	grpcAddr, releaseGRPCAddr := reserveTCPAddressInternal(t)
	defer releaseGRPCAddr()
	client.managerGRPCAddr = grpcAddr
	client.registrationTimeout = 100 * time.Millisecond

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.connectAndServe(ctx)
	}()

	select {
	case <-wsConnectedCh:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "expected auto transport to fall back to websocket when gRPC is unavailable")
	}

	cancel()

	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout waiting for auto fallback tunnel shutdown")
	}
}

func TestTunnelClient_connectAndServe_AutoFallsBackToWebSocketWhenGRPCSetupHangs(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	grpcAddr, stopGRPC := startHangingTCPServerInternal(t, ctx)
	defer stopGRPC()

	managerURL, wsConnectedCh, stopManager := startTestWebSocketTunnelManagerInternal(t, ctx)
	defer stopManager()

	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportAuto,
		ManagerApiUrl: managerURL,
		AgentToken:    "valid-token",
	}, http.NotFoundHandler())
	client.managerGRPCAddr = grpcAddr
	client.registrationTimeout = 100 * time.Millisecond

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.connectAndServe(ctx)
	}()

	select {
	case <-wsConnectedCh:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "expected auto transport to fall back to websocket when gRPC setup hangs")
	}

	cancel()

	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout waiting for auto fallback tunnel shutdown")
	}
}

func TestTunnelClient_connectAndServe_GRPCDoesNotFallbackToWebSocket(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	managerURL, wsConnectedCh, stopManager := startTestWebSocketTunnelManagerInternal(t, ctx)
	defer stopManager()

	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportGRPC,
		ManagerApiUrl: managerURL,
		AgentToken:    "valid-token",
	}, http.NotFoundHandler())
	grpcAddr, releaseGRPCAddr := reserveTCPAddressInternal(t)
	defer releaseGRPCAddr()
	client.managerGRPCAddr = grpcAddr
	client.registrationTimeout = 100 * time.Millisecond

	err := client.connectAndServe(ctx)
	require.Error(t, err)

	select {
	case <-wsConnectedCh:
		require.FailNow(t, "explicit gRPC transport should not fall back to websocket")
	default:
	}
}

func TestTunnelClient_connectAndServe_OpensGRPCWhenAvailable(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	envID := "env-auto-poll-grpc"
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

	managerURL, stopManager := startTestGRPCTunnelServerOnAPIPathInternal(t, ctx, tunnelServer)
	defer stopManager()

	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportGRPC,
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
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout waiting for gRPC tunnel shutdown")
	}
}

func startTestWebSocketTunnelManagerInternal(t *testing.T, ctx context.Context) (string, <-chan struct{}, func()) {
	t.Helper()

	wsConnectedCh := make(chan struct{}, 1)
	managerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tunnel/connect" {
			http.NotFound(w, r)
			return
		}

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
			EnvironmentID: "env-ws-fallback",
			SessionID:     "session-ws-fallback",
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
	}))

	return managerServer.URL, wsConnectedCh, managerServer.Close
}

func reserveTCPAddressInternal(t *testing.T) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return lis.Addr().String(), func() {
		require.NoError(t, lis.Close())
	}
}

func startHangingTCPServerInternal(t *testing.T, ctx context.Context) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, acceptErr := lis.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				<-ctx.Done()
			}()
		}
	}()

	stop := func() {
		_ = lis.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			require.FailNow(t, "timeout waiting for hanging TCP server shutdown")
		}
	}

	return lis.Addr().String(), stop
}

func startTestGRPCTunnelServerOnAPIPathInternal(t *testing.T, ctx context.Context, tunnelServer *TunnelServer) (string, func()) {
	t.Helper()
	return startTestTunnelServiceOnAPIPathInternal(t, ctx, tunnelServer)
}

func startTestTunnelServiceOnAPIPathInternal(t *testing.T, ctx context.Context, service tunnelpb.TunnelServiceServer) (string, func()) {
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
