// Source: docklet-next/issues/81
// Revision: e0a44b221176de20524bf7829741b1246461c927
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original Edge recovery regression tests.

package environment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

// The blocked stack is a barrier for lock interleavings, without production hooks.
func agentStackContains(functions ...string) bool {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	for _, stack := range strings.Split(string(buf[:n]), "\n\n") {
		matches := true
		for _, function := range functions {
			matches = matches && strings.Contains(stack, function)
		}
		if matches {
			return true
		}
	}
	return false
}

func runAgentConnectCheck(ctx context.Context, f *agentFixture, done chan<- error) {
	_, err := f.service.CheckAgentCompatibility(ctx, f.id)
	done <- err
}

func recoveryRequest(ctx context.Context, f *agentFixture, transport, route string) error {
	method, path := http.MethodGet, "/api/environments/0/containers"
	if route == "health" {
		method, path = http.MethodHead, "/api/environments/0/system/health"
	}
	if transport == "service" {
		if route == "health" {
			method, path = http.MethodGet, "/api/health"
		}
		_, err := f.service.ExecuteRemoteRequest(ctx, f.id, method, path, nil)
		return err
	}
	router := echo.New()
	resolver := func(context.Context, string) (string, *string, bool, error) {
		return "edge://example", &f.token, true, nil
	}
	router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver, nil, nil, edge.GetRegistry(), nil, middleware.EnvironmentProxySecurity{AgentAccess: f.service.AgentAccess}))
	router.Add(method, strings.Replace(path, "/0/", "/:id/", 1), func(c *echo.Context) error { return c.NoContent(500) })
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, strings.Replace(path, "/0/", "/"+f.id+"/", 1), nil).WithContext(ctx))
	if rec.Code != http.StatusOK {
		return &recoveryHTTPError{status: rec.Code, body: rec.Body.String()}
	}
	return nil
}

type recoveryHTTPError struct {
	status int
	body   string
}

func (e *recoveryHTTPError) Error() string { return http.StatusText(e.status) + ": " + e.body }

func TestEdgeRequestRecoversConcreteTunnel(t *testing.T) {
	for _, transport := range []string{"service", "middleware"} {
		for _, route := range []string{"ordinary", "health"} {
			for _, callback := range []string{"arrival", "connect-callback"} {
				t.Run(transport+"/"+route+"/"+callback, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
					defer cancel()
					f := newAgentFixture(t, "edge", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
					var workers sync.WaitGroup
					defer func() { cancel(); workers.Wait() }()
					edge.GetRegistry().Unregister(f.id)
					f.conn = &agentMemoryConn{send: f.conn.send}
					f.tunnel = edge.NewAgentTunnelWithConn(f.id, f.conn)
					f.tunnel.APIKeyID, f.tunnel.TokenHash = "example", edge.AgentTokenHash(f.token)
					// Keep responses pending until the callback has passed session lookup.
					respond := make(chan struct{})
					send := f.conn.send
					f.conn.send = func(m *edge.TunnelMessage) {
						select {
						case <-respond:
							send(m)
						case <-ctx.Done():
						}
					}
					done := make(chan error, 1)
					workers.Add(1)
					go func() {
						defer workers.Done()
						done <- recoveryRequest(ctx, f, transport, route)
					}()
					require.Eventually(t, func() bool {
						return edge.GetDemandRegistry().DesiredStatus(f.id, false, time.Now()) == edge.TunnelStatusRequired
					}, time.Second, time.Millisecond, "request must be waiting for an on-demand tunnel")
					edge.GetRegistry().Register(f.id, f.tunnel)
					connected := make(chan error, 1)
					if callback == "connect-callback" {
						workers.Add(1)
						go func() {
							defer workers.Done()
							runAgentConnectCheck(ctx, f, connected)
						}()
						require.Eventually(t, func() bool {
							return agentStackContains("runAgentConnectCheck", "checkAgentCompatibility")
						}, time.Second, time.Millisecond, "connect check must pass session lookup before responses finish")
					}
					close(respond)
					select {
					case err := <-done:
						require.NoError(t, err, "the first request must recover within its credential session")
					case <-ctx.Done():
						t.Fatal("request did not recover")
					}
					if callback == "connect-callback" {
						select {
						case err := <-connected:
							require.NoError(t, err)
						case <-ctx.Done():
							t.Fatal("connect compatibility check did not complete")
						}
					}
					if route == "ordinary" {
						require.Equal(t, 1, f.count("GET /api/environments/0/containers"))
						require.Positive(t, f.count("GET /api/app-version"), "recovered tunnel needs its own verdict")
					} else {
						health := "HEAD /api/environments/0/system/health"
						if transport == "service" {
							health = "GET /api/health"
						}
						require.Equal(t, 1, f.count(health))
					}
					require.False(t, f.conn.IsClosed())
				})
			}
		}
	}
}

func TestEdgeOldTunnelObserverDoesNotCancelCurrentSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	f := newAgentFixture(t, "edge", version.Info{})
	env, err := f.service.GetEnvironmentByID(ctx, f.id)
	require.NoError(t, err)
	initial := f.service.agentSessionFor(ctx, env)
	defer initial.cancel()

	f.service.agentRuntime.mu.Lock()
	locked := true
	defer func() {
		if locked {
			f.service.agentRuntime.mu.Unlock()
		}
	}()
	observed := make(chan *agentSession, 1)
	go func() { observed <- f.service.agentSessionFor(ctx, env) }()
	require.Eventually(t, func() bool {
		return agentStackContains("TestEdgeOldTunnelObserverDoesNotCancelCurrentSession.func", "agentSessionFor")
	}, time.Second, time.Millisecond, "old registry observer must be blocked on the runtime lock")

	replacement := edge.NewAgentTunnelWithConn(f.id, &agentMemoryConn{})
	replacement.APIKeyID, replacement.TokenHash = "example", edge.AgentTokenHash(f.token)
	edge.GetRegistry().Register(f.id, replacement)
	// Install the session a reconnect observer created before the older reader got the lock.
	newOwner := &EnvironmentService{}
	current := newOwner.agentSessionFor(ctx, env)
	defer current.cancel()
	f.service.agentRuntime.sessions[f.id] = current
	streamCtx, finish := context.WithCancel(ctx)
	defer finish()
	stop := context.AfterFunc(current.ctx, finish)
	defer stop()
	f.service.agentRuntime.mu.Unlock()
	locked = false

	select {
	case stale := <-observed:
		require.NoError(t, current.ctx.Err(), "old tunnel identity must not revoke the current session")
		require.Same(t, current, stale, "an old tunnel snapshot must reuse the credential session")
	case <-ctx.Done():
		t.Fatal("old registry observer remained blocked")
	}
	require.NoError(t, current.ctx.Err(), "old tunnel identity must not revoke the current session")
	require.NoError(t, streamCtx.Err(), "the current stream must stay open")
	require.False(t, replacement.Conn.IsClosed())
}
