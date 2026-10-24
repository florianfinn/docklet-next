// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package environment

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type agentRoundTrip func(*http.Request) (*http.Response, error)

func (f agentRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type agentMemoryConn struct {
	mu     sync.Mutex
	closed bool
	send   func(*edge.TunnelMessage)
}

func (c *agentMemoryConn) Send(m *edge.TunnelMessage) error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return edge.ErrTunnelConnectionClosed
	}
	c.send(m)
	return nil
}
func (c *agentMemoryConn) Receive() (*edge.TunnelMessage, error) { return nil, io.EOF }
func (c *agentMemoryConn) IsExpectedReceiveError(error) bool     { return true }
func (c *agentMemoryConn) Close() error                          { c.mu.Lock(); c.closed = true; c.mu.Unlock(); return nil }
func (c *agentMemoryConn) IsClosed() bool                        { c.mu.Lock(); defer c.mu.Unlock(); return c.closed }
func (c *agentMemoryConn) Transport() string                     { return edge.EdgeTransportWebSocket }

type agentFixture struct {
	service *EnvironmentService
	tunnel  *edge.AgentTunnel
	conn    *agentMemoryConn
	token   string
	id      string
	mu      sync.Mutex
	calls   []string
	info    version.Info
}

func newAgentFixture(t *testing.T, mode string, info version.Info) *agentFixture {
	t.Helper()
	f := &agentFixture{token: "example", id: "example-host", info: info}
	db := setupEnvironmentServiceTestDB(t)
	httpClient := &http.Client{Transport: agentRoundTrip(func(r *http.Request) (*http.Response, error) {
		body := f.respond(r.Method, r.URL.Path, r.Header)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{"Content-Type": {"application/json"}}, Request: r}, nil
	})}
	f.service = NewEnvironmentService(db, httpClient, nil, nil, nil, nil)
	t.Cleanup(func() {
		f.service.agentRuntime.mu.Lock()
		sessions := make([]*agentSession, 0, len(f.service.agentRuntime.sessions))
		for _, session := range f.service.agentRuntime.sessions {
			session.cancel()
			sessions = append(sessions, session)
		}
		f.service.agentRuntime.mu.Unlock()
		for _, session := range sessions {
			session.checkMu.Lock()
			check := session.check
			session.checkMu.Unlock()
			if check != nil {
				select {
				case <-check.done:
				case <-time.After(11 * time.Second):
					t.Error("shared agent check outlived its attempt cap")
				}
			}
		}
	})
	apiURL := "http://agent.example.com"
	if mode == "edge" {
		apiURL = "edge://example"
	}
	require.NoError(t, db.Create(&Environment{ID: f.id, ApiUrl: apiURL, Name: "Example", Enabled: true, IsEdge: mode == "edge", Status: "pending", AccessToken: &f.token, ApiKeyID: new("example")}).Error)
	if mode == "edge" {
		f.conn = &agentMemoryConn{}
		f.tunnel = edge.NewAgentTunnelWithConn(f.id, f.conn)
		f.tunnel.APIKeyID = "example"
		f.tunnel.TokenHash = edge.AgentTokenHash(f.token)
		f.conn.send = func(m *edge.TunnelMessage) {
			if m.Type != edge.MessageTypeCommandRequest {
				return
			}
			headers := http.Header{}
			for k, v := range m.Headers {
				headers.Set(k, v)
			}
			body := f.respond(m.Method, m.Path, headers)
			pending, ok := f.tunnel.Pending.Load(m.ID)
			require.True(t, ok)
			pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeResponse, Status: 200, Body: body}
		}
		edge.GetRegistry().Register(f.id, f.tunnel)
		t.Cleanup(func() { edge.GetRegistry().Unregister(f.id); edge.ForgetPollState(f.id) })
	}
	return f
}
func (f *agentFixture) respond(method, path string, headers http.Header) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, method+" "+path)
	if path == "/api/app-version" {
		body, _ := json.Marshal(f.info)
		return body
	}
	return []byte(`{"success":true}`)
}
func (f *agentFixture) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, p := range f.calls {
		if p == path {
			count++
		}
	}
	return count
}

func TestHostAgentVersionMatrix(t *testing.T) {
	previousVersion := config.Version
	config.Version = "2.0.0"
	defer func() { config.Version = previousVersion }()
	for _, mode := range []string{"direct", "edge"} {
		for _, state := range []string{"ok", "below-minimum", "fingerprint-mismatch"} {
			t.Run(mode+"/"+state, func(t *testing.T) {
				info := version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}
				if state == "below-minimum" {
					info.AgentContractVersion = contract.MinimumAgentVersion - 1
				}
				if state == "fingerprint-mismatch" {
					info.AgentContractFingerprint = "different"
				}
				f := newAgentFixture(t, mode, info)
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				expected := "online"
				if state != "ok" {
					expected = "update-required"
				}
				status, err := f.service.TestConnection(ctx, f.id, nil)
				require.NoError(t, err)
				require.Equal(t, expected, status)
				env, err := f.service.GetEnvironmentByID(ctx, f.id)
				require.NoError(t, err)
				require.Equal(t, expected, env.Status)
				require.NotNil(t, env.LastSeen)
				for _, route := range []struct{ method, path string }{
					{"GET", "/api/health"}, {"GET", "/api/app-version"}, {"HEAD", "/api/environments/0/system/health"}, {"POST", "/api/environments/0/system/upgrade"}, {"GET", "/api/environments/0/containers"}, {"POST", "/api/container-registries/sync"}, {"PUT", "/api/environments/0/settings"},
				} {
					var body []byte
					if route.method == "POST" && strings.HasSuffix(route.path, "/system/upgrade") {
						body = []byte(`{"targetVersion":"1.0.0"}`)
					}
					_, err := f.service.ExecuteRemoteRequest(ctx, f.id, route.method, route.path, body)
					allowed := state == "ok" || contract.AgentRecoveryRoute(route.method, route.path)
					if allowed {
						require.NoError(t, err)
						require.Positive(t, f.count(route.method+" "+route.path))
					} else {
						require.ErrorIs(t, err, ErrAgentUpdateRequired)
						require.Zero(t, f.count(route.method+" "+route.path))
					}
				}
				router := echo.New()
				resolver := func(ctx context.Context, id string) (string, *string, bool, error) {
					e, err := f.service.GetEnvironmentByID(ctx, id)
					if err != nil {
						return "", nil, false, err
					}
					return e.ApiUrl, e.AccessToken, e.Enabled, nil
				}
				router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver, nil, nil, edge.GetRegistry(), nil, middleware.EnvironmentProxySecurity{AgentAccess: f.service.AgentAccess, HTTPClient: f.service.httpClient}))
				router.GET("/api/environments/:id/containers", func(c *echo.Context) error { return c.NoContent(500) })
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest("GET", "/api/environments/"+f.id+"/containers", http.NoBody).WithContext(ctx))
				if state == "ok" {
					require.Equal(t, 200, w.Code)
				} else {
					require.Equal(t, 409, w.Code)
				}
			})
		}
	}
}

func TestAgentRotationAndRemovalRevokeEstablishedSessions(t *testing.T) {
	for _, action := range []string{"rotate", "remove"} {
		t.Run(action, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			f := newAgentFixture(t, "edge", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
			streamCtx, finish, err := f.service.AgentAccess(ctx, f.id, &f.token, "GET", "/api/health")
			require.NoError(t, err)
			defer finish()
			edge.GetPollRuntimeRegistry().Update(f.id, time.Second, time.Now())
			edge.TouchTunnelDemand(f.id, time.Minute)
			keys := apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
			f.service.apiKeys = keys
			original, err := keys.CreateEnvironmentApiKey(ctx, f.id)
			require.NoError(t, err)
			require.NoError(t, f.service.db.Model(&Environment{}).Where("id = ?", f.id).Update("api_key_id", original.ID).Error)
			f.tunnel.APIKeyID = original.ID
			env, err := f.service.GetEnvironmentByID(ctx, f.id)
			require.NoError(t, err)
			if action == "rotate" {
				replacement, err := f.service.RegenerateEnvironmentApiKey(ctx, env, "operator", "operator")
				require.NoError(t, err)
				require.NotEqual(t, f.token, replacement)
				_, err = f.service.ResolveEdgeSessionKey(ctx, f.id, f.token)
				require.Error(t, err)

			} else {
				require.NoError(t, f.service.DeleteEnvironment(ctx, f.id, nil, nil))
			}
			require.False(t, edge.GetPollRuntimeRegistry().Get(f.id, time.Now()).IsPresent())
			require.Equal(t, edge.TunnelStatusIdle, edge.GetDemandRegistry().DesiredStatus(f.id, false, time.Now()))
			requestCtx, requestCancel := context.WithTimeout(ctx, 100*time.Millisecond)
			defer requestCancel()
			_, requestErr := f.service.ExecuteRemoteRequest(requestCtx, f.id, "GET", "/api/environments/0/containers", nil)
			require.Error(t, requestErr)
			require.Zero(t, f.count("GET /api/environments/0/containers"))
			require.Zero(t, f.count("GET /api/app-version"), "replacement token must never reach the revoked tunnel")
			select {
			case <-streamCtx.Done():
			case <-ctx.Done():
				t.Fatal("proxied stream remained open")
			}
			require.True(t, f.conn.IsClosed())
			_, err = f.service.ResolveEdgeEnvironmentByToken(ctx, f.token)
			require.Error(t, err)

		})
	}
}

func TestAgentCurrentKeyRequiredBeforeTokenInjection(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	f.tunnel.APIKeyID = "test-only"
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err := f.service.ExecuteRemoteRequest(ctx, f.id, "GET", "/api/app-version", nil)
	require.Error(t, err)
	require.True(t, f.conn.IsClosed())
	require.Zero(t, len(f.calls))
}
