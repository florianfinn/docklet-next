// Source: getarcaneapp/arcane/backend/pkg/libarcane/edge/ws_proxy.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; retain browser cookies at the hub boundary; session-bound viewer lifetime and durable validation; preserve upstream close metadata and terminal exit delivery; revoked terminal viewers forward no new input.

package edge

import (
	"context"
	"log/slog"
	"net/http"
	"time"
	"uuid"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/labstack/echo/v5"

	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
)

const (
	// tunnelWSPingWait bounds the ping round-trip on the browser-facing side of
	// a proxied WebSocket. Reverse proxies (Caddy, Nginx, ...) silently drop
	// idle TCP connections; without a heartbeat the master would hold a dead
	// stream open until something else terminates it.
	tunnelWSPingWait      = 10 * time.Second
	tunnelWSPingPeriod    = 54 * time.Second
	tunnelWSDataWriteWait = 10 * time.Second
)

// ProxyWebSocketRequest proxies a WebSocket upgrade through an edge tunnel.
// This handles logs, stats, and other streaming endpoints.
//
// checkOrigin must be the same Origin validator the local WebSocket endpoints
// use. It is required: the caller's session cookie has already been validated by
// the time this runs, so accepting any Origin would let an attacker-controlled
// page open a terminal or log stream in the tunnelled environment.
func ProxyWebSocketRequest(c *echo.Context, tunnel *AgentTunnel, targetPath string, checkOrigin func(*http.Request) bool) error {
	req := c.Request()
	ctx := req.Context()

	if checkOrigin == nil {
		slog.ErrorContext(ctx, "Refusing edge WebSocket proxy without an origin validator", "path", targetPath)
		return echo.NewHTTPError(http.StatusForbidden, "websocket origin validation unavailable")
	}
	clientWS, err := wshub.Accept(c.Response(), req, checkOrigin)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to upgrade WebSocket for edge proxy", "error", err)
		return nil
	}
	defer func() { _ = clientWS.CloseNow() }()
	terminal := wshub.IsTerminalRequest(req)
	relay := wshub.NewTerminalRelay(ctx, clientWS, terminal)
	ctx = wshub.WithTerminalRelay(ctx, relay)
	if terminal {
		defer relay.Finish(websocket.StatusNormalClosure, "")
	}
	viewerCtx, viewerCancel := context.WithCancel(ctx)
	defer viewerCancel()

	// Client frames are forwarded into tunnel messages capped at
	// maxGRPCTunnelMessageSize; allow the same size here instead of
	// coder/websocket's 32KB default.
	clientWS.SetReadLimit(maxGRPCTunnelMessageSize)

	streamID := uuid.New().String()
	streamParent := viewerCtx
	if terminal {
		streamParent = context.WithoutCancel(viewerCtx)
	}
	streamCtx, cancel := context.WithCancel(streamParent)
	defer cancel()
	lease, err := sessionlife.Register(viewerCtx, func() {
		viewerCancel()
		if !terminal {
			cancel()
			_ = clientWS.CloseNow()
		}
	})
	if err != nil {
		return nil
	}
	defer lease.Release()

	// Register the stream before sending the start message so agent replies
	// are never dropped when the routing goroutine is faster than this goroutine.
	pending := &PendingRequest{
		ResponseCh: make(chan *TunnelMessage, 512),
		failureCh:  make(chan error, 1),
	}
	clientDoneCh := make(chan struct{})

	tunnel.Pending.Store(streamID, pending)
	defer tunnel.Pending.Delete(streamID)

	headers := buildWebSocketHeaders(req)
	if openStreamErr := DefaultCommandClient.OpenStream(streamCtx, tunnel, &CommandRequest{
		ID:      streamID,
		Method:  http.MethodGet,
		Path:    targetPath,
		Query:   req.URL.RawQuery,
		Headers: headers,
	}); openStreamErr != nil {
		slog.ErrorContext(ctx, "Failed to send WebSocket start to agent", "error", openStreamErr)
		return nil
	}

	slog.DebugContext(ctx, "Started WebSocket stream through edge tunnel",
		"streamId", streamID,
		"environmentId", tunnel.EnvironmentID,
		"path", targetPath,
	)

	// Goroutine to read from client and send to agent
	go forwardClientToAgent(viewerCtx, streamCtx, clientWS, tunnel, streamID, clientDoneCh)

	if terminal {
		forwardTerminalAgentToClient(viewerCtx, streamCtx, clientWS, tunnel, pending, streamID, clientDoneCh)
	} else {
		forwardAgentToClient(viewerCtx, streamCtx, clientWS, tunnel, pending, streamID, clientDoneCh)
	}
	if terminal {
		relay.Finish(websocket.StatusNormalClosure, "")
	}
	cancel()
	_ = clientWS.CloseNow()
	<-clientDoneCh
	return nil
}

func buildWebSocketHeaders(req *http.Request) map[string]string {
	headers := make(map[string]string)
	for k, v := range AgentRequestHeaders(req.Header) {
		if len(v) > 0 && http.CanonicalHeaderKey(k) != HeaderCookie {
			headers[k] = v[0]
		}
	}
	return headers
}

func forwardClientToAgent(ctx, streamCtx context.Context, clientWS *websocket.Conn, tunnel *AgentTunnel, streamID string, doneCh chan<- struct{}) {
	defer close(doneCh)
	for {
		msgType, data, err := clientWS.Read(streamCtx)
		if err != nil {
			if !wshub.IsExpectedClose(err) {
				slog.DebugContext(ctx, "Client WebSocket read error", "error", err)
			}
			if wshub.Relay(ctx) != nil && wshub.Relay(ctx).IsTerminal() {
				_ = sendStreamDataInternal(tunnel, streamID, int(websocket.MessageText), wshub.TerminalCloseControl)
			} else {
				sendWebSocketClose(tunnel, streamID)
			}
			return
		}

		if !isForwardableWSMessage(int(msgType)) {
			continue
		}
		// ctx is the viewer context: a revoked terminal may finish its close protocol but never sends new input.
		if ctx.Err() != nil && !wshub.IsTerminalClose(msgType, data) {
			continue
		}

		if sendStreamDataErr := sendStreamDataInternal(tunnel, streamID, int(msgType), data); sendStreamDataErr != nil {
			slog.DebugContext(ctx, "Failed to send WebSocket data to agent", "error", sendStreamDataErr)
			return
		}
		if relay := wshub.Relay(ctx); relay != nil && relay.IsTerminal() && wshub.IsTerminalClose(msgType, data) {
			return
		}
	}
}

func forwardAgentToClient(ctx, streamCtx context.Context, clientWS *websocket.Conn, tunnel *AgentTunnel, pending *PendingRequest, streamID string, clientDoneCh <-chan struct{}) {
	pingTicker := sessionlife.NewTicker(streamCtx, tunnelWSPingPeriod)
	defer pingTicker.Stop()

	for {
		select {
		case <-streamCtx.Done():
			return
		case <-ctx.Done():
			return
		case <-tunnel.done:
			return
		case <-pending.failureCh:
			return
		case <-clientDoneCh:
			return
		case <-pingTicker.C:
			if sessionlife.Check(streamCtx) != nil {
				sendWebSocketClose(tunnel, streamID)
				return
			}
			// Ping round-trips; the pong is serviced by forwardClientToAgent's read.
			pctx, pcancel := context.WithTimeout(streamCtx, tunnelWSPingWait)
			err := clientWS.Ping(pctx)
			pcancel()
			if err != nil {
				slog.DebugContext(ctx, "Failed to ping client WebSocket", "streamId", streamID, "error", err)
				sendWebSocketClose(tunnel, streamID)
				return
			}
		case msg, ok := <-pending.ResponseCh:
			if !ok {
				return
			}
			if shouldStop, err := handleAgentMessage(ctx, streamCtx, clientWS, msg, streamID); err != nil {
				slog.DebugContext(ctx, "Failed to write to client WebSocket", "error", err)
				return
			} else if shouldStop {
				return
			}
		}
	}
}

func handleAgentMessage(ctx, streamCtx context.Context, clientWS *websocket.Conn, msg *TunnelMessage, streamID string) (bool, error) {
	switch msg.Type {
	case MessageTypeWebSocketData, MessageTypeStreamData:
		return false, writeWebSocketData(streamCtx, clientWS, msg)
	case MessageTypeWebSocketClose, MessageTypeStreamClose, MessageTypeStreamEnd:
		slog.DebugContext(ctx, "Agent closed WebSocket stream", "streamId", streamID, "error", msg.Error)
		status, reason := decodeStreamClose(msg.Error)
		if relay := wshub.Relay(ctx); relay != nil {
			relay.Finish(status, reason)
		} else {
			_ = clientWS.Close(status, reason)
		}
		return true, nil
	case MessageTypeRequest,
		MessageTypeResponse,
		MessageTypeHeartbeat,
		MessageTypeHeartbeatAck,
		MessageTypeWebSocketStart,
		MessageTypeRegister,
		MessageTypeRegisterResponse,
		MessageTypeEvent,
		MessageTypeCommandRequest,
		MessageTypeCommandAck,
		MessageTypeCommandOutput,
		MessageTypeCommandComplete,
		MessageTypeFileChunk,
		MessageTypeStreamOpen,
		MessageTypeCancelRequest:
		slog.DebugContext(ctx, "Ignoring tunnel message", "type", msg.Type, "streamId", streamID)
		return false, nil
	default:
		slog.DebugContext(ctx, "Unknown tunnel message", "type", msg.Type, "streamId", streamID)
		return false, nil
	}
}

func writeWebSocketData(streamCtx context.Context, clientWS *websocket.Conn, msg *TunnelMessage) error {
	msgType := websocket.MessageType(msg.WSMessageType)
	if msgType != websocket.MessageText && msgType != websocket.MessageBinary {
		slog.WarnContext(streamCtx, "Dropping WebSocket message with unsupported type", "messageType", msg.WSMessageType)
		return nil
	}
	wctx, cancel := context.WithTimeout(streamCtx, tunnelWSDataWriteWait)
	defer cancel()
	if relay := wshub.Relay(streamCtx); relay != nil {
		return relay.Write(wctx, msgType, msg.Body)
	}
	return clientWS.Write(wctx, msgType, msg.Body)
}

func sendStreamDataInternal(tunnel *AgentTunnel, streamID string, msgType int, data []byte) error {
	wsDataMsg := &TunnelMessage{
		ID:            streamID,
		Type:          MessageTypeStreamData,
		Body:          data,
		WSMessageType: msgType,
	}
	return tunnel.Conn.Send(wsDataMsg)
}

func sendWebSocketClose(tunnel *AgentTunnel, streamID string) {
	closeMsg := &TunnelMessage{
		ID:   streamID,
		Type: MessageTypeStreamClose,
	}
	_ = tunnel.Conn.Send(closeMsg)
}

func isForwardableWSMessage(msgType int) bool {
	return msgType == int(websocket.MessageText) || msgType == int(websocket.MessageBinary)
}
