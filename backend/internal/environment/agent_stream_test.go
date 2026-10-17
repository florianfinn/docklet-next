// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original in-flight transport revocation tests.

package environment

import (
	"context"
	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/labstack/echo/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

type agentBlockingBody struct{ ctx context.Context }

func (b *agentBlockingBody) Read([]byte) (int, error) { <-b.ctx.Done(); return 0, b.ctx.Err() }
func (b *agentBlockingBody) Close() error             { return nil }

func TestAgentRevocationEndsInflightStreams(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, action := range []string{"rotate", "remove"} {
			t.Run(mode+"/"+action, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				f := newAgentFixture(t, mode, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
				entered := make(chan struct{}, 1)
				if mode == "direct" {
					original := f.service.httpClient.Transport
					f.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
						if r.URL.Path == "/api/environments/0/containers" {
							entered <- struct{}{}
							return &http.Response{StatusCode: 200, Header: http.Header{}, Body: &agentBlockingBody{ctx: r.Context()}, Request: r}, nil
						}
						return original.RoundTrip(r)
					})
				} else {
					original := f.conn.send
					f.conn.send = func(m *edge.TunnelMessage) {
						if m.Type == edge.MessageTypeCommandRequest && m.Path == "/api/environments/0/containers" {
							require.Equal(t, f.token, m.Headers[edge.HeaderAgentToken])
							entered <- struct{}{}
							return
						}
						original(m)
					}
				}
				finished := make(chan error, 1)
				go func() {
					_, err := f.service.ExecuteRemoteRequest(ctx, f.id, "GET", "/api/environments/0/containers", nil)
					finished <- err
				}()
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("stream did not start")
				}
				if action == "remove" {
					require.NoError(t, f.service.DeleteEnvironment(ctx, f.id, nil, nil))
				} else {
					keys := apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
					f.service.apiKeys = keys
					env, err := f.service.GetEnvironmentByID(ctx, f.id)
					require.NoError(t, err)
					_, err = f.service.RegenerateEnvironmentApiKey(ctx, env, "operator", "operator")
					require.NoError(t, err)
				}
				select {
				case err := <-finished:
					require.Error(t, err)
				case <-ctx.Done():
					t.Fatal("revoked stream did not terminate")
				}
			})
		}
	}
}

func TestAgentRevocationEndsMiddlewareWebSocket(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, action := range []string{"rotate", "remove"} {
			t.Run(mode+"/"+action, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				f := newAgentFixture(t, mode, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
				if mode == "direct" {
					agentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						conn, err := websocket.Accept(w, r, nil)
						if err != nil {
							return
						}
						defer conn.CloseNow()
						if err := conn.Write(ctx, websocket.MessageText, []byte("ready")); err != nil {
							return
						}
						_, _, _ = conn.Read(ctx)
					}))
					defer agentServer.Close()
					require.NoError(t, f.service.db.Model(&Environment{}).Where("id = ?", f.id).Update("api_url", agentServer.URL).Error)
				} else {
					original := f.conn.send
					f.conn.send = func(m *edge.TunnelMessage) {
						if m.Type == edge.MessageTypeStreamOpen {
							pending, ok := f.tunnel.Pending.Load(m.ID)
							require.True(t, ok)
							pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeStreamData, Body: []byte("ready"), WSMessageType: int(websocket.MessageText)}
							return
						}
						original(m)
					}
				}
				resolver := func(ctx context.Context, id string) (string, *string, bool, error) {
					env, err := f.service.GetEnvironmentByID(ctx, id)
					if err != nil {
						return "", nil, false, err
					}
					return env.ApiUrl, env.AccessToken, env.Enabled, nil
				}
				router := echo.New()
				router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver, nil, nil, edge.GetRegistry(), func(*http.Request) bool { return true }, middleware.EnvironmentProxySecurity{AgentAccess: f.service.AgentAccess, HTTPClient: f.service.httpClient}))
				router.GET("/api/environments/:id/ws/system/stats", func(c *echo.Context) error { return c.NoContent(500) })
				proxyServer := httptest.NewServer(router)
				defer proxyServer.Close()
				conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(proxyServer.URL, "http")+"/api/environments/"+f.id+"/ws/system/stats", nil)
				require.NoError(t, err)
				defer conn.CloseNow()
				_, ready, err := conn.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "ready", string(ready))
				finished := make(chan error, 1)
				go func() { _, _, err := conn.Read(ctx); finished <- err }()
				if action == "remove" {
					require.NoError(t, f.service.DeleteEnvironment(ctx, f.id, nil, nil))
				} else {
					f.service.apiKeys = apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
					env, err := f.service.GetEnvironmentByID(ctx, f.id)
					require.NoError(t, err)
					_, err = f.service.RegenerateEnvironmentApiKey(ctx, env, "operator", "operator")
					require.NoError(t, err)
				}
				select {
				case err := <-finished:
					require.Error(t, err)
					require.NotErrorIs(t, err, context.DeadlineExceeded)
				case <-time.After(500 * time.Millisecond):
					t.Fatal("revoked middleware WebSocket remained open")
				}
			})
		}
	}
}
