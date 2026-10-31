// Source: docklet-next/issues/302
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original frozen-peer tests on the shipped shared HTTP/gRPC listener.

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	tunnelpb "github.com/florianfinn/docklet-next/backend/proto/tunnel/v1"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestFrozenEdgePeersReapedOnShippedServeHTTPListener(t *testing.T) {
	for _, transport := range []string{"websocket", "grpc"} {
		t.Run(transport, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			registry := edge.NewTunnelRegistry()
			id := "example-frozen-" + transport
			state := make(chan bool, 8)
			swept := make(chan struct{}, 8)
			ticks := make(chan time.Time)
			var clockNanos atomic.Int64
			clockNanos.Store(time.Now().UnixNano())
			heartbeatAck := make(chan struct{}, 8)
			var heartbeat func()
			server := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return id, nil }, func(_ context.Context, _ string, connected bool) { state <- connected })
			server.SetCleanupSchedule(&edge.CleanupSchedule{Now: func() time.Time { return time.Unix(0, clockNanos.Load()) }, Ticks: ticks, AfterSweep: func() { swept <- struct{}{} }})
			router := echo.New()
			router.GET("/api/tunnel/connect", server.HandleConnect)
			handler, grpcServer := configureTunnelServerInternal(ctx, &config.Config{}, router, server, nil, "127.0.0.1:0")
			host := httptest.NewUnstartedServer(handler)
			protocols := new(http.Protocols)
			protocols.SetHTTP1(true)
			protocols.SetUnencryptedHTTP2(true)
			host.Config.Protocols = protocols
			host.Start()
			defer host.Close()
			defer grpcServer.Stop()
			go server.StartCleanupLoop(ctx)
			defer func() { cancel(); server.WaitForCleanupDone() }()
			if transport == "websocket" {
				conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(host.URL, "http")+"/api/tunnel/connect", nil)
				require.NoError(t, err)
				defer conn.CloseNow()
				require.NoError(t, wsjson.Write(ctx, conn, &edge.TunnelMessage{Type: edge.MessageTypeRegister, AgentToken: "example"}))
				var ack edge.TunnelMessage
				require.NoError(t, wsjson.Read(ctx, conn, &ack))
				require.True(t, ack.Accepted)
				heartbeat = func() {
					require.NoError(t, wsjson.Write(ctx, conn, &edge.TunnelMessage{ID: "example-heartbeat", Type: edge.MessageTypeHeartbeat}))
				}
				// Read the actual heartbeat acknowledgement before advancing the sweep.
				go func() {
					for {
						var msg edge.TunnelMessage
						if wsjson.Read(ctx, conn, &msg) != nil {
							return
						}
						if msg.Type == edge.MessageTypeHeartbeatAck {
							heartbeatAck <- struct{}{}
						}
					}
				}()
			} else {
				conn, err := grpc.NewClient(strings.TrimPrefix(host.URL, "http://"), grpc.WithTransportCredentials(insecure.NewCredentials()))
				require.NoError(t, err)
				defer conn.Close()
				stream, err := conn.NewStream(metadata.NewOutgoingContext(ctx, metadata.Pairs("x-arcane-agent-token", "example")), &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/api/tunnel/connect")
				require.NoError(t, err)
				require.NoError(t, stream.SendMsg(&tunnelpb.AgentMessage{Payload: &tunnelpb.AgentMessage_Register{Register: &tunnelpb.RegisterRequest{AgentToken: "example"}}}))
				var ack tunnelpb.ManagerMessage
				require.NoError(t, stream.RecvMsg(&ack))
				require.True(t, ack.GetRegisterResponse().GetAccepted())
				heartbeat = func() {
					require.NoError(t, stream.SendMsg(&tunnelpb.AgentMessage{Payload: &tunnelpb.AgentMessage_HeartbeatPing{HeartbeatPing: &tunnelpb.HeartbeatPing{}}}))
				}
				go func() {
					for {
						var msg tunnelpb.ManagerMessage
						if stream.RecvMsg(&msg) != nil {
							return
						}
						if msg.GetHeartbeatPong() != nil {
							heartbeatAck <- struct{}{}
						}
					}
				}()
			}
			select {
			case connected := <-state:
				require.True(t, connected)
			case <-ctx.Done():
				t.Fatal("registration barrier missing")
			}
			tunnel, ok := registry.Get(id).Get()
			require.True(t, ok)
			base := tunnel.GetLastHeartbeat()
			sweep := func(now time.Time) {
				t.Helper()
				select {
				case ticks <- now:
				case <-ctx.Done():
					t.Fatal("sweep tick not delivered")
				}
				select {
				case <-swept:
				case <-ctx.Done():
					t.Fatal("sweep not joined")
				}
			}
			clockNanos.Store(base.Add(60 * time.Second).UnixNano())
			heartbeat()
			select {
			case <-heartbeatAck:
			case <-ctx.Done():
				t.Fatal("heartbeat acknowledgement missing")
			}
			require.Equal(t, base.Add(60*time.Second).UnixNano(), tunnel.GetLastHeartbeat().UnixNano())
			sweep(base.Add(105 * time.Second))
			require.False(t, tunnel.Conn.IsClosed(), "healthy peer stays live beyond initial 90 seconds")
			sweep(base.Add(150 * time.Second))
			require.False(t, tunnel.Conn.IsClosed(), "inclusive 90-second window")
			sweep(base.Add(165 * time.Second))
			require.True(t, registry.Get(id).IsAbsent(), "frozen transport must be removed by 105 seconds")
			require.True(t, tunnel.Conn.IsClosed())
			require.Equal(t, "edge.heartbeat-stale", tunnel.DisconnectErr)
		})
	}
}
