// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original transport-neutral terminal lifetime boundary.

package ws

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	containertypes "github.com/florianfinn/docklet-next/types/container"
)

// TerminalLifetime belongs to one accepted hub session, never a resource key.
type TerminalLifetime interface {
	SetCloser(func())
	ObserveExit(containertypes.ShellExit)
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
			h.ObserveExit(exit)
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

// TerminalRelay serializes an upstream exit with cancellation at the hub. It
// emits a fallback only if the agent did not already supply an authoritative exit.
type TerminalRelay struct {
	mu          sync.Mutex
	ctx         context.Context
	conn        *websocket.Conn
	terminal    bool
	ended       bool
	exitReason  string
	closed      bool
	browserGone bool
}

func NewTerminalRelay(ctx context.Context, conn *websocket.Conn, terminal bool) *TerminalRelay {
	return &TerminalRelay{ctx: ctx, conn: conn, terminal: terminal}
}
func IsTerminalRequest(r *http.Request) bool {
	return strings.Contains(r.URL.Path, "/ws/containers/") && strings.HasSuffix(r.URL.Path, "/terminal")
}
func (r *TerminalRelay) Write(ctx context.Context, kind websocket.MessageType, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return io.EOF
	}
	// Audit the agent exit even after the browser has completed its close.
	if r.terminal && kind == websocket.MessageText {
		var exit containertypes.ShellExit
		if json.Unmarshal(data, &exit) == nil && exit.Type == "exit" {
			r.ended = true
			r.exitReason = exit.Reason
			ObserveTerminal(r.ctx, kind, data)
		}
	}
	if r.browserGone {
		return nil
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := r.conn.Write(writeCtx, kind, data)
	return err
}
func (r *TerminalRelay) Finish(status websocket.StatusCode, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	if r.ended && reason == "" {
		reason = r.exitReason
		if reason != "" {
			status = websocket.StatusPolicyViolation
		}
	}
	if r.terminal {
		if !r.ended {
			exitReason := reason
			switch reason {
			case "container.shell-protocol", "container.shell-unavailable", "container.shell-orphaned", "container.shell-not-running":
			default:
				exitReason = ""
			}
			if !r.browserGone {
				WriteTerminalExit(r.ctx, r.conn, nil, exitReason)
			}
		}
		if status != websocket.StatusNormalClosure && status != websocket.StatusPolicyViolation {
			status = websocket.StatusPolicyViolation
		}
	}
	if !r.browserGone {
		_ = r.conn.Close(status, reason)
	}
}

// BrowserClosed keeps agent output draining for the final audit, without writes
// to a socket whose WebSocket close handshake already completed.
func (r *TerminalRelay) BrowserClosed()   { r.mu.Lock(); r.browserGone = true; r.mu.Unlock() }
func (r *TerminalRelay) IsTerminal() bool { return r.terminal }
func (r *TerminalRelay) Ended() bool      { r.mu.Lock(); defer r.mu.Unlock(); return r.ended }

var TerminalCloseControl = []byte(`{"type":"close"}`)

func IsTerminalClose(kind websocket.MessageType, data []byte) bool {
	var control containertypes.ShellClose
	return kind == websocket.MessageText && json.Unmarshal(data, &control, json.RejectUnknownMembers(true)) == nil && control.Type == "close"
}

type relayKey struct{}

func WithTerminalRelay(ctx context.Context, relay *TerminalRelay) context.Context {
	return context.WithValue(ctx, relayKey{}, relay)
}
func Relay(ctx context.Context) *TerminalRelay {
	relay, _ := ctx.Value(relayKey{}).(*TerminalRelay)
	return relay
}
