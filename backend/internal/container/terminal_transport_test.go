// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original shell end-to-end Direct and Edge transport evidence.

package container

import (
	"context"
	"encoding/json/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/containershell"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

// shellTransportHub runs the actual hub environment middleware, the Direct
// proxy or registered Edge server/client, and the supplied agent HTTP handler.
func shellTransportHub(t *testing.T, transport string, agent http.Handler, sessions *containershell.Registry, access middleware.EnvironmentProxySecurity) *httptest.Server {
	t.Helper()
	agentHTTP := httptest.NewServer(agent)
	t.Cleanup(agentHTTP.Close)
	registry := edge.NewTunnelRegistry()
	endpoint := agentHTTP.URL
	if transport == "edge" {
		connected := make(chan struct{})
		var once sync.Once
		server := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return "example-host", nil }, func(_ context.Context, _ string, online bool) {
			if online {
				once.Do(func() { close(connected) })
			}
		})
		router := echo.New()
		router.GET("/api/tunnel/connect", server.HandleConnect)
		manager := httptest.NewServer(router)
		t.Cleanup(manager.Close)
		address, err := url.Parse(agentHTTP.URL)
		require.NoError(t, err)
		agentClient := edge.NewTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: "test-only", Listen: address.Hostname(), Port: address.Port()}, agent)
		life, cancel := context.WithCancel(t.Context())
		joined := make(chan struct{})
		go func() { defer close(joined); agentClient.StartWithErrorChan(life, nil) }()
		t.Cleanup(func() { cancel(); registry.Unregister("example-host"); <-joined })
		select {
		case <-connected:
		case <-time.After(5 * time.Second):
			t.Fatal("Edge registration missing")
		}
		endpoint = "edge://example-host"
	}
	hub := echo.New()
	access.ShellSessions = sessions
	hub.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) {
		return endpoint, new("test-only"), true, nil
	}, func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
		return authz.SudoPermissionSet(), &user.Actor{ID: "example-user", Username: "operator"}, true
	}, authz.NewPermissionMatcher(), registry, func(r *http.Request) bool { return r.Header.Get("Origin") == "https://hub.example.com" }, access))
	hub.GET("/api/environments/:id/ws/containers/:containerId/terminal", func(c *echo.Context) error { return c.NoContent(418) })
	server := httptest.NewServer(hub)
	t.Cleanup(server.Close)
	return server
}
func dialShellHub(t *testing.T, hub *httptest.Server) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.Dial(t.Context(), strings.Replace(hub.URL, "http", "ws", 1)+"/api/environments/example-host/ws/containers/example/terminal?shell=auto", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://hub.example.com"}}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}
func dockerTerminalAgent(t *testing.T, f *shellDocker) http.Handler {
	t.Helper()
	f.running.Store(true)
	f.commands = make(chan []string, 4)
	f.resizes = make(chan string, 4)
	f.tick = make(chan time.Time, 1)
	server := httptest.NewServer(f)
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost(strings.Replace(server.URL, "http://", "tcp://", 1)), client.WithAPIVersion("1.53"), client.WithHTTPClient(server.Client()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	svc := &ContainerService{dockerService: (&docker.DockerClientService{}).WithClient(cli)}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Arcane-Agent-Token") != "test-only" && r.Header.Get("X-API-Key") != "test-only" {
			w.WriteHeader(403)
			return
		}
		query, err := containershell.ParseQuery(r.URL.Query())
		if err != nil {
			w.WriteHeader(400)
			return
		}
		conn, err := wshub.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		if f.agentSockets != nil {
			f.agentSockets <- conn
		}
		ctx := sessionlife.WithCadence(r.Context(), func(time.Duration, bool) sessionlife.Cadence { return sessionlife.Cadence{C: f.tick, Stop: func() {}} })
		svc.ServeTerminal(ctx, conn, "example", query)
	})
}
func TestShellRealDirectAndEdgeFramesAndEarlyFailures(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		for _, scenario := range []string{"EOF", "create", "protocol", "idle", "stop"} {
			t.Run(transport+"/"+scenario, func(t *testing.T) {
				f := &shellDocker{}
				if scenario == "stop" {
					f.attached = make(chan net.Conn, 1)
				}
				if scenario == "create" {
					f.failure = "create"
				}
				hub := shellTransportHub(t, transport, dockerTerminalAgent(t, f), containershell.New(nil), middleware.EnvironmentProxySecurity{})
				conn := dialShellHub(t, hub)
				want := ""
				if scenario != "create" {
					kind, data := readShell(t, conn)
					require.Equal(t, websocket.MessageBinary, kind)
					require.Equal(t, "ready", string(data))
					require.NoError(t, conn.Write(t.Context(), websocket.MessageBinary, []byte("binary-stdin")))
					kind, data = readShell(t, conn)
					require.Equal(t, websocket.MessageBinary, kind)
					require.Equal(t, "binary-stdin", string(data))
					require.NoError(t, conn.Write(t.Context(), websocket.MessageText, []byte(`{"type":"resize","cols":90,"rows":30}`)))
					require.NoError(t, conn.Write(t.Context(), websocket.MessageBinary, []byte("after-resize")))
					_, data = readShell(t, conn)
					require.Equal(t, "after-resize", string(data))
					select {
					case resize := <-f.resizes:
						require.Equal(t, "h=30&w=90", resize)
					default:
						t.Fatal("resize frame lost or coerced")
					}
					if scenario == "stop" {
						f.running.Store(false)
						require.NoError(t, (<-f.attached).Close())
						want = ""
					} else if scenario == "protocol" {
						require.NoError(t, conn.Write(t.Context(), websocket.MessageText, []byte(`{"type":"bad-control"}`)))
						want = "container.shell-protocol"
					} else {
						// An idle shell remains interactive; no timeout or policy close is introduced.
						require.NoError(t, conn.Write(t.Context(), websocket.MessageBinary, []byte("exit\n")))
						want = ""
					}
				}
				exit := readShellExit(t, conn)
				require.Equal(t, want, exit.Reason)
				if want == "" && scenario != "create" {
					require.NotNil(t, exit.Code)
					require.Equal(t, 23, *exit.Code)
				}
				_, _, err := conn.Read(t.Context())
				code := websocket.StatusNormalClosure
				if want != "" {
					code = websocket.StatusPolicyViolation
				}
				require.Equal(t, code, websocket.CloseStatus(err))
				var closeError websocket.CloseError
				require.ErrorAs(t, err, &closeError)
				require.Equal(t, want, closeError.Reason)
			})
		}
	}
}

func TestShellExitProtocolShape(t *testing.T) {
	data, err := json.Marshal(dto.ShellExit{Type: "exit", Reason: "container.shell-unavailable"})
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"exit","code":null,"reason":"container.shell-unavailable"}`, string(data))
}
