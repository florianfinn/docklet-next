// Source: docklet-next/issues/292
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original caller-level agent access envelope tests over Direct and Edge.

package activity

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
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
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

// The JSON list uses the production handler method in Huma. Stream polling
// has no JSON route: assert its typed error directly at the poll caller boundary.
func TestActivityCallersAgentAccessEnvelopeDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, state := range []string{"update-required", "invalid-token"} {
			for _, caller := range []string{"list", "stream-poll"} {
				t.Run(mode+"/"+caller+"/"+state, func(t *testing.T) {
					f, db := newCallerAgent(t, mode, state)
					require.NoError(t, db.AutoMigrate(Models()...))
					h := NewHandler(NewActivityService(db, nil), EnvironmentDependencies{
						ProxyJSONRequest:               handlerutil.RemoteJSONProxy(f.service.ProxyJSONRequest),
						ProxyJSONRequestForEnvironment: f.service.ProxyJSONRequestForEnvironment,
					})
					ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
					defer cancel()
					if caller == "list" {
						_, api := humatest.New(t)
						huma.Get(api, "/environments/{id}/activities", h.ListActivities)
						response := api.DoCtx(ctx, http.MethodGet, "/environments/"+f.row.ID+"/activities?limit=50&order=desc")
						requireCallerEnvelope(t, state, response.Code, response.Body.String())
					} else {
						_, err := h.proxyListActivitiesForEnvironmentInternal(ctx, f.row, &ListActivitiesInput{EnvironmentID: f.row.ID, Limit: 50, Order: "desc"})
						require.Error(t, err)
						typed, ok := err.(huma.StatusError)
						require.True(t, ok, "stream poll keeps the translated HTTP status: %v", err)
						body, marshalErr := json.Marshal(err)
						require.NoError(t, marshalErr)
						requireCallerEnvelope(t, state, typed.GetStatus(), string(body))
					}
					f.requireCalls(t, state, http.MethodGet, "/api/environments/0/activities?limit=50&order=desc&start=0")
				})
			}
		}
	}
}
