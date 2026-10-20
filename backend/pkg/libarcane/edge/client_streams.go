// Source: getarcaneapp/arcane/backend/pkg/libarcane/edge/client.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; extracted focused request, stream and recorder ownership

package edge

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
)

func (c *TunnelClient) handleWebSocketStart(ctx context.Context, conn TunnelConnection, msg *TunnelMessage, workers *sync.WaitGroup) {
	if msg.Command == "" {
		if commandName, ok := ResolveEdgeCommandName(http.MethodGet, msg.Path, true).Get(); ok {
			msg.Command = commandName
		}
	}
	streamID := msg.ID
	slog.DebugContext(ctx, "Starting WebSocket stream", "streamId", streamID, "path", msg.Path)

	localURL := c.buildLocalWebSocketURLInternal(msg)
	headers := c.buildLocalWebSocketHeadersInternal(msg)

	ws, resp, err := c.dialLocalWebSocket(ctx, localURL, headers)
	// coder/websocket leaves resp.Body nil on a successful handshake.
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		attrs := []any{"error", err, "url", localURL}
		if resp != nil {
			attrs = append(attrs, "status", resp.StatusCode)
			if resp.Body != nil {
				buf := make([]byte, 512)
				n, _ := resp.Body.Read(buf)
				if n > 0 {
					attrs = append(attrs, "response_body", string(buf[:n]))
				}
			}
		}
		slog.ErrorContext(ctx, "Failed to dial local WebSocket", attrs...)
		c.sendWebSocketClose(conn, streamID)
		return
	}

	streamCtx, cancel := context.WithCancel(ctx)
	stream := c.registerStream(conn, streamID, ws, cancel)

	workers.Go(func() { c.startLocalWebSocketReadLoop(ctx, streamCtx, streamID, ws, stream) })
	workers.Go(func() { c.startLocalWebSocketWriteLoop(ctx, streamCtx, ws, stream, cancel) })
}

func (c *TunnelClient) handleStreamOpen(ctx context.Context, conn TunnelConnection, msg *TunnelMessage, workers *sync.WaitGroup) {
	if !ValidateEdgeCommand(msg.Command, http.MethodGet, msg.Path, true) {
		c.sendStreamCloseMessage(conn, msg.ID, "unsupported edge stream")
		return
	}

	c.handleWebSocketStart(ctx, conn, &TunnelMessage{
		ID:      msg.ID,
		Type:    MessageTypeWebSocketStart,
		Command: msg.Command,
		Path:    msg.Path,
		Query:   msg.Query,
		Headers: msg.Headers,
	}, workers)
}

func (c *TunnelClient) buildLocalWebSocketURLInternal(msg *TunnelMessage) string {
	path := msg.Path
	if msg.Query != "" {
		path = path + "?" + msg.Query
	}
	host := c.localWebSocketHostInternal()
	return "ws://" + net.JoinHostPort(host, c.localPort) + path
}

func (c *TunnelClient) localWebSocketHostInternal() string {
	listenHost := strings.TrimSpace(c.cfg.Listen)
	if listenHost == "" {
		return "localhost"
	}

	// LISTEN may be just a host ("0.0.0.0"), host:port ("0.0.0.0:3552"),
	// IPv6 ("::"), or bracketed IPv6 with port ("[::]:3552").
	if strings.HasPrefix(listenHost, ":") {
		return "localhost"
	}
	if host, _, err := net.SplitHostPort(listenHost); err == nil {
		listenHost = host
	}

	trimmed := strings.Trim(listenHost, "[]")

	switch trimmed {
	case "", "0.0.0.0", "::":
		return "localhost"
	default:
		return trimmed
	}
}

// localDialSkipHeaders lists headers that must not be forwarded when the
// agent dials its own local HTTP server for a proxied WebSocket stream.
// This includes:
//   - Standard WebSocket handshake headers (coder/websocket sets its own)
//   - Browser-specific headers that were forwarded through the tunnel from
//     the manager.  These cause handshake failures because the agent's
//     WebSocket upgrader validates the Origin against localhost, not the
//     browser's remote origin.
var localDialSkipHeaders = map[string]bool{
	// WebSocket handshake (coder/websocket adds its own)
	"Sec-Websocket-Key":        true,
	"Sec-Websocket-Version":    true,
	"Sec-Websocket-Extensions": true,
	"Upgrade":                  true,
	"Connection":               true,
	"Host":                     true,

	// Browser headers forwarded through the tunnel that are invalid
	// for a server-to-server local dial.
	"Origin":             true,
	"Cookie":             true,
	"Authorization":      true,
	"Referer":            true,
	"Sec-Fetch-Dest":     true,
	"Sec-Fetch-Mode":     true,
	"Sec-Fetch-Site":     true,
	"Sec-Fetch-User":     true,
	"Sec-Ch-Ua":          true,
	"Sec-Ch-Ua-Mobile":   true,
	"Sec-Ch-Ua-Platform": true,
}

func (c *TunnelClient) buildLocalWebSocketHeadersInternal(msg *TunnelMessage) http.Header {
	headers := http.Header{}
	for k, v := range msg.Headers {
		canonicalKey := http.CanonicalHeaderKey(k)
		if !localDialSkipHeaders[canonicalKey] {
			headers.Set(canonicalKey, v)
		}
	}

	if c.cfg.AgentToken != "" {
		headers.Set(HeaderAPIKey, c.cfg.AgentToken)
		headers.Set(HeaderAgentToken, c.cfg.AgentToken)
	}

	return headers
}

func (c *TunnelClient) dialLocalWebSocket(ctx context.Context, localURL string, headers http.Header) (*websocket.Conn, *http.Response, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ws, resp, err := websocket.Dial(dialCtx, localURL, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		return nil, resp, err
	}
	// Local stream frames (exec output, log lines) are forwarded into tunnel
	// messages capped at maxGRPCTunnelMessageSize; allow the same size here
	// instead of coder/websocket's 32KB default.
	ws.SetReadLimit(maxGRPCTunnelMessageSize)
	return ws, resp, nil
}

func (c *TunnelClient) registerStream(conn TunnelConnection, streamID string, ws *websocket.Conn, cancel context.CancelFunc) *activeWSStream {
	stream := &activeWSStream{
		ws:     ws,
		conn:   conn,
		cancel: cancel,
		dataCh: make(chan wsPayload, 100),
	}
	c.activeStreams.Store(streamID, stream)
	return stream
}

func (c *TunnelClient) closeWebSocketStream(streamID string, stream *activeWSStream) {
	stream.mu.Lock()
	if stream.closed {
		stream.mu.Unlock()
		return
	}
	stream.closed = true
	close(stream.dataCh)
	stream.mu.Unlock()

	stream.cancel()
	_ = stream.ws.CloseNow()
	c.activeStreams.Delete(streamID)
}

// closeAllStreams tears down every active WebSocket stream. It is called when
// the message loop exits so a reconnect cannot leak stream goroutines, local
// sockets, or activeStreams entries. closeWebSocketStream is idempotent, so it
// is safe to race a concurrent manager-driven stream close.
func (c *TunnelClient) closeAllStreams() {
	c.activeStreams.Range(func(key, value any) bool {
		streamID, ok := key.(string)
		if !ok {
			return true
		}
		if stream, localOk := value.(*activeWSStream); localOk {
			c.closeWebSocketStream(streamID, stream)
		}
		return true
	})
}

func (c *TunnelClient) startLocalWebSocketReadLoop(ctx, streamCtx context.Context, streamID string, ws *websocket.Conn, stream *activeWSStream) {
	defer func() {
		c.closeWebSocketStream(streamID, stream)
	}()

	for {
		msgType, data, err := ws.Read(streamCtx)
		if err != nil {
			if !wshub.IsExpectedClose(err) {
				slog.DebugContext(ctx, "Local WebSocket read error", "error", err)
			}
			c.sendWebSocketClose(stream.conn, streamID)
			return
		}

		if sendWebSocketDataErr := c.sendWebSocketData(stream.conn, streamID, int(msgType), data); sendWebSocketDataErr != nil {
			slog.DebugContext(ctx, "Failed to send WebSocket data to manager", "error", sendWebSocketDataErr)
			return
		}
	}
}

func (c *TunnelClient) startLocalWebSocketWriteLoop(ctx, streamCtx context.Context, ws *websocket.Conn, stream *activeWSStream, cancel context.CancelFunc) {
	for {
		select {
		case <-streamCtx.Done():
			return
		case payload, ok := <-stream.dataCh:
			if !ok {
				return
			}
			msgType := websocket.MessageType(payload.messageType)
			if msgType != websocket.MessageText && msgType != websocket.MessageBinary {
				slog.WarnContext(ctx, "Dropping WebSocket message with unsupported type", "messageType", payload.messageType)
				continue
			}
			if err := ws.Write(streamCtx, msgType, payload.data); err != nil {
				slog.DebugContext(ctx, "Failed to write to local WebSocket", "error", err)
				cancel()
				return
			}
		}
	}
}

func (c *TunnelClient) sendWebSocketData(conn TunnelConnection, streamID string, msgType int, data []byte) error {
	if conn == nil {
		return ErrNoActiveAgentTunnel
	}
	wsDataMsg := &TunnelMessage{
		ID:            streamID,
		Type:          MessageTypeWebSocketData,
		Body:          data,
		WSMessageType: msgType,
	}
	return conn.Send(wsDataMsg)
}

func (c *TunnelClient) sendWebSocketClose(conn TunnelConnection, streamID string) {
	c.sendStreamCloseMessage(conn, streamID, "")
}

func (c *TunnelClient) sendStreamCloseMessage(conn TunnelConnection, streamID, message string) {
	if conn == nil {
		return
	}
	closeMsg := &TunnelMessage{
		ID:    streamID,
		Type:  MessageTypeStreamClose,
		Error: message,
	}
	_ = conn.Send(closeMsg)
}

// handleWebSocketData handles incoming WebSocket data from the manager.
func (c *TunnelClient) handleWebSocketData(ctx context.Context, msg *TunnelMessage) {
	c.handleStreamData(ctx, msg)
}

func (c *TunnelClient) handleStreamData(ctx context.Context, msg *TunnelMessage) {
	streamRaw, ok := c.activeStreams.Load(msg.ID)
	if !ok {
		slog.DebugContext(ctx, "Received WebSocket data for unknown stream", "streamId", msg.ID)
		return
	}
	stream, ok := streamRaw.(*activeWSStream)
	if !ok {
		return
	}
	stream.mu.Lock()
	if stream.closed {
		stream.mu.Unlock()
		return
	}
	select {
	case stream.dataCh <- wsPayload{messageType: msg.WSMessageType, data: msg.Body}:
		stream.mu.Unlock()
	default:
		stream.mu.Unlock()
		// Drop if channel is full (backpressure)
		slog.DebugContext(ctx, "Dropping WebSocket data due to backpressure", "streamId", msg.ID)
	}
}

// handleWebSocketClose handles WebSocket close from the manager.
func (c *TunnelClient) handleWebSocketClose(ctx context.Context, msg *TunnelMessage) {
	c.handleStreamClose(ctx, msg)
}

func (c *TunnelClient) handleStreamClose(ctx context.Context, msg *TunnelMessage) {
	streamRaw, ok := c.activeStreams.Load(msg.ID)
	if !ok {
		return
	}
	stream, ok := streamRaw.(*activeWSStream)
	if !ok {
		return
	}
	c.closeWebSocketStream(msg.ID, stream)
	slog.DebugContext(ctx, "Closed WebSocket stream", "streamId", msg.ID)
}

// sendErrorResponse sends an error response
