// Source: getarcaneapp/arcane/backend/pkg/libarcane/edge/client.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; extracted request handling with dormant bounded runtime ingress and cancellation.

package edge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// messageLoop processes incoming messages from the manager
func (c *TunnelClient) messageLoop(ctx context.Context, conn TunnelConnection, workers *sync.WaitGroup) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			msg, err := conn.Receive()
			if err != nil {
				return fmt.Errorf("failed to receive message: %w", err)
			}

			switch msg.Type {
			case MessageTypeRequest:
				c.scheduleRuntimeRequestInternal(ctx, conn, msg, workers, false)
			case MessageTypeCommandRequest:
				if runtimeRequestPathInternal(msg.Path) {
					c.scheduleRuntimeRequestInternal(ctx, conn, msg, workers, true)
				} else if transferID := strings.TrimSpace(msg.Metadata[bodyTransferMetadataKey]); transferID != "" {
					c.beginCommandRequestTransferInternal(ctx, conn, transferID, msg)
				} else {
					workers.Go(func() { c.handleCommandRequest(ctx, conn, msg) })
				}
			case MessageTypeFileChunk:
				c.handleCommandRequestChunkInternal(ctx, msg, workers)
			case MessageTypeWebSocketStart:
				c.handleWebSocketStart(ctx, conn, msg, workers)
			case MessageTypeStreamOpen:
				c.handleStreamOpen(ctx, conn, msg, workers)
			case MessageTypeWebSocketData:
				c.handleWebSocketData(ctx, msg)
			case MessageTypeStreamData:
				c.handleStreamData(ctx, msg)
			case MessageTypeWebSocketClose:
				c.handleWebSocketClose(ctx, msg)
			case MessageTypeStreamClose:
				c.handleStreamClose(ctx, msg)
			case MessageTypeCancelRequest:
				c.cancelRuntimeRequestInternal(conn, msg.ID)
			case MessageTypeResponse, MessageTypeHeartbeat, MessageTypeStreamEnd, MessageTypeEvent, MessageTypeCommandAck, MessageTypeCommandOutput, MessageTypeCommandComplete:
				slog.DebugContext(ctx, "Ignoring message type on agent", "type", msg.Type)
			case MessageTypeHeartbeatAck:
				slog.DebugContext(ctx, "Received heartbeat ack")
			case MessageTypeRegisterResponse:
				if !msg.Accepted {
					return fmt.Errorf("manager rejected tunnel registration: %s", msg.Error)
				}
				slog.InfoContext(ctx, "Edge tunnel re-registered",
					"transport", conn.Transport(),
					"environmentId", msg.EnvironmentID,
				)
			case MessageTypeRegister:
				slog.DebugContext(ctx, "Ignoring register message on agent")
			default:
				slog.WarnContext(ctx, "Unknown message type", "type", msg.Type)
			}
		}
	}
}

func (c *TunnelClient) beginCommandRequestTransferInternal(ctx context.Context, conn TunnelConnection, transferID string, msg *TunnelMessage) {
	if c == nil || conn == nil || msg == nil {
		return
	}

	timeout := c.requestTimeoutInternal()
	transfer := &commandRequestTransfer{ctx: ctx, request: msg, conn: conn}
	transfer.timerMu.Lock()
	previous, loaded := c.requestTransfers.Swap(transferID, transfer)
	transfer.timer = time.AfterFunc(timeout, func() {
		if c.requestTransfers.CompareAndDelete(transferID, transfer) {
			slog.WarnContext(ctx, "Command body transfer expired", "transferId", transferID, "commandId", msg.ID)
			c.sendCommandCompleteInternal(conn, msg.ID, http.StatusRequestTimeout, "command body transfer timed out")
		}
	})
	transfer.timerMu.Unlock()
	if loaded {
		if previous, ok := previous.(*commandRequestTransfer); ok {
			previous.stopInternal()
			if runtimeRequestPathInternal(previous.request.Path) {
				c.cancelRuntimeRequestInternal(previous.conn, previous.request.ID)
			}
			c.sendCommandCompleteInternal(previous.conn, previous.request.ID, http.StatusBadRequest, "duplicate command body transfer ID")
		}
	}
}

func (c *TunnelClient) handleCommandRequestChunkInternal(ctx context.Context, msg *TunnelMessage, workers *sync.WaitGroup) {
	if c == nil || msg == nil {
		return
	}

	value, ok := c.requestTransfers.Load(msg.ID)
	if !ok {
		slog.WarnContext(ctx, "Received command body chunk for unknown transfer", "transferId", msg.ID)
		return
	}
	transfer, ok := value.(*commandRequestTransfer)
	if !ok {
		c.requestTransfers.Delete(msg.ID)
		slog.WarnContext(ctx, "Discarded invalid command body transfer state", "transferId", msg.ID)
		return
	}
	if msg.Sequence != transfer.nextSequence {
		if c.requestTransfers.CompareAndDelete(msg.ID, transfer) {
			transfer.stopInternal()
			if runtimeRequestPathInternal(transfer.request.Path) {
				c.cancelRuntimeRequestInternal(transfer.conn, transfer.request.ID)
			}
			c.sendCommandCompleteInternal(transfer.conn, transfer.request.ID, http.StatusBadRequest, "command body chunks arrived out of order")
		}
		return
	}

	if runtimeRequestPathInternal(transfer.request.Path) && (transfer.ctx.Err() != nil || transfer.body.Len()+len(msg.Body) > runtimeRequestBodyLimit) {
		if c.requestTransfers.CompareAndDelete(msg.ID, transfer) {
			transfer.stopInternal()
			c.cancelRuntimeRequestInternal(transfer.conn, transfer.request.ID)
		}
		c.sendCommandCompleteInternal(transfer.conn, transfer.request.ID, http.StatusRequestEntityTooLarge, "runtime body unavailable")
		return
	}
	_, _ = transfer.body.Write(msg.Body)
	transfer.nextSequence++
	if !msg.EOF {
		return
	}

	if c.requestTransfers.CompareAndDelete(msg.ID, transfer) {
		transfer.stopInternal()
		transfer.request.Body = append([]byte(nil), transfer.body.Bytes()...)
		if runtimeRequestPathInternal(transfer.request.Path) {
			c.startRuntimeHandlerInternal(transfer.ctx, transfer.conn, transfer.request, workers, true)
		} else {
			workers.Go(func() { c.handleCommandRequest(transfer.ctx, transfer.conn, transfer.request) })
		}
	}
}

func (c *TunnelClient) clearCommandRequestTransfersInternal(conn TunnelConnection) {
	if c == nil {
		return
	}

	c.requestTransfers.Range(func(transferID, value any) bool {
		transfer, ok := value.(*commandRequestTransfer)
		if !ok {
			c.requestTransfers.Delete(transferID)
			return true
		}
		if transfer.conn != conn {
			return true
		}
		if c.requestTransfers.CompareAndDelete(transferID, transfer) {
			transfer.stopInternal()
		}
		return true
	})
}

func (c *TunnelClient) handleCommandRequest(ctx context.Context, conn TunnelConnection, msg *TunnelMessage) {
	if !ValidateEdgeCommand(msg.Command, msg.Method, msg.Path, false) && !(c.runtimeCommands && runtimeCommandValidInternal(msg.Command, msg.Method, msg.Path)) {
		c.sendCommandCompleteInternal(conn, msg.ID, http.StatusBadRequest, "unsupported edge command")
		return
	}
	if conn == nil {
		return
	}
	if err := conn.Send(&TunnelMessage{ID: msg.ID, Type: MessageTypeCommandAck, Command: msg.Command}); err != nil {
		slog.WarnContext(ctx, "Failed to acknowledge edge command", "id", msg.ID, "command", msg.Command, "error", err)
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.requestTimeoutInternal())
	defer cancel()

	req, err := c.buildLocalHTTPRequest(reqCtx, msg)
	if err != nil {
		c.sendCommandCompleteInternal(conn, msg.ID, http.StatusInternalServerError, fmt.Sprintf("failed to create request: %v", err))
		return
	}

	recorder := newCommandResponseRecorderInternal(msg.ID, msg.Command, conn)
	c.handler.ServeHTTP(recorder, req)
	if closeErr := recorder.Close(); closeErr != nil {
		slog.WarnContext(reqCtx, "Failed to finalize command response", "id", msg.ID, "command", msg.Command, "error", closeErr)
	}
}

func (c *TunnelClient) buildLocalHTTPRequest(ctx context.Context, msg *TunnelMessage) (*http.Request, error) {
	var body io.Reader
	var bodyBytes []byte
	if len(msg.Body) > 0 {
		bodyBytes = append([]byte(nil), msg.Body...)
		body = bytes.NewReader(bodyBytes)
	}

	path := msg.Path
	if msg.Query != "" {
		path = path + "?" + msg.Query
	}

	if runtimeRequestPathInternal(msg.Path) {
		if !c.runtimeCommands || len(msg.Body) > runtimeRequestBodyLimit || ctx.Err() != nil {
			return nil, errors.New("runtime ingress unavailable")
		}
		ctx = context.WithValue(ctx, runtimeMemoryBodyKeyInternal{}, true)
	}
	req, err := http.NewRequestWithContext(ctx, msg.Method, path, body)
	if err != nil {
		return nil, err
	}

	if bodyBytes != nil && !runtimeRequestPathInternal(msg.Path) {
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
	}

	// Set headers. Note: Go's net/http does not populate req.Host from
	// Header.Set("Host", ...) — it must be set explicitly on the field.
	for k, v := range msg.Headers {
		if http.CanonicalHeaderKey(k) == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}

	return req, nil
}

// handleRequest processes an incoming request and sends back a response
func (c *TunnelClient) handleRequest(ctx context.Context, conn TunnelConnection, msg *TunnelMessage) {
	if isGRPCConnection(conn) {
		c.handleRequestStreaming(ctx, conn, msg)
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.requestTimeoutInternal())
	defer cancel()
	reqCtx = context.WithValue(reqCtx, internalTunnelRequestContextKey{}, true)

	slog.DebugContext(reqCtx, "Processing tunneled request", "id", msg.ID, "method", msg.Method, "path", msg.Path, "bodyLength", len(msg.Body))

	req, err := c.buildLocalHTTPRequest(reqCtx, msg)
	if err != nil {
		c.sendErrorResponse(conn, msg.ID, http.StatusInternalServerError, fmt.Sprintf("failed to create request: %v", err))
		return
	}

	// Use a response recorder to capture the response
	rw := &responseRecorder{
		headers:    make(http.Header),
		statusCode: http.StatusOK,
	}

	// Execute the request through the local handler
	c.handler.ServeHTTP(rw, req)

	// Build response message
	respHeaders := make(map[string]string)
	for k, v := range rw.headers {
		if len(v) > 0 {
			respHeaders[k] = v[0]
		}
	}

	resp := &TunnelMessage{
		ID:      msg.ID,
		Type:    MessageTypeResponse,
		Status:  rw.statusCode,
		Headers: respHeaders,
		Body:    rw.body.Bytes(),
	}

	if sendErr := conn.Send(resp); sendErr != nil {
		slog.ErrorContext(reqCtx, "Failed to send response", "id", msg.ID, "error", sendErr)
	} else {
		slog.DebugContext(reqCtx, "Sent tunneled response", "id", msg.ID, "status", rw.statusCode)
	}
}

func isGRPCConnection(conn TunnelConnection) bool {
	if conn == nil {
		return false
	}
	return conn.Transport() == EdgeTransportGRPC
}

func (c *TunnelClient) handleRequestStreaming(ctx context.Context, conn TunnelConnection, msg *TunnelMessage) {
	reqCtx, cancel := context.WithTimeout(ctx, c.requestTimeoutInternal())
	defer cancel()
	reqCtx = context.WithValue(reqCtx, internalTunnelRequestContextKey{}, true)

	slog.DebugContext(reqCtx, "Processing tunneled request (streaming)", "id", msg.ID, "method", msg.Method, "path", msg.Path, "bodyLength", len(msg.Body))

	req, err := c.buildLocalHTTPRequest(reqCtx, msg)
	if err != nil {
		c.sendErrorResponse(conn, msg.ID, http.StatusInternalServerError, fmt.Sprintf("failed to create request: %v", err))
		return
	}

	recorder := newStreamingResponseRecorder(msg.ID, conn)
	c.handler.ServeHTTP(recorder, req)

	if closeErr := recorder.Close(); closeErr != nil {
		slog.WarnContext(reqCtx, "Failed to finalize streamed response", "id", msg.ID, "error", closeErr)
	}
}

// handleWebSocketStart handles a WebSocket stream start request from the manager.
