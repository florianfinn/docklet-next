// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original real hub URL pre-upgrade acceptance matrix.

package ws

import (
	"context"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/containershell"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestShellQueryRejectedBeforeUpgradeThroughHubURL(t *testing.T) {
	for _, transport := range []string{"local", "direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("invalid shell reached upstream")
				w.WriteHeader(500)
			}))
			defer upstream.Close()
			e := echo.New()
			sessions := containershell.New(nil)
			e.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) {
				target := upstream.URL
				if transport == "edge" {
					target = "edge://example-host"
				}
				token := "test-only"
				return target, &token, true, nil
			}, func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
				return authz.SudoPermissionSet(), &user.Actor{ID: "example-user"}, true
			}, authz.NewPermissionMatcher(), edge.NewTunnelRegistry(), func(*http.Request) bool { return true }, middleware.EnvironmentProxySecurity{ShellSessions: sessions}))
			h := &WebSocketHandler{shellSessions: sessions}
			e.GET("/api/environments/:id/ws/containers/:containerId/terminal", h.ContainerExec)
			hub := httptest.NewServer(e)
			defer hub.Close()
			env := "example-host"
			if transport == "local" {
				env = "0"
			}
			for _, query := range []string{"shell=/bin/sh", "cols=0", "rows=1001", "shell=sh&shell=bash"} {
				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, hub.URL+"/api/environments/"+env+"/ws/containers/example-container/terminal?"+query, nil)
				require.NoError(t, err)
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
				req.Header.Set("Sec-WebSocket-Version", "13")
				req.Header.Set("Sec-WebSocket-Key", "MDEyMzQ1Njc4OWFiY2RlZg==")
				response, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				require.Equal(t, 400, response.StatusCode)
				require.Contains(t, string(body), "container.shell-invalid")
			}
		})
	}
}

type shellPreflightAudit struct {
	starts atomic.Int32
	ended  chan struct{}
}

func (a *shellPreflightAudit) StartActivity(_ context.Context, _ activitylib.StartRequest) (*activityDTO.Activity, error) {
	a.starts.Add(1)
	return &activityDTO.Activity{ID: "example"}, nil
}
func (a *shellPreflightAudit) CompleteActivity(_ context.Context, _ string, _ activityDTO.Status, _ string, _ *string, _ ...string) (*activityDTO.Activity, error) {
	if a.ended != nil {
		close(a.ended)
	}
	return &activityDTO.Activity{ID: "example"}, nil
}
func TestShellHTTPRefusalProbeDoesNotOpenActivity(t *testing.T) {
	for _, transport := range []string{"local", "direct"} {
		t.Run(transport, func(t *testing.T) {
			audit := &shellPreflightAudit{}
			sessions := containershell.New(audit)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUpgradeRequired) }))
			defer upstream.Close()
			e := echo.New()
			e.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) {
				return upstream.URL, new("test-only"), true, nil
			}, func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
				return authz.SudoPermissionSet(), &user.Actor{ID: "example-user"}, true
			}, authz.NewPermissionMatcher(), nil, func(*http.Request) bool { return true }, middleware.EnvironmentProxySecurity{ShellSessions: sessions}))
			h := &WebSocketHandler{shellSessions: sessions}
			e.GET("/api/environments/:id/ws/containers/:containerId/terminal", h.ContainerExec)
			server := httptest.NewServer(e)
			defer server.Close()
			env := "example-host"
			if transport == "local" {
				env = "0"
			}
			response, err := http.Get(server.URL + "/api/environments/" + env + "/ws/containers/example/terminal?shell=auto")
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusUpgradeRequired, response.StatusCode)
			require.Zero(t, audit.starts.Load())
			require.Zero(t, sessions.ActiveCount())
		})
	}
}

func TestShellLocalUpgradeFailureReleasesAcceptedHandle(t *testing.T) {
	audit := &shellPreflightAudit{ended: make(chan struct{})}
	sessions := containershell.New(audit)
	router := echo.New()
	handler := &WebSocketHandler{shellSessions: sessions}
	router.GET("/api/environments/:id/ws/containers/:containerId/terminal", handler.ContainerExec)
	server := httptest.NewServer(router)
	defer server.Close()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/environments/0/ws/containers/example/terminal", nil)
	require.NoError(t, err)
	request.Header = http.Header{"Connection": {"Upgrade"}, "Upgrade": {"websocket"}, "Sec-WebSocket-Version": {"13"}}
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 400, response.StatusCode)
	<-audit.ended
	require.EqualValues(t, 1, audit.starts.Load())
	require.Zero(t, sessions.ActiveCount())
}
