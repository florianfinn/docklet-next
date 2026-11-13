// Source: docklet-next/issues/379
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original log-download proofs through hub middleware, real Direct/Edge and Docker client.

package container

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

// The real Docker client receives a generated body without allocating a log-sized fixture.
// Holding after the last byte distinguishes hub cancellation from producer EOF.
type downloadSource struct {
	size                                          int
	hold, broken                                  bool
	started, cancelled, closed, release           chan struct{}
	startOnce, cancelOnce, closeOnce, releaseOnce sync.Once
	reads                                         atomic.Int32
}

func newDownloadSource(t *testing.T, size int, hold bool) *downloadSource {
	s := &downloadSource{size: size, hold: hold, started: make(chan struct{}), cancelled: make(chan struct{}), closed: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(s.stop)
	return s
}

func (s *downloadSource) stop() { s.releaseOnce.Do(func() { close(s.release) }) }

type downloadReader struct {
	source    *downloadSource
	ctx       context.Context
	remaining int
}

func (r *downloadReader) Read(p []byte) (int, error) {
	s := r.source
	s.startOnce.Do(func() { close(s.started) })
	if r.remaining > 0 {
		n := min(len(p), r.remaining)
		for i := range p[:n] {
			p[i] = 'x'
		}
		r.remaining -= n
		return n, nil
	}
	if s.hold {
		select {
		case <-r.ctx.Done():
			s.cancelOnce.Do(func() { close(s.cancelled) })
			return 0, r.ctx.Err()
		case <-s.release:
		}
	}
	if s.broken {
		return 0, errors.New("synthetic Docker log read failure")
	}
	return 0, io.EOF
}
func (r *downloadReader) Close() error {
	r.source.closeOnce.Do(func() { close(r.source.closed) })
	return nil
}

type downloadRoundTripper func(*http.Request) (*http.Response, error)

func (f downloadRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func downloadAgent(t *testing.T, source *downloadSource) http.Handler {
	t.Helper()
	httpClient := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		var body io.ReadCloser
		switch {
		case strings.HasSuffix(r.URL.Path, "/json"):
			body = io.NopCloser(strings.NewReader(`{"Id":"0123456789abcdef","Config":{"Tty":true}}`))
		case strings.HasSuffix(r.URL.Path, "/logs"):
			if tail := r.URL.Query().Get("tail"); tail != "" && tail != "all" {
				return nil, errors.New("unexpected Docker log tail")
			}
			if r.URL.Query().Get("timestamps") != "1" {
				return nil, errors.New("Docker log timestamps not enabled")
			}
			source.reads.Add(1)
			body = &downloadReader{source: source, ctx: r.Context(), remaining: source.size}
		default:
			return nil, errors.New("unexpected Docker request")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body, Request: r}, nil
	})}
	cli, err := client.New(client.WithHost("tcp://docker.example.com"), client.WithAPIVersion("1.53"), client.WithHTTPClient(httpClient))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	svc := &ContainerService{dockerService: (&docker.DockerClientService{}).WithClient(cli)}
	handler := &ContainerHandler{containerService: svc}
	router := echo.New()
	api := humaecho.New(router, huma.DefaultConfig("Download test", "1.0.0"))
	huma.Register(api, huma.Operation{OperationID: "download-container-logs", Method: http.MethodGet, Path: "/api/environments/{id}/containers/{containerId}/logs/download"}, handler.DownloadContainerLogs)
	return router
}

type downloadHub struct {
	server  *httptest.Server
	sent    *atomic.Int32
	barrier func()
}

func newDownloadHub(t *testing.T, transport string, agent http.Handler, oldAgent bool) downloadHub {
	t.Helper()
	agentHTTP := httptest.NewServer(agent)
	t.Cleanup(agentHTTP.Close)
	registry := edge.NewTunnelRegistry()
	endpoint := agentHTTP.URL
	var sent atomic.Int32
	barrier := func() {}
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
		if oldAgent {
			ws, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(manager.URL, "http")+"/api/tunnel/connect", nil)
			require.NoError(t, err)
			conn := edge.NewTunnelConn(ws)
			capabilities := slices.DeleteFunc(edge.AdvertisedEdgeCommands(), func(c string) bool { return c == "container.logs.download" })
			require.NoError(t, conn.Send(&edge.TunnelMessage{Type: edge.MessageTypeRegister, AgentToken: "test-only", Capabilities: append(capabilities, contract.EdgeProtocolCapability())}))
			registered, err := conn.Receive()
			require.NoError(t, err)
			require.True(t, registered.Accepted)
			ack := make(chan struct{}, 1)
			joined := make(chan struct{})
			go func() {
				defer close(joined)
				for {
					msg, err := conn.Receive()
					if err != nil {
						return
					}
					switch msg.Type {
					case edge.MessageTypeCommandRequest:
						sent.Add(1)
						_ = conn.Send(&edge.TunnelMessage{ID: msg.ID, Type: edge.MessageTypeCommandComplete, Status: 400, Error: "unsupported edge command"})
					case edge.MessageTypeHeartbeatAck:
						ack <- struct{}{}
					}
				}
			}()
			barrier = func() {
				require.NoError(t, conn.Send(&edge.TunnelMessage{Type: edge.MessageTypeHeartbeat}))
				downloadSignal(t, ack, "old agent heartbeat barrier")
			}
			t.Cleanup(func() { _ = conn.Close(); <-joined; registry.Unregister("example-host") })
		} else {
			address, err := url.Parse(agentHTTP.URL)
			require.NoError(t, err)
			client := edge.NewTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: "test-only", Listen: address.Hostname(), Port: address.Port()}, agent)
			life, cancel := context.WithCancel(t.Context())
			joined := make(chan struct{})
			go func() { defer close(joined); client.StartWithErrorChan(life, nil) }()
			t.Cleanup(func() { cancel(); registry.Unregister("example-host"); <-joined })
		}
		downloadSignal(t, connected, "Edge registration")
		endpoint = "edge://example-host"
	}
	hub := echo.New()
	hub.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := access.WithController(c.Request().Context(), &access.Controller{Runtime: func(context.Context, string, string) ([]access.Container, error) {
				return []access.Container{{ID: "assigned", Names: []string{"/vulnerabilities"}}}, nil
			}})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	matcher := authz.NewPermissionMatcher()
	matcher.Add(http.MethodGet, "/containers/{containerId}/logs/download", authz.PermContainersLogs)
	hub.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) {
		return endpoint, new("test-only"), true, nil
	}, func(_ context.Context, c *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
		ps := authz.SudoPermissionSet()
		if c.Request().Header.Get("X-Test-Principal") == "restricted" {
			ps = access.Synthesize("example-user", []user.AppAssignment{{EnvironmentID: "example-host", ProjectID: "example-stack", Level: "restricted"}})
		}
		return ps, &user.Actor{ID: "example-user", Username: "operator"}, true
	}, matcher, registry, nil, middleware.EnvironmentProxySecurity{}))
	hub.GET("/api/environments/:id/containers/:containerId/logs/download", func(c *echo.Context) error { return c.NoContent(418) })
	httpServer := httptest.NewServer(hub)
	t.Cleanup(httpServer.Close)
	return downloadHub{httpServer, &sent, barrier}
}
func downloadSignal(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("%s not observed within barrier", label)
	}
}
func downloadRequest(t *testing.T, hub downloadHub, name, principal string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, hub.server.URL+"/api/environments/example-host/containers/"+name+"/logs/download", nil)
	require.NoError(t, err)
	req.Header.Set("X-Test-Principal", principal)
	resp, err := hub.server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestLogsDownloadTransportRightsAndRoute(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		for _, principal := range []string{"admin", "restricted"} {
			for _, name := range []string{"assigned", "vulnerabilities", "foreign"} {
				t.Run(transport+"/"+principal+"/"+name, func(t *testing.T) {
					source := newDownloadSource(t, 31, false)
					hub := newDownloadHub(t, transport, downloadAgent(t, source), false)
					resp := downloadRequest(t, hub, name, principal)
					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					if principal == "restricted" && name == "foreign" {
						require.Equal(t, http.StatusNotFound, resp.StatusCode, "foreign container is refused before forwarding")
						require.Zero(t, source.reads.Load())
						require.NotContains(t, string(body), strings.Repeat("x", 31))
						return
					}
					require.Equal(t, http.StatusOK, resp.StatusCode, "download command must succeed")
					require.Equal(t, strings.Repeat("x", 31), string(body))
					require.Equal(t, "attachment; filename=container-0123456789ab-logs.log", resp.Header.Get("Content-Disposition"))
					if transport == "edge" {
						command, ok := edge.ResolveEdgeCommandName(http.MethodGet, "/api/environments/0/containers/"+name+"/logs/download", false).Get()
						require.True(t, ok)
						require.Equal(t, "container.logs.download", command)
						require.Contains(t, edge.AdvertisedEdgeCommands(), command)
					}
				})
			}
		}
	}
}
func TestLogsDownloadOldAgentRefusedBeforeSend(t *testing.T) {
	source := newDownloadSource(t, 31, false)
	hub := newDownloadHub(t, "edge", downloadAgent(t, source), true)
	resp := downloadRequest(t, hub, "assigned", "admin")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	hub.barrier()
	require.Zero(t, hub.sent.Load(), "unsupported download must not be sent")
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	require.JSONEq(t, `{"success":false,"code":"agent.update-required"}`, string(body))
}
func TestLogsDownloadEdgeCapAndTruncation(t *testing.T) {
	for _, scenario := range []string{"cap", "cap-plus-one", "over-cap-held", "truncated"} {
		t.Run(scenario, func(t *testing.T) {
			size := dto.MaxEdgeLogsDownloadBytes
			if scenario == "cap-plus-one" || scenario == "over-cap-held" {
				size++
			}
			if scenario == "truncated" {
				size = 31
			}
			source := newDownloadSource(t, size, scenario == "over-cap-held")
			source.broken = scenario == "truncated"
			// Cleanup must unblock the source before any HTTP server waits for handlers.
			hub := newDownloadHub(t, "edge", downloadAgent(t, source), false)
			t.Cleanup(source.stop)
			resp := downloadRequest(t, hub, "vulnerabilities", "admin")
			if scenario == "cap-plus-one" || scenario == "over-cap-held" {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode, "cap+1 must be refused before any success bytes")
				require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
				require.Empty(t, resp.Header.Get("Content-Disposition"))
				require.JSONEq(t, `{"code":"container.logs-too-large","status":413}`, string(body))
				if source.hold {
					downloadSignal(t, source.cancelled, "agent log read cancellation")
				}
				downloadSignal(t, source.closed, "Docker reader close after cap")
			} else {
				require.Equal(t, http.StatusOK, resp.StatusCode)
				n, err := io.Copy(io.Discard, resp.Body)
				require.NoError(t, err)
				require.Equal(t, int64(size), n, "exact cap or current truncation bytes")
				require.Equal(t, "attachment; filename=container-0123456789ab-logs.log", resp.Header.Get("Content-Disposition"))
			}
		})
	}
}
func TestLogsDownloadBrowserAbortStopsReader(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			source := newDownloadSource(t, 0, true)
			hub := newDownloadHub(t, transport, downloadAgent(t, source), false)
			t.Cleanup(source.stop)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, hub.server.URL+"/api/environments/example-host/containers/assigned/logs/download", nil)
			require.NoError(t, err)
			joined := make(chan struct{})
			go func() {
				defer close(joined)
				resp, err := hub.server.Client().Do(req)
				if err == nil {
					_ = resp.Body.Close()
				}
			}()
			downloadSignal(t, source.started, "Docker read start")
			cancel()
			downloadSignal(t, joined, "browser abort")
			downloadSignal(t, source.cancelled, "browser abort reaches agent Docker read")
			downloadSignal(t, source.closed, "Docker reader close after browser abort")
		})
	}
}

func TestLogsDownloadDirectAboveEdgeCap(t *testing.T) {
	source := newDownloadSource(t, dto.MaxEdgeLogsDownloadBytes+1, false)
	hub := newDownloadHub(t, "direct", downloadAgent(t, source), false)
	resp := downloadRequest(t, hub, "assigned", "admin")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	n, err := io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	require.Equal(t, int64(dto.MaxEdgeLogsDownloadBytes+1), n)
	require.Equal(t, "attachment; filename=container-0123456789ab-logs.log", resp.Header.Get("Content-Disposition"))
}
