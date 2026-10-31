// Source: getarcaneapp/arcane/backend/api/ws/exec.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; session-bound viewer lifetime and durable validation; M14 hub shell acceptance and container-owned terminal delegation.

package ws

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/containershell"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
	systemtypes "github.com/florianfinn/docklet-next/types/system"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
)

// ============================================================================
// Container Exec WebSocket Endpoint
// ============================================================================

// ContainerExec provides interactive terminal access to a container.
//
//	@Summary		Execute command in container via WebSocket
//	@Description	Interactive terminal access to a container over WebSocket
//	@Tags			WebSocket
//	@Param			id			path	string	true	"Environment ID"
//	@Param			containerId	path	string	true	"Container ID"
//	@Param			shell		query	string	false	"Shell to execute"	default(auto)
//	@Router			/api/environments/{id}/ws/containers/{containerId}/terminal [get]
func (h *WebSocketHandler) ContainerExec(c *echo.Context) error {
	containerID := c.Param("containerId")
	if strings.TrimSpace(containerID) == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"success": false, "error": "Container ID is required"})
	}

	if _, err := containershell.ParseQuery(c.QueryParams()); err != nil {
		return c.JSON(400, map[string]any{"code": "container.shell-invalid", "status": 400})
	}
	if h.shellSessions != nil {
		authenticatedUser, _ := c.Get(string(middleware.ContextKeyCurrentUser)).(user.ActorProvider)
		var actor *user.Actor
		if authenticatedUser != nil {
			actor = authenticatedUser.Actor()
		}
		ctx, handle, err := h.shellSessions.Accept(c.Request().Context(), c.QueryParams(), actor, c.Param("id"), containerID, httpx.IsWebSocketUpgradeRequest(c.Request()))
		if err != nil {
			return c.NoContent(http.StatusServiceUnavailable)
		}
		if handle != nil {
			defer handle.Release()
		}
		c.SetRequest(c.Request().WithContext(ctx))
	}
	query, _ := containershell.ParseQuery(c.QueryParams())

	conn, unregister, ok := h.acceptWSInternal(c, systemtypes.WSKindContainerExec, containerID)
	if !ok {
		return nil
	}
	defer unregister()
	defer func() { _ = conn.CloseNow() }()

	// Allow large terminal pastes; coder/websocket's default limit is 32KB.
	conn.SetReadLimit(1 << 20)

	ctx, cancel := context.WithCancelCause(c.Request().Context())
	defer cancel(nil)

	// The pong is serviced by the concurrent stdin reader.
	defer startWSKeepaliveInternal(ctx, func() { cancel(errors.New("websocket ping failed")) }, conn, 54*time.Second)()

	if handle := wshub.Terminal(ctx); handle != nil {
		handle.SetCloser(func() { cancel(context.Canceled) })
	}
	h.containerService.ServeTerminal(ctx, conn, containerID, query)
	return nil
}
