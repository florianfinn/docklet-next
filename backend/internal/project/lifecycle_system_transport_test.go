// Source: docklet-next/issues/409
// Revision: e683d402f358e87b8eef5b81a71b530fd70cc0fe
// Retrieved: 2026-11-10
// License: Apache-2.0
// Deviations: Original system-protection refusal through the real runtime sender and receiver over Direct and Edge.

package project

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/contract"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

func systemProtectionTunnel(t *testing.T, handler http.Handler, token string) *edge.AgentTunnel {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	ready := make(chan *edge.AgentTunnel, 1)
	manager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		conn := edge.NewTunnelConn(ws)
		register, err := conn.Receive()
		if err != nil {
			return
		}
		if register.Type != edge.MessageTypeRegister {
			return
		}
		tunnel := edge.NewAgentTunnelWithConn("remote", conn)
		tunnel.SessionID = "test-session"
		tunnel.AgentInstance = register.AgentInstance
		if err := conn.Send(&edge.TunnelMessage{Type: edge.MessageTypeRegisterResponse, Accepted: true, SessionID: tunnel.SessionID}); err != nil {
			return
		}
		ready <- tunnel
		for {
			msg, err := conn.Receive()
			if err != nil {
				return
			}
			if pending, ok := tunnel.Pending.Load(msg.ID); ok {
				select {
				case pending.(*edge.PendingRequest).ResponseCh <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}))
	done := make(chan struct{})
	go func() {
		defer close(done)
		edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: token}, handler).StartWithErrorChan(ctx, nil)
	}()
	t.Cleanup(func() {
		cancel()
		manager.CloseClientConnections()
		manager.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("agent did not stop")
		}
	})
	select {
	case tunnel := <-ready:
		return tunnel
	case <-time.After(10 * time.Second):
		t.Fatal("agent registration failed")
		return nil
	}
}

func TestSystemProtectionPreflightCodeDirectEdge(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "Direct", true: "Edge"}[isEdge], func(t *testing.T) {
			f := newLifecycleFixture(t)
			current := f.establishBaseline()
			f.service.imageService.ImageGuard().SystemReferences = func() []string { return []string{"registry.example.com/app:1.0.0"} }
			adapter := activity.NewRuntimeServiceAdapter(f.service, nil)
			receiver, err := activity.NewRuntimeReceiver(f.runtime, adapter)
			require.NoError(t, err)
			token := rand.Text()
			handler, err := activity.NewRuntimeReceiverHTTP(receiver, &auth.AuthService{}, &config.Config{AgentMode: true, AgentToken: token}, time.Minute)
			require.NoError(t, err)
			server := httptest.NewServer(handler)
			t.Cleanup(server.Close)
			tunnels := edge.NewTunnelRegistry()
			endpoint := server.URL
			if isEdge {
				tunnels.Register("remote", systemProtectionTunnel(t, handler, token))
				endpoint = "edge://example"
			}
			sender, err := activity.NewRuntimeSender(activity.RuntimeRemoteTarget{EnvironmentID: "remote", Endpoint: endpoint, Token: token, Edge: isEdge, AgentContractVersion: contract.AgentHubVersion}, func(ctx context.Context, _ string, _ *string, _ string, _ string) (context.Context, context.CancelFunc, error) {
				return ctx, func() {}, nil
			}, server.Client(), tunnels)
			require.NoError(t, err)
			_, err = sender.Resolve(t.Context(), activityDTO.RuntimeTargetRequest{Registration: activityDTO.RuntimeExecutorIdentity{ManagerID: f.binding.ManagerID, RegistrationID: f.binding.RegistrationID, ExecutorGeneration: f.binding.ExecutorGeneration}, Target: activity.RuntimeTarget{Kind: "stack", ID: f.project.ID}, Lifecycle: &dto.LifecycleRequest{ExpectedRevision: current.SavedRevision}})
			var problem *imageDTO.OperationError
			require.ErrorAs(t, err, &problem)
			require.Equal(t, "image.system-protected", problem.Problem.Code)
			require.Equal(t, 409, problem.Status)
			f.mu.Lock()
			defer f.mu.Unlock()
			require.Zero(t, f.pulls)
			require.Zero(t, f.tagWrites)
			require.Zero(t, f.deploys)
			require.Equal(t, lifecycleContainerID, f.workloads["app"].ID)
			require.True(t, f.workloads["app"].Running)
		})
	}
}
