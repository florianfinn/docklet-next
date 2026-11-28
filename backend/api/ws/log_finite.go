// Source: docklet-next/issues/694
// Revision: b80bae99612a0d7f81481fee8f65d4be4dd2caa4
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original lossless finite log reads for bounded windows and pattern mode.

package ws

import (
	"context"
	"encoding/json/v2"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/labstack/echo/v5"
	"go.getarcane.app/docker"
)

// serveFiniteLogStreamInternal answers a non-following read on its own connection instead of the
// shared hub. The live hub drops a slow viewer to protect the others; a finite read must deliver
// its whole window, so each record is written in order and a slow reader slows the Docker read.
// The read ends with a normal close after the last record; a closed connection cancels the source.
// Finite reads always send one record per frame; `batched` applies to live streams only.
func (h *WebSocketHandler) serveFiniteLogStreamInternal(
	c *echo.Context,
	kind, resourceID, label string,
	params logStreamParams,
	stream logStreamer,
	normalizeJSON func(string) wshub.LogMessage,
	normalizeText func(string) string,
) {
	conn, unregister, ok := h.acceptWSInternal(c, kind, resourceID)
	if !ok {
		return
	}
	defer unregister()
	defer func() { _ = conn.CloseNow() }()
	// The viewer never sends; CloseRead services control frames and cancels on close.
	ctx := conn.CloseRead(context.WithoutCancel(c.Request().Context()))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Like live streams, the session is rechecked on the transport cadence; expiry ends the read.
	defer startWSKeepaliveInternal(ctx, cancel, conn, 54*time.Second)()

	lines := make(chan sourceLogLine, 256)
	go func() {
		defer close(lines)
		runLogSourceInternal(ctx, resourceID, label, params, stream, lines, func() {})
	}()
	var seq atomic.Uint64
	for line := range lines {
		var frame []byte
		if params.format == "json" {
			message := normalizeSourceLogLine(line, normalizeJSON)
			message.Seq = seq.Add(1)
			if message.Timestamp == "" {
				message.Timestamp = wshub.NowRFC3339()
			}
			encoded, err := json.Marshal(message)
			if err != nil {
				continue
			}
			frame = encoded
		} else {
			text := line.record.Message
			if line.structured {
				text = docker.LogText(line.record)
			}
			if normalizeText != nil {
				text = normalizeText(text)
			}
			frame = []byte(text)
		}
		if conn.Write(ctx, websocket.MessageText, frame) != nil {
			cancel()
			for range lines {
			}
			return
		}
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
}
