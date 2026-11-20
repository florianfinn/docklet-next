// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

// realResolverTunnel runs the production command sender, websocket receiver,
// command registry, local HTTP dispatcher and response collector in both peers.
func realResolverTunnel(t *testing.T, handler http.Handler, environmentID string) *edge.AgentTunnel {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	ready := make(chan *edge.AgentTunnel, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		conn := edge.NewTunnelConn(socket)
		defer conn.Close()
		registration, err := conn.Receive()
		if err != nil {
			return
		}
		if err = conn.Send(&edge.TunnelMessage{Type: edge.MessageTypeRegisterResponse, Accepted: true, SessionID: "example-session"}); err != nil {
			return
		}
		tunnel := edge.NewAgentTunnelWithConn(environmentID, conn)
		tunnel.Capabilities = registration.Capabilities
		tunnel.SessionID = "example-session"
		tunnel.AgentInstance = registration.AgentInstance
		ready <- tunnel
		for {
			message, err := conn.Receive()
			if err != nil {
				return
			}
			if pending, ok := tunnel.Pending.Load(message.ID); ok {
				select {
				case pending.(*edge.PendingRequest).ResponseCh <- message:
				case <-ctx.Done():
					return
				}
			}
		}
	}))
	client := edge.NewTunnelClient(&edge.Config{EdgeAgent: true, EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: server.URL, AgentToken: "example"}, handler)
	done := make(chan struct{})
	go func() { defer close(done); client.StartWithErrorChan(ctx, nil) }()
	t.Cleanup(func() {
		cancel()
		server.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("tunnel did not stop")
		}
	})
	select {
	case tunnel := <-ready:
		return tunnel
	case <-time.After(5 * time.Second):
		t.Fatal("tunnel did not register")
		return nil
	}
}
func TestR11N3ResolverDirectEdgeRealSenderReceiver(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, outcome := range []string{"applied", "rolled-back", "recovery-required", "refused"} {
			t.Run(mode+"/"+outcome, func(t *testing.T) {
				x, f, q := resolverFixture(t)
				f.fail = outcome == "rolled-back" || outcome == "recovery-required"
				f.restoreFail = outcome == "recovery-required"
				if outcome == "refused" {
					f.prepareErr = dto.Fail(dto.EnvKeyExists, 422)
				}
				m := &Module{Agent: x.a, Settings: x.settings, Modules: x.mods, Hub: &Hub{DB: x.db.DB}}
				router := contractRouter(m, true)
				handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/environments/0/system/health" {
						w.WriteHeader(200)
						return
					}
					if r.URL.Path == "/api/app-version" {
						_ = json.NewEncoder(w).Encode(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
						return
					}
					router.ServeHTTP(w, r)
				})
				server := httptest.NewServer(handler)
				defer server.Close()
				endpoint := server.URL
				token := "example"
				key := "example-key"
				if mode == "edge" {
					endpoint = "edge://example"
					tunnel := realResolverTunnel(t, handler, q.Instance.EnvironmentID)
					tunnel.TokenHash = edge.AgentTokenHash(token)
					tunnel.APIKeyID = key
					edge.GetRegistry().Register(q.Instance.EnvironmentID, tunnel)
					defer edge.GetRegistry().Unregister(q.Instance.EnvironmentID)
				}
				require.NoError(t, x.db.Exec("INSERT INTO api_keys(id,name,key_hash,key_prefix,kind) VALUES(?,?,?,?,?)", key, "Example key", "example", "example", "environment").Error)
				require.NoError(t, x.db.Create(&environment.Environment{ID: q.Instance.EnvironmentID, ApiUrl: endpoint, Name: "Example", Enabled: true, IsEdge: mode == "edge", Status: "pending", AccessToken: &token, ApiKeyID: &key}).Error)
				hosts := environment.NewEnvironmentService(x.db, server.Client(), nil, nil, x.settings, nil)
				_, compatErr := hosts.CheckAgentCompatibility(t.Context(), q.Instance.EnvironmentID)
				require.NoError(t, compatErr)
				transport := &HostTransport{Hosts: hosts}
				result, err := transport.CreateResolver(t.Context(), q)

				require.NoError(t, err)
				require.Equal(t, dto.State(outcome), result.State)
				require.Equal(t, dto.ResolverOperation, result.Kind)
				if outcome == "recovery-required" {
					require.Equal(t, dto.RestoreFailed, result.Reason)
					require.NotNil(t, result.Current)
				}
				raw, _ := json.Marshal(result)
				require.NotContains(t, string(raw), "submitted-sentinel")
				query := dto.Query{ID: q.ID, Instance: q.Instance, Generation: q.Generation, Registration: q.Registration}
				queried, err := transport.Query(t.Context(), query)
				require.NoError(t, err)
				require.Equal(t, result.State, queried.State)
				require.Equal(t, result.Reason, queried.Reason)
			})
		}
	}
}
