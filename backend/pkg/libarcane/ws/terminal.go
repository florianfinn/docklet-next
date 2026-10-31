// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original transport-neutral terminal lifetime boundary.

package ws

import (
	"context"
	"encoding/json/v2"
	"time"

	"github.com/coder/websocket"
	containertypes "github.com/florianfinn/docklet-next/types/container"
)

// TerminalLifetime belongs to one accepted hub session, never a resource key.
type TerminalLifetime interface {
	SetCloser(func())
	Observe(string)
}
type terminalKey struct{}

func WithTerminal(ctx context.Context, handle TerminalLifetime) context.Context {
	return context.WithValue(ctx, terminalKey{}, handle)
}
func Terminal(ctx context.Context) TerminalLifetime {
	h, _ := ctx.Value(terminalKey{}).(TerminalLifetime)
	return h
}

// ObserveTerminal records only typed end metadata, never terminal content.
func ObserveTerminal(ctx context.Context, kind websocket.MessageType, data []byte) {
	if kind != websocket.MessageText {
		return
	}
	var exit containertypes.ShellExit
	if json.Unmarshal(data, &exit) == nil && exit.Type == "exit" {
		if h := Terminal(ctx); h != nil {
			h.Observe(exit.Reason)
		}
	}
}

func WriteTerminalExit(ctx context.Context, conn *websocket.Conn, code *int, reason string) {
	data, _ := json.Marshal(containertypes.ShellExit{Type: "exit", Code: code, Reason: reason})
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = conn.Write(writeCtx, websocket.MessageText, data)
	ObserveTerminal(ctx, websocket.MessageText, data)
}
