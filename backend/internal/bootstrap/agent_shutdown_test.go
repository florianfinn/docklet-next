// Source: docklet-next/issues/330
// Revision: a37d77d05c92bde3a4eab0cf6fa47ab36411e011
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original regressions for agent requests failing during shutdown (M16 acceptance DV1 D; review ACC-R1-01/02 probes adopted).

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/di"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/user"
)

func shutdownWait(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("effect not observed")
	}
}

// A read in flight when the agent stops fails like the cancelled producer; each hub consumer must still answer with
// the host being unavailable (class rows A1, H1–H4).
func TestAgentShutdownReachesHubConsumersAsUnavailable(t *testing.T) {
	for _, kind := range []string{"cold-images", "cached-images", "image-detail", "point-list", "baseline"} {
		t.Run(kind, func(t *testing.T) {
			suffix := map[string]string{"image-detail": "/images/example", "point-list": "/updater/restore-points", "baseline": "/updater/projects/example/baseline"}[kind]
			if suffix == "" {
				suffix = "/images"
			}
			entered := make(chan struct{})
			router := echo.New()
			router.GET("/api/environments/0"+suffix, func(c *echo.Context) error {
				close(entered)
				<-c.Request().Context().Done()
				problem := imagetypes.Failure("image.usage-unavailable", 503)
				if kind == "point-list" || kind == "baseline" {
					problem = imagetypes.Failure("update.persistence-failed", 500)
				}
				return c.JSON(problem.Status, problem)
			})
			lc := fxtest.NewLifecycle(t)
			srv, err := NewHTTPServer(lc, HTTPServerParams{AppCtx: t.Context(), Config: &config.Config{AgentMode: true, Listen: "127.0.0.1", Port: "0"}, Router: router})
			require.NoError(t, err)
			lc.RequireStart()
			store := &hoststate.Store{}
			fresh := `{"success":true,"data":[{"id":"example"}],"pagination":{"totalItems":1}}`
			if kind == "cached-images" {
				store.Put(hoststate.Key{HostID: "example", List: suffix + "?"}, []byte(fresh))
			}
			hub := echo.New()
			hub.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id",
				func(context.Context, string) (string, *string, bool, error) {
					return "http://" + srv.Addr, nil, true, nil
				},
				func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
					return authz.SudoPermissionSet(), nil, true
				}, nil, nil, nil,
				middleware.EnvironmentProxySecurity{Snapshots: store},
			))
			hub.Any("/api/environments/:id/*", func(c *echo.Context) error { return c.NoContent(http.StatusTeapot) })
			rec := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				hub.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/environments/example"+suffix, nil))
				close(done)
			}()
			shutdownWait(t, entered)
			lc.RequireStop()
			shutdownWait(t, done)
			if kind == "cached-images" {
				require.Equal(t, []any{http.StatusOK, "agent.unavailable"}, []any{rec.Code, rec.Header().Get("X-Docklet-Host-Reason")})
				require.JSONEq(t, fresh, rec.Body.String())
				return
			}
			require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), `"code":"agent.unavailable"`)
		})
	}
}

// Only an uncommitted server failure of an agent is withheld; committed responses and hub answers stay as they were
// (rows A1, A2, A7).
func TestAgentShutdownWithholdsOnlyUncommittedAgentFailures(t *testing.T) {
	for _, h2 := range []bool{false, true} {
		for _, mode := range []string{"uncommitted", "success", "header", "write", "flush", "hub"} {
			t.Run(fmt.Sprintf("h2=%t/%s", h2, mode), func(t *testing.T) {
				entered := make(chan struct{})
				router := echo.New()
				router.GET("/read", func(c *echo.Context) error {
					switch mode {
					case "header":
						c.Response().WriteHeader(http.StatusOK)
					case "write":
						_, _ = c.Response().Write([]byte("prefix"))
					case "flush":
						c.Response().(http.Flusher).Flush()
					}
					close(entered)
					<-c.Request().Context().Done()
					if mode == "success" {
						return c.JSON(http.StatusOK, map[string]string{"code": "original.success"})
					}
					return c.JSON(http.StatusServiceUnavailable, map[string]string{"code": "original.failure"})
				})
				lc := fxtest.NewLifecycle(t)
				srv, err := NewHTTPServer(lc, HTTPServerParams{AppCtx: t.Context(), Config: &config.Config{AgentMode: mode != "hub", Listen: "127.0.0.1", Port: "0"}, Router: router})
				require.NoError(t, err)
				lc.RequireStart()
				var protocols http.Protocols
				protocols.SetHTTP1(!h2)
				protocols.SetUnencryptedHTTP2(h2)
				transport := &http.Transport{Protocols: &protocols}
				defer transport.CloseIdleConnections()
				client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
				type result struct {
					status int
					body   string
					err    error
				}
				results := make(chan result, 1)
				go func() {
					resp, e := client.Get("http://" + srv.Addr + "/read") //nolint:noctx // bounded by the client timeout
					if e != nil {
						results <- result{err: e}
						return
					}
					defer resp.Body.Close()
					body, e := io.ReadAll(resp.Body)
					results <- result{resp.StatusCode, string(body), e}
				}()
				shutdownWait(t, entered)
				lc.RequireStop()
				got := <-results
				switch mode {
				case "uncommitted":
					require.Error(t, got.err, "status=%d body=%s", got.status, got.body)
					require.NotContains(t, got.err.Error(), "Timeout")
				case "hub":
					require.NoError(t, got.err)
					require.Equal(t, http.StatusServiceUnavailable, got.status)
					require.Contains(t, got.body, "original.failure")
				case "success":
					require.NoError(t, got.err)
					require.Equal(t, http.StatusOK, got.status)
					require.Contains(t, got.body, "original.success")
				default:
					require.NoError(t, got.err)
					require.Equal(t, http.StatusOK, got.status)
					require.Contains(t, got.body, "original.failure")
				}
			})
		}
	}
}

// Edge reads run through the tunnel, not the listener; a hub read (a command request) cancelled by the lifecycle stop must
// never deliver its failure (row A5 over both transports).
func TestAgentShutdownEdgeTunnelNeverDeliversCancelledFailure(t *testing.T) {
	for _, transport := range []string{"websocket", "grpc"} {
		t.Run(transport, func(t *testing.T) {
			registry := edge.NewTunnelRegistry()
			tunnelServer := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return "example", nil }, nil)
			hubRouter := echo.New()
			hubRouter.GET("/api/tunnel/connect", tunnelServer.HandleConnect)
			hubLC := fxtest.NewLifecycle(t)
			hubServer, err := NewHTTPServer(hubLC, HTTPServerParams{AppCtx: t.Context(), Config: &config.Config{Listen: "127.0.0.1", Port: "0", EdgeMTLSMode: "disabled"}, Router: hubRouter, TunnelServer: tunnelServer})
			require.NoError(t, err)
			hubLC.RequireStart()
			defer hubLC.RequireStop()
			outcomes := map[string]int{}
			for i := 0; i < 30; i++ {
				entered := make(chan struct{})
				router := echo.New()
				router.GET("/api/environments/0/images", func(c *echo.Context) error {
					close(entered)
					<-c.Request().Context().Done()
					return c.JSON(http.StatusServiceUnavailable, imagetypes.Failure("image.usage-unavailable", 503))
				})
				appCtx, cancel := context.WithCancel(t.Context())
				lc := fxtest.NewLifecycle(t)
				signal := newShutdownSignalInternal()
				cfg := &config.Config{AgentMode: true, EdgeAgent: true, Listen: "127.0.0.1", Port: "0", EdgeMTLSMode: "disabled", EdgeTransport: transport, ManagerApiUrl: "http://" + hubServer.Addr, AgentToken: "example"}
				agentServer, err := NewHTTPServer(lc, HTTPServerParams{AppCtx: appCtx, Config: cfg, Router: router, Shutdown: signal})
				require.NoError(t, err)
				startEdgeTunnelClient(appCtx, lc, cfg, router, agentServer, signal)
				lc.RequireStart()
				var tunnel *edge.AgentTunnel
				require.Eventually(t, func() bool {
					var ok bool
					tunnel, ok = registry.Get("example").Get()
					return ok && !tunnel.Conn.IsClosed()
				}, 3*time.Second, 5*time.Millisecond)
				rec := httptest.NewRecorder()
				c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/environments/example/images", nil), rec)
				finished := make(chan struct{})
				go func() {
					defer close(finished)
					if e := edge.ProxyHTTPRequest(c, tunnel, "/api/environments/0/images"); e != nil {
						t.Errorf("proxy: %v", e)
					}
				}()
				shutdownWait(t, entered)
				lc.RequireStop()
				cancel()
				shutdownWait(t, finished)
				outcomes[fmt.Sprintf("%d %t", rec.Code, strings.Contains(rec.Body.String(), `"code":"agent.unavailable"`))]++
				registry.Unregister("example")
			}
			require.Equal(t, map[string]int{"502 true": 30}, outcomes)
		})
	}
}

// Every end of the application context sets the signal first: a parent cancellation and a fatal actor-host failure
// (the normal stop sets it in Run and in the stop hooks).
func TestAppLifetimeSetsShutdownSignalBeforeEnding(t *testing.T) {
	for _, source := range []string{"parent", "fatal"} {
		t.Run(source, func(t *testing.T) {
			parent, cancelParent := context.WithCancel(t.Context())
			defer cancelParent()
			signal := newShutdownSignalInternal()
			lifetime := newAppLifetimeInternal(parent, signal)
			defer lifetime.release()
			if source == "parent" {
				cancelParent()
			} else {
				actorHostFailure(lifetime.end)(errors.New("example failure"))
			}
			// Begin happens before the cancellation, so no observer of the ended context sees an unset signal.
			<-lifetime.ctx.Done()
			require.True(t, signal.begun())
			if source == "fatal" {
				require.ErrorIs(t, context.Cause(lifetime.ctx), di.ErrActorHostLost)
			}
		})
	}
}

// The production stop sequence driven by each cancellation source never delivers a cancelled Edge read's failure
// (rows A5 with the parent and fatal sources; review ACC-R2 probe adopted).
func TestAgentShutdownEdgeCancellationSourcesNeverDeliverFailure(t *testing.T) {
	for _, variant := range []string{"websocket/parent", "websocket/fatal", "grpc/parent", "grpc/fatal"} {
		transport, source, _ := strings.Cut(variant, "/")
		t.Run(variant, func(t *testing.T) {
			registry := edge.NewTunnelRegistry()
			tunnelServer := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return "example", nil }, nil)
			hubRouter := echo.New()
			hubRouter.GET("/api/tunnel/connect", tunnelServer.HandleConnect)
			hubLC := fxtest.NewLifecycle(t)
			hubServer, err := NewHTTPServer(hubLC, HTTPServerParams{AppCtx: t.Context(), Config: &config.Config{Listen: "127.0.0.1", Port: "0", EdgeMTLSMode: "disabled"}, Router: hubRouter, TunnelServer: tunnelServer})
			require.NoError(t, err)
			hubLC.RequireStart()
			defer hubLC.RequireStop()
			outcomes := map[string]int{}
			for i := 0; i < 100; i++ {
				entered := make(chan struct{})
				router := echo.New()
				router.GET("/api/environments/0/images", func(c *echo.Context) error {
					close(entered)
					<-c.Request().Context().Done()
					return c.JSON(http.StatusServiceUnavailable, imagetypes.Failure("image.usage-unavailable", 503))
				})
				parent, cancelParent := context.WithCancel(t.Context())
				signal := newShutdownSignalInternal()
				lifetime := newAppLifetimeInternal(parent, signal)
				lc := fxtest.NewLifecycle(t)
				cfg := &config.Config{AgentMode: true, EdgeAgent: true, Listen: "127.0.0.1", Port: "0", EdgeMTLSMode: "disabled", EdgeTransport: transport, ManagerApiUrl: "http://" + hubServer.Addr, AgentToken: "example"}
				agentServer, err := NewHTTPServer(lc, HTTPServerParams{AppCtx: lifetime.ctx, Config: cfg, Router: router, Shutdown: signal})
				require.NoError(t, err)
				startEdgeTunnelClient(lifetime.ctx, lc, cfg, router, agentServer, signal)
				lc.RequireStart()
				stopped := make(chan error, 1)
				go func() {
					_ = waitForShutdown(parent, lifetime.ctx, make(chan os.Signal))
					signal.Begin()
					stopped <- lc.Stop(context.WithoutCancel(t.Context()))
				}()
				var tunnel *edge.AgentTunnel
				require.Eventually(t, func() bool {
					var ok bool
					tunnel, ok = registry.Get("example").Get()
					return ok && !tunnel.Conn.IsClosed()
				}, 3*time.Second, 5*time.Millisecond)
				rec := httptest.NewRecorder()
				c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/environments/example/images", nil), rec)
				finished := make(chan struct{})
				go func() {
					defer close(finished)
					if e := edge.ProxyHTTPRequest(c, tunnel, "/api/environments/0/images"); e != nil {
						t.Errorf("proxy: %v", e)
					}
				}()
				shutdownWait(t, entered)
				if source == "parent" {
					cancelParent()
				} else {
					actorHostFailure(lifetime.end)(errors.New("example failure"))
				}
				require.NoError(t, <-stopped)
				lifetime.release()
				cancelParent()
				shutdownWait(t, finished)
				outcomes[fmt.Sprintf("%d %t", rec.Code, strings.Contains(rec.Body.String(), `"code":"agent.unavailable"`))]++
				registry.Unregister("example")
			}
			require.Equal(t, map[string]int{"502 true": 100}, outcomes)
		})
	}
}
