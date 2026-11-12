// Source: docklet-next/issues/242
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original hub policy cleanup after successful Direct or Edge stack destruction.

package middleware

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/labstack/echo/v5"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

func destroyedStack(method, suffix string) string {
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	if method == http.MethodDelete && len(parts) == 3 && parts[0] == "projects" && parts[1] != "" && parts[2] == "destroy" {
		return parts[1]
	}
	return ""
}

// cleanupStack forgets the hub policy of a stack the agent destroyed. A cleanup
// failure never turns the agent's success into an error: the row then shows as a
// missing target and follows the ordinary pruning rule.
func (m *EnvironmentMiddleware) cleanupStack(c *echo.Context, status int) {
	env := c.Param(m.paramName)
	target := destroyedStack(c.Request().Method, m.buildResourceSuffix(c.Request().URL.Path, env))
	if target != "" && status >= 200 && status < 300 && m.policyCleanup != nil {
		if err := m.policyCleanup(c.Request().Context(), env, target); err != nil {
			slog.WarnContext(c.Request().Context(), "Hub update policy cleanup after stack destroy failed", "environmentId", env, "error", err)
		}
	}
}

// stackDestroyBodyLimit bounds the destroy flags body forwarded through the Edge tunnel.
const stackDestroyBodyLimit = 64 << 10

func (m *EnvironmentMiddleware) proxyStackDestroy(c *echo.Context, tunnel *edge.AgentTunnel, env string) error {
	var body []byte
	if c.Request().Body != nil {
		read, err := io.ReadAll(io.LimitReader(c.Request().Body, stackDestroyBodyLimit+1))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": "failed to read request body"})
		}
		if len(read) > stackDestroyBodyLimit {
			return c.JSON(http.StatusRequestEntityTooLarge, map[string]any{
				"success": false,
				"code":    "request.too-large",
				"data":    map[string]any{"error": "Request body is too large"},
			})
		}
		body = read
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), edge.DefaultProxyTimeout)
	defer cancel()
	headers := map[string]string{}
	for key, values := range edge.AgentRequestHeaders(c.Request().Header) {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	status, reply, body, err := edge.ProxyRequest(ctx, tunnel, c.Request().Method, m.buildProxyPath(c, env), c.Request().URL.RawQuery, headers, body)
	if err != nil {
		return m.abortEdgeTunnelUnavailable(c)
	}
	m.cleanupStack(c, status)
	for key, value := range reply {
		c.Response().Header().Set(key, value)
	}
	return c.Blob(status, "application/json", body)
}
