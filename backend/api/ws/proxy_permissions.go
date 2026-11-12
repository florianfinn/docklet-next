// Source: getarcaneapp/arcane/backend/api/ws/proxy_permissions.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed Swarm.; M15 resource authorization and isolated transport helpers.

package ws

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
)

// proxiedWSRoute couples a proxied WebSocket route (relative to the
// /environments/{id}/ws group) with its handler and the permission it requires.
// It is the single source of truth for both Echo route registration and the
// remote environment proxy permission matcher, so the two can never drift.
type proxiedWSRoute struct {
	path    string
	handler echo.HandlerFunc
	perm    string
}

// proxiedRoutes returns the WebSocket streams that are proxied to remote
// environments, each paired with its required permission.
func (h *WebSocketHandler) proxiedRoutes() []proxiedWSRoute {
	return []proxiedWSRoute{
		{"/projects/:projectId/logs", h.ProjectLogs, authz.PermProjectsLogs},
		{"/containers/:containerId/logs", h.ContainerLogs, authz.PermContainersLogs},
		{"/containers/:containerId/stats", h.ContainerStats, authz.PermContainersRead},
		{"/containers/:containerId/terminal", h.ContainerExec, authz.PermContainersExec},
		{"/system/stats", h.SystemStats, authz.PermSystemRead},
	}
}

// AddProxiedPermissions registers the permissions required by proxied WebSocket
// routes with the environment proxy permission matcher. These streams are
// served by Echo (not Huma), so they are absent from the OpenAPI document and
// must be registered with the matcher separately. The paths are stored relative
// to /environments/{id} (i.e. prefixed with "/ws"), matching the resource
// suffix the proxy computes for forwarded requests.
func AddProxiedPermissions(m *authz.PermissionMatcher) {
	var h WebSocketHandler
	for _, r := range h.proxiedRoutes() {
		m.AddOperation(http.MethodGet, "/ws"+r.path, r.perm, "GET /api/environments/:id/ws"+r.path)
	}
}
