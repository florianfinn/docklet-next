// Source: docklet-next/issues/507
// Revision: 8380898628dbb860f193773e2e3822dacb783269
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original volume workspace path refusal through Direct and Edge.

package workspace

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/api/types/volume"
	"github.com/stretchr/testify/require"
)

// A helper symlink refusal on a remote host must reach the client as a path refusal, not as rejected credentials.
func TestVolumeWorkspacePathRefusalDirectEdge(t *testing.T) {
	for _, transport := range []string{"Direct", "Edge"} {
		t.Run(transport, func(t *testing.T) {
			docker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "workspace-volume", Driver: "local"}))
			}))
			t.Cleanup(docker.Close)
			execs := 0
			service := NewService(Dependencies{
				Docker:        dockerServiceForRefusalInternal(t, docker),
				Locks:         &utils.KeyedMutex{},
				AcquireHelper: func(context.Context, string) (string, func(), error) { return "helper-example", func() {}, nil },
				RequireACFS:   func(context.Context, string, string) error { return nil },
				Exec: func(context.Context, string, string, []string) (string, string, error) {
					execs++
					return "", "ARCANE_SYMLINK", errors.New("exit status 1")
				},
			})
			agent := echo.New()
			api := humaecho.NewWithGroup(agent, agent.Group("/api"), huma.DefaultConfig("workspace", "test"))
			api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
				request = context.WithValue(request, user.CurrentUserContextKey{}, &user.SystemUser)
				next(huma.WithContext(ctx, request))
			})
			RegisterRoutes(api, NewHandler(service, nil, t.Context()))
			server := httptest.NewServer(agent)
			t.Cleanup(server.Close)
			registry := edge.NewTunnelRegistry()
			endpoint := server.URL
			token := rand.Text()
			if transport == "Edge" {
				registry.Register("remote", refusalTunnelInternal(t, agent, token))
				endpoint = "edge://example"
			}
			hub := echo.New()
			hub.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) { return endpoint, &token, true, nil }, func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
				return authz.SudoPermissionSet(), &user.SystemUser, true
			}, nil, registry, nil, middleware.EnvironmentProxySecurity{HTTPClient: server.Client()}))
			hub.GET("/api/environments/:id/*", func(c *echo.Context) error { return c.NoContent(500) })
			request := httptest.NewRequest(http.MethodGet, "/api/environments/remote/volumes/workspace-volume/workspace/file?relativePath=link-example", nil)
			response := httptest.NewRecorder()
			hub.ServeHTTP(response, request)
			require.Equal(t, 1, execs, "the agent helper must have refused the path")
			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"code":"volume.workspace-path-refused"`)
			require.NotContains(t, response.Body.String(), "agent.invalid-token")
		})
	}
}

func dockerServiceForRefusalInternal(t *testing.T, server *httptest.Server) *docker.DockerClientService {
	return docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeWorkspaceTestDockerClientInternal(t, server))
}

func refusalTunnelInternal(t *testing.T, handler http.Handler, token string) *edge.AgentTunnel {
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
		if err != nil || register.Type != edge.MessageTypeRegister {
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
