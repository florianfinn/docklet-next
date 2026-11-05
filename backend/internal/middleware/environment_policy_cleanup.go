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

func (m *EnvironmentMiddleware) cleanupStack(c *echo.Context, status int) error {
	env := c.Param(m.paramName)
	target := destroyedStack(c.Request().Method, m.buildResourceSuffix(c.Request().URL.Path, env))
	if target != "" && status >= 200 && status < 300 && m.policyCleanup != nil {
		return m.policyCleanup(c.Request().Context(), env, target)
	}
	return nil
}

func (m *EnvironmentMiddleware) proxyStackDestroy(c *echo.Context, tunnel *edge.AgentTunnel, env string) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), edge.DefaultProxyTimeout)
	defer cancel()
	headers := map[string]string{}
	for key, values := range edge.AgentRequestHeaders(c.Request().Header) {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	status, reply, body, err := edge.ProxyRequest(ctx, tunnel, c.Request().Method, m.buildProxyPath(c, env), c.Request().URL.RawQuery, headers, nil)
	if err != nil {
		return m.abortEdgeTunnelUnavailable(c)
	}
	if err = m.cleanupStack(c, status); err != nil {
		return pointFailure(c, "update.persistence-failed", 500)
	}
	for key, value := range reply {
		c.Response().Header().Set(key, value)
	}
	return c.Blob(status, "application/json", body)
}
