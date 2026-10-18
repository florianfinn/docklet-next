// Source: docklet-next/issues/108
// Revision: dd403606fbbf763798d43f4ee854adcb105e01b1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original activity list source-completeness contract and regression evidence.

package activity

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

func TestActivityListDirectOutageAndRecovery(t *testing.T) {
	for _, mode := range []string{"refusal", "timeout"} {
		for _, populated := range []bool{false, true} {
			t.Run(mode+"/"+map[bool]string{true: "nonempty", false: "empty"}[populated], func(t *testing.T) {
				var activityCalls, dials atomic.Int32
				var offline atomic.Bool
				offline.Store(mode == "timeout")
				server := serveActivityAgent(t, func(w http.ResponseWriter, r *http.Request) {
					activityCalls.Add(1)
					if offline.Load() {
						<-r.Context().Done()
						return
					}
					writeActivityAgentPage(t, w, []activity.Activity{}, 0)
				})
				transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
					dials.Add(1)
					return (&net.Dialer{}).DialContext(ctx, network, address)
				}}
				t.Cleanup(transport.CloseIdleConnections)
				client := &http.Client{Transport: transport, Timeout: time.Second}
				endpoint := server.URL
				if mode == "refusal" {
					// Reserve then close a real listener: the actual native dial must refuse.
					listener, err := net.Listen("tcp", "127.0.0.1:0")
					require.NoError(t, err)
					endpoint = "http://" + listener.Addr().String()
					require.NoError(t, listener.Close())
				}
				f := newActivityListFixture(t, false, endpoint, client)
				want := []activity.Activity{}
				if populated {
					want = append(want, f.seed(t, activityListHost))
				}
				page := assertActivityListPage(t, f.request(t, activityListHost, ""), "agent.unavailable", want)
				require.Equal(t, int64(len(want)), page.Pagination.TotalItems)
				require.Positive(t, dials.Load(), "must reach real native dial")
				if mode == "timeout" {
					require.Equal(t, int32(1), activityCalls.Load(), "must reach and block the activity HTTP handler")
				}
				offline.Store(false)
				require.NoError(t, f.service.db.Model(&environment.Environment{}).Where("id = ?", activityListHost).Update("api_url", server.URL).Error)
				assertActivityListPage(t, f.request(t, activityListHost, ""), "", want)
				require.Equal(t, map[string]int32{"refusal": 1, "timeout": 2}[mode], activityCalls.Load())
			})
		}
	}
}

func TestActivityListEnabledEdgeOutageAndRecovery(t *testing.T) {
	for _, mode := range []string{"missing", "closed"} {
		for _, populated := range []bool{false, true} {
			t.Run(mode+"/"+map[bool]string{true: "nonempty", false: "empty"}[populated], func(t *testing.T) {
				f := newActivityListFixture(t, true, "edge://example", nil)
				want := []activity.Activity{}
				if populated {
					want = append(want, f.seed(t, activityListHost))
				}
				edge.GetRegistry().Unregister(activityListHost)
				if mode == "closed" {
					tunnel, _ := connectActivityListEdge(t, func(w http.ResponseWriter, r *http.Request) {
						t.Error("closed tunnel must not forward")
						w.WriteHeader(500)
					})
					require.NoError(t, tunnel.CloseWithReason("example disconnection"))
					require.True(t, tunnel.Conn.IsClosed())
				}
				var registered environment.Environment
				require.NoError(t, f.service.db.First(&registered, "id = ?", activityListHost).Error)
				require.True(t, registered.Enabled)
				assertActivityListPage(t, f.request(t, activityListHost, ""), "agent.unavailable", want)
				require.Equal(t, edge.TunnelStatusRequired, edge.GetDemandRegistry().DesiredStatus(activityListHost, false, time.Now()), "real production acquire path must request the tunnel")
				_, calls := connectActivityListEdge(t, func(w http.ResponseWriter, r *http.Request) { writeActivityAgentPage(t, w, []activity.Activity{}, 0) })
				assertActivityListPage(t, f.request(t, activityListHost, ""), "", want)
				require.Equal(t, int32(1), calls.Load(), "recovery must traverse the live WebSocket tunnel")
			})
		}
	}
}

func TestActivityListHealthyEdgeAndErrors(t *testing.T) {
	for _, status := range []int{200, 401, 403, 500} {
		for _, populated := range []bool{false, true} {
			t.Run(http.StatusText(status)+"/"+map[bool]string{true: "nonempty", false: "empty"}[populated], func(t *testing.T) {
				f := newActivityListFixture(t, true, "edge://example", nil)
				_, calls := connectActivityListEdge(t, func(w http.ResponseWriter, r *http.Request) {
					if status != 200 {
						http.Error(w, "example upstream failure", status)
						return
					}
					rows := []activity.Activity{}
					if populated {
						rows = append(rows, remoteActivityListRow())
					}
					writeActivityAgentPage(t, w, rows, int64(len(rows)))
				})
				want := []activity.Activity{}
				if populated {
					want = append(want, f.seed(t, activityListHost))
				}
				rec := f.request(t, activityListHost, "")
				if status == 200 {
					if populated {
						row := remoteActivityListRow()
						applyActivitySourceInternal(&row, activityListHost, "Example host")
						want = append(want, row)
					}
					assertActivityListPage(t, rec, "", want)
				} else {
					expected := status
					if status == 401 {
						expected = 502
					}
					require.Equal(t, expected, rec.Code, rec.Body.String())
					require.Empty(t, rec.Header().Get(activityListReason))
				}
				require.Equal(t, int32(1), calls.Load())
			})
		}
	}
}

// connectActivityListEdge exchanges real WebSocket command frames with the production
// proxy/command client. Only the remote agent's HTTP application is a controlled fixture.
func connectActivityListEdge(t *testing.T, activities http.HandlerFunc) (*edge.AgentTunnel, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept WebSocket: %v", err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var message edge.TunnelMessage
			if err := json.Unmarshal(data, &message); err != nil {
				t.Errorf("decode tunnel command: %v", err)
				return
			}
			if message.Type != edge.MessageTypeCommandRequest {
				t.Errorf("unexpected frame %s", message.Type)
				return
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(message.Method, message.Path, nil)
			if req.URL.Path == "/api/app-version" {
				_ = json.NewEncoder(rec).Encode(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
			} else if req.URL.Path == "/api/environments/0/activities" {
				if message.Headers["X-Arcane-Agent-Token"] != "example-agent-token" {
					t.Error("missing target credential")
				}
				calls.Add(1)
				activities(rec, req)
			} else {
				t.Errorf("unexpected Edge path %s", req.URL.Path)
				rec.WriteHeader(404)
			}
			response, _ := json.Marshal(edge.TunnelMessage{ID: message.ID, Type: edge.MessageTypeCommandComplete, Status: rec.Code, Body: rec.Body.Bytes()})
			if err := conn.Write(r.Context(), websocket.MessageText, response); err != nil {
				return
			}
		}
	}))
	conn, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	tunnel := edge.NewAgentTunnelWithConn(activityListHost, edge.NewTunnelConn(conn))
	tunnel.TokenHash = edge.AgentTokenHash("example-agent-token")
	edge.GetRegistry().Register(activityListHost, tunnel)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			message, err := tunnel.Conn.Receive()
			if err != nil {
				return
			}
			if pending, ok := tunnel.Pending.Load(message.ID); ok {
				pending.(*edge.PendingRequest).ResponseCh <- message
			}
		}
	}()
	t.Cleanup(func() { _ = tunnel.CloseWithReason(""); server.Close(); <-done })
	return tunnel, calls
}
