// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: c477a3d9aba28f8ae7ab6683b56e380a36fa1aa1c6d8e7de3bffeb4b61b7305f
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original escaped route identity regression tests for issue #260.

package edge

import (
	"fmt"
	dto "github.com/florianfinn/docklet-next/types/vulnerability"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAgentEscapedCommandsAndStreams(t *testing.T) {
	for _, item := range []struct{ suffix, route, want string }{
		{"/images/team%2Frepo%3Atag", "/images/:name", "team/repo:tag"},
		{"/containers/app%2Eblue", "/containers/:name", "app.blue"},
		{"/volumes/app%2Ddata", "/volumes/:name", "app-data"},
		{"/projects/stack%20blue/compose", "/projects/:name/compose", "stack blue"},
		{"/projects/stack%20blue/workspace/file", "/projects/:name/workspace/file", "stack blue"},
		{"/volumes/app%2Ddata/workspace/file", "/volumes/:name/workspace/file", "app-data"},
	} {
		t.Run(item.suffix, func(t *testing.T) {
			path := "/api/environments/0" + item.suffix
			command, ok := ResolveEdgeCommandName("GET", path, false).Get()
			require.True(t, ok)
			agent := echo.NewWithConfig(echo.Config{Router: echo.NewRouter(echo.RouterConfig{UnescapePathParamValues: true})})
			called := false
			agent.GET("/api/environments/:id"+item.route, func(c *echo.Context) error {
				called = true
				require.Equal(t, item.want, c.Param("name"))
				require.Equal(t, "dir/file name.txt", c.QueryParam("path"))
				require.Equal(t, path, c.Request().URL.EscapedPath())
				return c.String(200, "resource")
			})
			client := &TunnelClient{handler: agent}
			conn := &fakeTunnelConn{}
			client.handleCommandRequest(t.Context(), conn, &TunnelMessage{ID: "v3", Command: command, Method: "GET", Path: path, Query: "path=dir%2Ffile+name.txt"})
			require.True(t, called)
			require.GreaterOrEqual(t, len(conn.msgs), 2)
			require.Equal(t, 200, conn.msgs[len(conn.msgs)-1].Status)
		})
	}
	for _, item := range []struct{ suffix, want string }{
		{"/ws/containers/app%2Eblue/logs", "container.logs.stream"},
		{"/ws/containers/app%2Eblue/terminal", "container.exec.stream"},
		{"/ws/projects/stack%20blue/logs", "project.logs.stream"},
	} {
		t.Run(item.suffix, func(t *testing.T) {
			path := "/api/environments/0" + item.suffix
			command, ok := ResolveEdgeCommandName("GET", path, true).Get()
			require.True(t, ok)
			require.Equal(t, item.want, command)
			require.True(t, ValidateEdgeCommand(command, "GET", path, true))
			client := &TunnelClient{cfg: &Config{Listen: "127.0.0.1"}, localPort: "9999"}
			target := client.buildLocalWebSocketURLInternal(&TunnelMessage{Path: path, Query: "tail=20&shell=%2Fbin%2Fsh"})
			parsed, e := url.Parse(target)
			require.NoError(t, e)
			require.Equal(t, path, parsed.EscapedPath())
			require.Equal(t, "/bin/sh", parsed.Query().Get("shell"))
			req := httptest.NewRequest(http.MethodGet, target, nil)
			require.Equal(t, path, req.URL.EscapedPath())
		})
	}
}
func TestCommandQueriesAcrossAllRoutes(t *testing.T) {
	for _, r := range dto.Routes() {
		for _, query := range []string{"?", "?bare", "?filter=a%3Fb%26c"} {
			t.Run(fmt.Sprint(r.Method, r.Path, query), func(t *testing.T) {
				path := "/api" + strings.NewReplacer("{id}", "0", "{imageId}", "team%2Frepo", "{activityId}", "run", "{ignoreId}", "ignore").Replace(r.Path) + query
				require.True(t, dto.IsRoute(r.Method, path))
				resolved, ok := ResolveEdgeCommandName(r.Method, path, false).Get()
				require.True(t, ok)
				require.Equal(t, r.CommandName, resolved)
				c := &vulnerabilityChunks{}
				tunnel := NewAgentTunnelWithConn("example", c)
				c.tunnel = tunnel
				_, e := NewCommandClient().Execute(t.Context(), tunnel, &CommandRequest{Method: r.Method, Path: path})
				require.ErrorIs(t, e, ErrCommandResponseTooLarge)
				require.True(t, c.cancelled)
			})
		}
	}
}
