// Source: getarcaneapp/arcane/backend/pkg/libarcane/edge/client.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; extracted focused request, stream and recorder ownership

package edge

import (
	"net/http"
)

func (c *TunnelClient) sendErrorResponse(conn TunnelConnection, requestID string, status int, message string) {
	if conn == nil {
		return
	}
	resp := &TunnelMessage{
		ID:     requestID,
		Type:   MessageTypeResponse,
		Status: status,
		Body:   []byte(message),
	}
	_ = conn.Send(resp)
}

func (c *TunnelClient) sendCommandCompleteInternal(conn TunnelConnection, commandID string, status int, message string) {
	if conn == nil {
		return
	}
	_ = conn.Send(&TunnelMessage{
		ID:     commandID,
		Type:   MessageTypeCommandComplete,
		Status: status,
		Error:  message,
	})
}

func newCommandResponseRecorderInternal(commandID, commandName string, conn TunnelConnection) *commandResponseRecorder {
	return &commandResponseRecorder{
		commandID:   commandID,
		commandName: commandName,
		conn:        conn,
		headers:     make(http.Header),
		statusCode:  http.StatusOK,
	}
}

func (r *commandResponseRecorder) Header() http.Header {
	return r.headers
}

func (r *commandResponseRecorder) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	originalLen := len(b)

	if len(b) == 0 {
		return 0, nil
	}

	if !r.streaming && r.buffer.Len()+len(b) <= defaultCommandChunkSize {
		return r.buffer.Write(b)
	}

	r.streaming = true
	if err := r.flushBufferLocked(); err != nil {
		return 0, err
	}

	for len(b) > 0 {
		chunk := b
		if len(chunk) > defaultCommandChunkSize {
			chunk = chunk[:defaultCommandChunkSize]
		}
		if err := r.conn.Send(&TunnelMessage{
			ID:       r.commandID,
			Type:     MessageTypeCommandOutput,
			Body:     append([]byte(nil), chunk...),
			Sequence: r.sequence,
			Command:  r.commandName,
		}); err != nil {
			return 0, err
		}
		r.sequence++
		b = b[len(chunk):]
	}

	return originalLen, nil
}

func (r *commandResponseRecorder) WriteHeader(statusCode int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statusCode = statusCode
	r.wroteHeader = true
}

func (r *commandResponseRecorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.streaming = true
	_ = r.flushBufferLocked()
}

func (r *commandResponseRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}

	if !r.streaming {
		err := r.conn.Send(&TunnelMessage{
			ID:        r.commandID,
			Type:      MessageTypeCommandComplete,
			Status:    r.statusCode,
			Headers:   flattenResponseHeadersInternal(r.headers),
			Body:      append([]byte(nil), r.buffer.Bytes()...),
			Streaming: false,
			Command:   r.commandName,
		})
		if err != nil {
			return err
		}
		r.closed = true
		return nil
	}

	if err := r.flushBufferLocked(); err != nil {
		return err
	}
	if err := r.conn.Send(&TunnelMessage{
		ID:        r.commandID,
		Type:      MessageTypeCommandComplete,
		Status:    r.statusCode,
		Headers:   flattenResponseHeadersInternal(r.headers),
		Streaming: true,
		Command:   r.commandName,
	}); err != nil {
		return err
	}

	r.closed = true
	return nil
}

func (r *commandResponseRecorder) flushBufferLocked() error {
	if r.buffer.Len() == 0 {
		return nil
	}
	if err := r.conn.Send(&TunnelMessage{
		ID:       r.commandID,
		Type:     MessageTypeCommandOutput,
		Body:     append([]byte(nil), r.buffer.Bytes()...),
		Sequence: r.sequence,
		Command:  r.commandName,
	}); err != nil {
		return err
	}
	r.sequence++
	r.buffer.Reset()
	return nil
}

func flattenResponseHeadersInternal(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for k, vs := range headers {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

func (r *responseRecorder) Header() http.Header {
	return r.headers
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}

func newStreamingResponseRecorder(requestID string, conn TunnelConnection) *streamingResponseRecorder {
	return &streamingResponseRecorder{
		requestID:  requestID,
		conn:       conn,
		headers:    make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (r *streamingResponseRecorder) Header() http.Header {
	return r.headers
}

func (r *streamingResponseRecorder) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.wroteHeader {
		if err := r.writeHeaderLocked(r.statusCode); err != nil {
			return 0, err
		}
	}

	if len(b) == 0 {
		return 0, nil
	}

	originalLen := len(b)
	for len(b) > 0 {
		chunk := b
		if len(chunk) > defaultCommandChunkSize {
			chunk = chunk[:defaultCommandChunkSize]
		}
		if err := r.conn.Send(&TunnelMessage{
			ID:   r.requestID,
			Type: MessageTypeStreamData,
			Body: append([]byte(nil), chunk...),
		}); err != nil {
			return 0, err
		}
		b = b[len(chunk):]
	}
	return originalLen, nil
}

func (r *streamingResponseRecorder) WriteHeader(statusCode int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.statusCode = statusCode
	if r.wroteHeader {
		return
	}
	if err := r.writeHeaderLocked(statusCode); err != nil {
		return
	}
}

func (r *streamingResponseRecorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.wroteHeader {
		_ = r.writeHeaderLocked(r.statusCode)
	}
}

func (r *streamingResponseRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	if !r.wroteHeader {
		if err := r.writeHeaderLocked(r.statusCode); err != nil {
			return err
		}
	}

	if err := r.conn.Send(&TunnelMessage{
		ID:   r.requestID,
		Type: MessageTypeStreamEnd,
	}); err != nil {
		return err
	}

	r.closed = true
	return nil
}

func (r *streamingResponseRecorder) writeHeaderLocked(statusCode int) error {
	respHeaders := make(map[string]string)
	for k, v := range r.headers {
		if len(v) > 0 {
			respHeaders[k] = v[0]
		}
	}
	respHeaders["X-Arcane-Tunnel-Stream"] = "1"

	if err := r.conn.Send(&TunnelMessage{
		ID:      r.requestID,
		Type:    MessageTypeResponse,
		Status:  statusCode,
		Headers: respHeaders,
	}); err != nil {
		return err
	}
	r.wroteHeader = true
	return nil
}

// StartTunnelClient starts the tunnel client with an owned background worker.
