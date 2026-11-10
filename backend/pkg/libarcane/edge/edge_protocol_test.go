// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Edge protocol marker exchange tests over the real gRPC, WebSocket and client paths.

package edge

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	tunnelpb "github.com/florianfinn/docklet-next/backend/proto/tunnel/v1"
	"github.com/florianfinn/docklet-next/types/contract"
)

func edgeProtocolMarkersInternal(capabilities []string) []string {
	var markers []string
	for _, capability := range capabilities {
		if contract.IsEdgeProtocolCapability(capability) {
			markers = append(markers, capability)
		}
	}
	return markers
}

func TestEdgeProtocolGRPCRegistrationRecordsAgentAndRepliesWithOwnMarker(t *testing.T) {
	cases := []struct {
		name  string
		caps  []string
		agent int
	}{
		{"no marker counts as one", []string{tunnelCapabilityChunkedRequest}, 1},
		{"newer agent marker is recorded, never echoed", []string{"edge-protocol/2", tunnelCapabilityChunkedRequest}, 2},
		{"duplicate markers count as one", []string{"edge-protocol/1", "edge-protocol/1"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			envID := "env-edge-protocol-grpc"
			GetRegistry().Unregister(envID)
			defer GetRegistry().Unregister(envID)
			lis, grpcServer, tunnelServer := startTestGRPCTunnelServer(ctx, envID)
			defer func() {
				cancel()
				tunnelServer.WaitForCleanupDone()
			}()
			go func() { _ = grpcServer.Serve(lis) }()
			defer grpcServer.GracefulStop()

			conn, err := grpc.NewClient("passthrough:///bufnet",
				grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			// startTestGRPCTunnelServer accepts exactly this fixture value.
			fixture := "valid-token"
			stream, err := tunnelpb.NewTunnelServiceClient(conn).Connect(testGRPCOutgoingContextInternal(ctx, fixture))
			require.NoError(t, err)
			require.NoError(t, stream.Send(&tunnelpb.AgentMessage{Payload: &tunnelpb.AgentMessage_Register{Register: &tunnelpb.RegisterRequest{
				AgentToken: fixture, Capabilities: tc.caps,
			}}}))

			reply, err := stream.Recv()
			require.NoError(t, err)
			require.True(t, reply.GetRegisterResponse().GetAccepted())
			require.Equal(t, []string{"edge-protocol/1"}, edgeProtocolMarkersInternal(reply.GetRegisterResponse().GetCapabilities()))

			var tunnel *AgentTunnel
			require.Eventually(t, func() bool {
				var ok bool
				tunnel, ok = GetRegistry().Get(envID).Get()
				return ok
			}, 2*time.Second, time.Millisecond)
			require.Equal(t, tc.agent, tunnel.EdgeProtocol)
		})
	}
}

func newEdgeProtocolWebSocketServerInternal(t *testing.T, registry *TunnelRegistry) *httptest.Server {
	t.Helper()
	server := NewTunnelServerWithRegistry(registry, func(_ context.Context, token string) (string, error) {
		if token != "example" {
			return "", errors.New("invalid token")
		}
		return "example-host", nil
	}, nil)
	server.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return "example-key", nil })
	router := echo.New()
	router.GET("/api/tunnel/connect", server.HandleConnect)
	return httptest.NewServer(router)
}

func TestEdgeProtocolWebSocketRegistrationFiltersPeerMarker(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	registry := NewTunnelRegistry()
	httpServer := newEdgeProtocolWebSocketServerInternal(t, registry)
	defer httpServer.Close()

	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(httpServer.URL, "http")+"/api/tunnel/connect", nil)
	require.NoError(t, err)
	defer conn.CloseNow()
	require.NoError(t, wsjson.Write(ctx, conn, &TunnelMessage{Type: MessageTypeRegister, AgentToken: "example", Capabilities: []string{"edge-protocol/2"}}))
	var reply TunnelMessage
	require.NoError(t, wsjson.Read(ctx, conn, &reply))
	require.True(t, reply.Accepted, "rejected: %q", reply.Error)
	require.Equal(t, []string{"edge-protocol/1"}, edgeProtocolMarkersInternal(reply.Capabilities))

	var tunnel *AgentTunnel
	require.Eventually(t, func() bool {
		var ok bool
		tunnel, ok = registry.Get("example-host").Get()
		return ok
	}, time.Second, time.Millisecond)
	require.Equal(t, 2, tunnel.EdgeProtocol)
}

func TestEdgeProtocolRealClientAndServerRecordEachOther(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	registry := NewTunnelRegistry()
	httpServer := newEdgeProtocolWebSocketServerInternal(t, registry)
	defer httpServer.Close()

	client := NewTunnelClient(&Config{
		EdgeTransport: EdgeTransportWebSocket,
		ManagerApiUrl: httpServer.URL,
		AgentToken:    "example",
	}, http.NotFoundHandler())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = client.connectAndServe(ctx)
	}()

	var tunnel *AgentTunnel
	require.Eventually(t, func() bool {
		var ok bool
		tunnel, ok = registry.Get("example-host").Get()
		registration, _ := client.registration.Load()
		return ok && registration.sessionID != ""
	}, 3*time.Second, 5*time.Millisecond)
	require.Equal(t, contract.EdgeProtocolVersion, client.HubEdgeProtocol())
	require.Equal(t, contract.EdgeProtocolVersion, tunnel.EdgeProtocol)
	require.Equal(t, []string{"edge-protocol/1"}, edgeProtocolMarkersInternal(tunnel.MetadataSnapshot().Capabilities))

	cancel()
	<-done
}
