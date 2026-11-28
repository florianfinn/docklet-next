// Source: docklet-next/issues/693
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original M47 host container lane ownership and log permission split over real Direct and Edge paths.

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	apiKeyDTO "github.com/florianfinn/docklet-next/types/apikey"
	"github.com/stretchr/testify/require"
)

// The lanes route is answered by the hub for remote hosts (the fixture's hub answers 599 for
// hub-local paths, the agent 200), and a key that may read containers but not their logs keeps
// the lanes while the log stream is refused.
func TestM47LanesStayOnHubAndLogsKeepTheirPermission(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x := newM15Transport(t, mode)
			f := x.fixture
			owner, err := f.users.CreateUser(t.Context(), &user.User{Username: "key-owner"})
			require.NoError(t, err)
			require.NoError(t, f.users.SetAdministrator(t.Context(), "operator", owner.ID, true))
			grant := func(name string, permissions ...string) string {
				grants := []apiKeyDTO.PermissionGrant{}
				for _, p := range permissions {
					grants = append(grants, apiKeyDTO.PermissionGrant{Permission: p, EnvironmentID: new("example")})
				}
				key, err := f.keys.CreateApiKey(t.Context(), owner.ID, authz.SudoPermissionSet(), apiKeyDTO.CreateApiKey{Name: name, Permissions: grants})
				require.NoError(t, err)
				return key.Key
			}
			readOnly := grant("lanes-read", authz.PermContainersRead)
			logs := grant("lanes-logs", authz.PermContainersRead, authz.PermContainersLogs)

			r := httptest.NewRequest("GET", "/api/environments/example/history/containers?range=24h", nil)
			r.Header.Set(middleware.HeaderApiKey, readOnly)
			w := httptest.NewRecorder()
			x.hub.ServeHTTP(w, r)
			require.Equal(t, 599, w.Code, "the lanes read must stay on the hub, never reach the agent: %s", w.Body.String())

			hub := httptest.NewServer(x.hub)
			defer hub.Close()
			dial := func(key string) (*websocket.Conn, *http.Response, error) {
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				headers := http.Header{"Origin": {f.cfg.AppUrl}}
				headers.Set(middleware.HeaderApiKey, key)
				return websocket.Dial(ctx, edge.HTTPToWebSocketURL(hub.URL)+"/api/environments/example/ws/containers/canonical/logs?follow=false&until=2026-01-02T03%3A05%3A00Z", &websocket.DialOptions{HTTPHeader: headers})
			}
			conn, response, err := dial(readOnly)
			require.Error(t, err, "containers:read alone must not open the log stream")
			if conn != nil {
				_ = conn.CloseNow()
			}
			require.NotNil(t, response)
			require.Equal(t, http.StatusForbidden, response.StatusCode)
			conn, _, err = dial(logs)
			require.NoError(t, err, "containers:logs opens the forwarded stream")
			readReady(t, conn)
			forwarded, _ := x.wsQuery.Load().(string)
			require.Contains(t, forwarded, "/ws/containers/canonical/logs?")
			require.Contains(t, forwarded, "until=2026-01-02T03%3A05%3A00Z", "the window end reaches the agent unchanged")
			projectKey := grant("project-logs", authz.PermProjectsRead, authz.PermProjectsLogs)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			headers := http.Header{"Origin": {f.cfg.AppUrl}}
			headers.Set(middleware.HeaderApiKey, projectKey)
			conn, _, err = websocket.Dial(ctx, edge.HTTPToWebSocketURL(hub.URL)+"/api/environments/example/ws/projects/project-uuid/logs?follow=false&until=2026-01-02T03%3A05%3A00Z", &websocket.DialOptions{HTTPHeader: headers})
			require.NoError(t, err, "project logs open the forwarded stream")
			readReady(t, conn)
			forwarded, _ = x.wsQuery.Load().(string)
			require.Contains(t, forwarded, "/ws/projects/project-uuid/logs?")
			require.Contains(t, forwarded, "until=2026-01-02T03%3A05%3A00Z", "the project window end reaches the agent unchanged")
		})
	}
}

// readReady waits for the agent's first frame, so the forwarded request has been observed.
func readReady(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, body, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "ready", string(body))
	_ = conn.CloseNow()
}
