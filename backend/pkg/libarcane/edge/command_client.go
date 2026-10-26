// Source: getarcaneapp/arcane/backend/pkg/libarcane/edge/command_client.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; finite runtime responses, cancellation signaling and pending limits.

package edge

import (
	"context"
	"errors"
	"fmt"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"net/http"
	"slices"
	"time"
	"uuid"
)

const (
	tunnelCapabilityChunkedRequest = "chunked-request"
	// tunnelCapabilityProtoParity signals the peer can decode the full
	// TunnelMessage vocabulary natively over gRPC (stream_data/stream_end
	// manager->agent, cancel_request agent->manager) instead of the legacy
	// re-encoded forms.
	tunnelCapabilityProtoParity = "proto-parity-v1"
	bodyTransferMetadataKey     = "body_transfer_id"
)

// ErrCommandResponseTooLarge distinguishes invalid agent evidence from transport loss.
var ErrCommandResponseTooLarge = errors.New("runtime response too large")

func NewCommandClient() *CommandClient {
	return &CommandClient{}
}

func (c *CommandClient) Execute(ctx context.Context, tunnel *AgentTunnel, req *CommandRequest) (*CommandResult, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateConnectedTunnelInternal(tunnel); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errors.New("command request is required")
	}

	commandName := req.Command
	if commandName == "" {
		resolved, ok := ResolveEdgeCommandName(req.Method, req.Path, false).Get()
		if !ok {
			return nil, fmt.Errorf("unsupported edge command for %s %s", req.Method, req.Path)
		}
		commandName = resolved
	}

	requestID := req.ID
	if requestID == "" {
		requestID = uuid.New().String()
	}

	timeoutMillis := req.TimeoutMillis
	if timeoutMillis <= 0 {
		timeoutMillis = int64(DefaultProxyTimeout / time.Millisecond)
	}

	msg := &TunnelMessage{
		ID:            requestID,
		Type:          MessageTypeCommandRequest,
		Command:       commandName,
		Method:        req.Method,
		Path:          req.Path,
		Query:         req.Query,
		Headers:       req.Headers,
		Body:          req.Body,
		TimeoutMillis: timeoutMillis,
		SessionID:     tunnel.SessionID,
		AgentInstance: tunnel.AgentInstance,
	}

	pendingLimit := 0
	if runtimeRequestPathInternal(req.Path) || pointListRequest(req.Method, req.Path) {
		pendingLimit = runtimePendingLimit
	}
	pending, err := registerBoundedPendingRequestInternal(tunnel, requestID, pendingLimit)
	if err != nil {
		return nil, err
	}
	defer tunnel.Pending.Delete(requestID)
	completed := false
	defer func() {
		if !completed && runtimeRequestPathInternal(req.Path) {
			_ = tunnel.Conn.Send(&TunnelMessage{ID: requestID, Type: MessageTypeCancelRequest})
		}
	}()

	chunkRequestBody := len(req.Body) > defaultCommandChunkSize && slices.Contains(tunnel.Capabilities, tunnelCapabilityChunkedRequest)
	if chunkRequestBody {
		transferID := uuid.New().String()
		msg.Body = nil
		msg.Metadata = map[string]string{bodyTransferMetadataKey: transferID}
	}

	if sendErr := tunnel.Conn.Send(msg); sendErr != nil {
		return nil, fmt.Errorf("tunnel request failed: %w", sendErr)
	}
	if chunkRequestBody {
		transferID := msg.Metadata[bodyTransferMetadataKey]
		for sequence, offset := int64(0), 0; offset < len(req.Body); sequence++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			end := min(offset+defaultCommandChunkSize, len(req.Body))
			if sendErr2 := tunnel.Conn.Send(&TunnelMessage{
				ID:       transferID,
				Type:     MessageTypeFileChunk,
				Body:     req.Body[offset:end],
				Sequence: sequence,
				EOF:      end == len(req.Body),
			}); sendErr2 != nil {
				return nil, fmt.Errorf("tunnel request body transfer failed: %w", sendErr2)
			}
			offset = end
		}
	}

	limit := 0
	if pointListRequest(req.Method, req.Path) {
		limit = dto.MaxRestorePointResponseBytes
	}
	if runtimeRequestPathInternal(req.Path) {
		limit = runtimeRequestBodyLimit
	}
	status, headers, body, err := collectCommandResponseBoundedInternal(ctx, tunnel, pending, req.Method, limit)
	if err != nil {
		return nil, err
	}

	completed = true
	return &CommandResult{
		Status:  status,
		Headers: headers,
		Body:    body,
	}, nil
}

func (c *CommandClient) OpenStream(ctx context.Context, tunnel *AgentTunnel, req *CommandRequest) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if err := validateConnectedTunnelInternal(tunnel); err != nil {
		return err
	}
	if req == nil {
		return errors.New("command request is required")
	}
	if req.ID == "" {
		return errors.New("stream ID is required")
	}

	commandName := req.Command
	if commandName == "" {
		resolved, ok := ResolveEdgeCommandName(http.MethodGet, req.Path, true).Get()
		if !ok {
			return fmt.Errorf("unsupported edge stream target %q", req.Path)
		}
		commandName = resolved
	}

	msg := &TunnelMessage{
		ID:        req.ID,
		Type:      MessageTypeStreamOpen,
		Command:   commandName,
		Path:      req.Path,
		Query:     req.Query,
		Headers:   req.Headers,
		SessionID: tunnel.SessionID,
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return tunnel.Conn.Send(msg)
	}
}

var DefaultCommandClient = NewCommandClient()

func validateConnectedTunnelInternal(tunnel *AgentTunnel) error {
	if tunnel == nil || tunnel.Conn == nil || tunnel.Conn.IsClosed() {
		return errors.New("edge tunnel is not connected")
	}
	return nil
}

// Point lists are finite responses; enforce bounds during assembly, including
// close-drain and pending response queues, before the hub decodes any body.
func pointListRequest(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	command, ok := ResolveEdgeCommandName(method, path, false).Get()
	return ok && (command == "updater.restore.list" || command == "updater.restore.host-list")
}
