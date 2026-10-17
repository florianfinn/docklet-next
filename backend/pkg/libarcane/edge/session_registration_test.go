// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package edge

import (
	"context"
	"errors"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/labstack/echo/v5"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tunnelpb "github.com/florianfinn/docklet-next/backend/proto/tunnel/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type memoryRegistrationStream struct {
	grpc.ServerStream
	ctx        context.Context
	first      bool
	token      string
	sent       []*tunnelpb.ManagerMessage
	onRegister func()
}

func (s *memoryRegistrationStream) Context() context.Context { return s.ctx }
func (s *memoryRegistrationStream) Recv() (*tunnelpb.AgentMessage, error) {
	if s.first {
		return nil, io.EOF
	}
	s.first = true
	return &tunnelpb.AgentMessage{Payload: &tunnelpb.AgentMessage_Register{Register: &tunnelpb.RegisterRequest{AgentToken: s.token}}}, nil
}
func (s *memoryRegistrationStream) Send(m *tunnelpb.ManagerMessage) error {
	s.sent = append(s.sent, m)
	if s.onRegister != nil {
		s.onRegister()
	}
	return nil
}

func TestEdgeRegistrationInProcessTokenLifecycle(t *testing.T) {
	for _, state := range []string{"valid", "wrong", "revoked", "revoked-after-auth"} {
		t.Run(state, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			token := "example"
			if state == "wrong" {
				token = "test-only"
			}
			revoked := state == "revoked"
			registry := NewTunnelRegistry()
			resolver := func(context.Context, string) (string, error) {
				if revoked || token != "example" {
					return "", errors.New("invalid agent token")
				}
				return "example-host", nil
			}
			server := NewTunnelServerWithRegistry(registry, resolver, nil)
			server.SetSessionKeyResolver(func(context.Context, string, string) (string, error) {
				if revoked {
					return "", errors.New("revoked")
				}
				return "example-key", nil
			})
			stream := &memoryRegistrationStream{ctx: metadata.NewIncomingContext(ctx, metadata.Pairs(HeaderAgentToken, token)), token: token}
			stream.onRegister = func() {
				tunnel, ok := registry.Get("example-host").Get()
				require.True(t, ok)
				require.Equal(t, "example-key", tunnel.APIKeyID)
				require.Equal(t, AgentTokenHash(token), tunnel.TokenHash)
			}
			err := server.authStreamInterceptorInternal()(nil, stream, &grpc.StreamServerInfo{FullMethod: tunnelpb.TunnelService_Connect_FullMethodName}, func(_ any, ss grpc.ServerStream) error {
				stream.ctx = ss.Context()
				if state == "revoked-after-auth" {
					revoked = true
				}
				return server.Connect(stream)
			})
			if state == "valid" {
				require.NoError(t, err)
				require.Len(t, stream.sent, 1)
				require.True(t, stream.sent[0].GetRegisterResponse().GetAccepted())
			} else {
				require.Error(t, err)
				require.Empty(t, stream.sent)
			}
			require.False(t, registry.Get("example-host").IsPresent())
		})
	}
}

func TestEdgeWebSocketRegistrationTokenLifecycle(t *testing.T) {
	for _, state := range []string{"valid", "wrong", "revoked", "revoked-after-auth"} {
		t.Run(state, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			registry := NewTunnelRegistry()
			revoked := state == "revoked"
			token := "example"
			if state == "wrong" {
				token = "test-only"
			}
			server := NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) {
				if revoked || token != "example" {
					return "", errors.New("invalid token")
				}
				if state == "revoked-after-auth" {
					revoked = true
				}
				return "example-host", nil
			}, nil)
			server.SetSessionKeyResolver(func(_ context.Context, id, supplied string) (string, error) {
				if revoked || supplied != "example" {
					return "", errors.New("revoked")
				}
				return "example-key", nil
			})
			router := echo.New()
			router.GET("/connect", server.HandleConnect)
			httpServer := httptest.NewServer(router)
			defer httpServer.Close()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(httpServer.URL, "http")+"/connect", nil)
			require.NoError(t, err)
			defer conn.CloseNow()
			require.NoError(t, wsjson.Write(ctx, conn, &TunnelMessage{Type: MessageTypeRegister, AgentToken: token}))
			var response TunnelMessage
			err = wsjson.Read(ctx, conn, &response)
			if state == "valid" {
				require.NoError(t, err)
				require.True(t, response.Accepted)
				tunnel, ok := registry.Get("example-host").Get()
				require.True(t, ok)
				require.Equal(t, "example-key", tunnel.APIKeyID)
				require.Equal(t, AgentTokenHash(token), tunnel.TokenHash)
			} else {
				require.True(t, err != nil || !response.Accepted, "invalid registration must be rejected")
				require.False(t, registry.Get("example-host").IsPresent())
			}
			_ = conn.CloseNow()
			require.Eventually(t, func() bool { return !registry.Get("example-host").IsPresent() }, time.Second, time.Millisecond)
		})
	}
}
