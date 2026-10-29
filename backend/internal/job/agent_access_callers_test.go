// Source: docklet-next/issues/292
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original caller-level agent access envelope tests over Direct and Edge.

package job

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Each package keeps its fixture test-only; both transports reach the real
// environment access check and forwarding code before the caller translates it.
type callerAgentFixture struct {
	service *environment.EnvironmentService
	row     environment.Environment
	mu      sync.Mutex
	calls   []string
}

type callerAgentTransport func(*http.Request) (*http.Response, error)

func (f callerAgentTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type callerAgentConn struct{ send func(*edge.TunnelMessage) }

func (c *callerAgentConn) Send(m *edge.TunnelMessage) error    { c.send(m); return nil }
func (*callerAgentConn) Receive() (*edge.TunnelMessage, error) { return nil, io.EOF }
func (*callerAgentConn) IsExpectedReceiveError(error) bool     { return true }
func (*callerAgentConn) Close() error                          { return nil }
func (*callerAgentConn) IsClosed() bool                        { return false }
func (*callerAgentConn) Transport() string                     { return edge.EdgeTransportWebSocket }

func newCallerAgent(t *testing.T, mode, state string) (*callerAgentFixture, *database.DB) {
	t.Helper()
	raw, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, raw.AutoMigrate(&environment.Environment{}))
	pool, err := raw.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	db := &database.DB{DB: raw}
	f := &callerAgentFixture{row: environment.Environment{
		BaseModel: database.BaseModel{ID: "example-host"}, Name: "Example", Enabled: true,
		ApiUrl: "http://agent.example.com", AccessToken: new("example-token"), IsEdge: mode == "edge",
	}}
	info := version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}
	if state == "update-required" {
		info.AgentContractFingerprint = "synthetic-private.example.com"
	}
	versionBody, err := json.Marshal(info)
	require.NoError(t, err)
	reply := func(method, path string) (int, []byte) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, method+" "+path)
		if path == "/api/app-version" {
			return http.StatusOK, versionBody
		}
		if method == http.MethodHead && path == "/api/environments/0/system/health" {
			return http.StatusNoContent, nil
		}
		return http.StatusUnauthorized, []byte(`{"error":"synthetic-private.example.com"}`)
	}
	client := &http.Client{Transport: callerAgentTransport(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "direct", mode, "Edge must use the tunnel, never the HTTP transport")
		status, body := reply(r.Method, r.URL.RequestURI())
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
	})}
	if mode == "edge" {
		f.row.ApiUrl = "edge://example"
		conn := &callerAgentConn{}
		tunnel := edge.NewAgentTunnelWithConn(f.row.ID, conn)
		tunnel.TokenHash = edge.AgentTokenHash(*f.row.AccessToken)
		conn.send = func(m *edge.TunnelMessage) {
			if m.Type != edge.MessageTypeCommandRequest {
				return
			}
			status, body := reply(m.Method, m.Path)
			pending, ok := tunnel.Pending.Load(m.ID)
			require.True(t, ok, "real tunnel pending request exists before its response")
			pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeResponse, Status: status, Body: body}
		}
		edge.GetRegistry().Register(f.row.ID, tunnel)
		t.Cleanup(func() { edge.GetRegistry().Unregister(f.row.ID); edge.ForgetPollState(f.row.ID) })
	}
	require.NoError(t, db.Create(&f.row).Error)
	f.service = environment.NewEnvironmentService(db, client, nil, nil, nil, nil)
	return f, db
}

func (f *callerAgentFixture) requireCalls(t *testing.T, state, method, path string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Contains(t, f.calls, "GET /api/app-version", "caller reaches compatibility probe through its transport")
	if state == "update-required" {
		require.NotContains(t, f.calls, method+" "+path, "incompatible agent blocks domain forwarding")
	} else {
		require.Contains(t, f.calls, method+" "+path, "401 comes from the actual domain request")
	}
}

func requireCallerEnvelope(t *testing.T, state string, status int, body string) {
	t.Helper()
	expected := http.StatusBadGateway
	if state == "update-required" {
		expected = http.StatusConflict
	}
	require.NotContains(t, body, "synthetic-private.example.com")
	require.Equal(t, expected, status)
	require.Equal(t, `{"success":false,"code":"agent.`+state+`"}`, strings.TrimSpace(body))
}

// Register only the production handler methods in Huma to isolate caller
// translation from permission middleware and unrelated job-service routes.
func TestJobCallersAgentAccessEnvelopeDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, state := range []string{"update-required", "invalid-token"} {
			for _, caller := range []struct{ name, method, path, body string }{
				{"get", http.MethodGet, "/job-schedules", ""},
				{"update", http.MethodPut, "/job-schedules", `{}`},
				{"restart", http.MethodPost, "/jobs/poll/restart", ""},
			} {
				t.Run(mode+"/"+caller.name+"/"+state, func(t *testing.T) {
					f, _ := newCallerAgent(t, mode, state)
					h := &JobSchedulesHandler{jobService: &JobService{}, proxyRemoteJSON: f.service.ProxyJSONRequest}
					_, api := humatest.New(t)
					huma.Get(api, "/environments/{id}/job-schedules", h.Get)
					huma.Put(api, "/environments/{id}/job-schedules", h.Update)
					huma.Post(api, "/environments/{id}/jobs/{jobId}/restart", h.RestartWorker)
					ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
					defer cancel()
					response := api.DoCtx(ctx, caller.method, "/environments/"+f.row.ID+caller.path, strings.NewReader(caller.body))
					requireCallerEnvelope(t, state, response.Code, response.Body.String())
					f.requireCalls(t, state, caller.method, "/api/environments/0"+caller.path)
				})
			}
		}
	}
}
