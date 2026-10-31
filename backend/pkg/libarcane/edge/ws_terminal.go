// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original bounded Edge terminal exit drain.

package edge

import (
	"context"
	"github.com/coder/websocket"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"time"
)

func forwardTerminalAgentToClient(viewer, stream context.Context, browser *websocket.Conn, tunnel *AgentTunnel, pending *PendingRequest, id string, browserDone <-chan struct{}) {
	relay := wshub.Relay(viewer)
	ticker := sessionlife.NewTicker(stream, tunnelWSPingPeriod)
	defer ticker.Stop()
	var deadline <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
		sendWebSocketClose(tunnel, id)
	}()
	beginClose := func(send bool) {
		if timer != nil {
			return
		}
		timer = time.NewTimer(5 * time.Second)
		deadline = timer.C
		if send {
			_ = sendStreamDataInternal(tunnel, id, int(websocket.MessageText), wshub.TerminalCloseControl)
		}
	}
	viewerDone := viewer.Done()
	for {
		select {
		case <-stream.Done():
			return
		case <-tunnel.done:
			return
		case <-pending.failureCh:
			return
		case <-viewerDone:
			beginClose(true)
			viewerDone = nil
		case <-browserDone:
			// The close control was already sent by the browser pump. Its socket may
			// have closed, but the agent stream remains open for the final exit.
			beginClose(false)
			browserDone = nil
		case <-deadline:
			return
		case <-ticker.C:
			if sessionlife.Check(viewer) != nil {
				beginClose(true)
			}
			if timer == nil {
				pingCtx, cancel := context.WithTimeout(stream, tunnelWSPingWait)
				err := browser.Ping(pingCtx)
				cancel()
				if err != nil {
					relay.BrowserClosed()
					beginClose(true)
				}
			}
		case msg, ok := <-pending.ResponseCh:
			if !ok {
				return
			}
			switch msg.Type {
			case MessageTypeWebSocketData, MessageTypeStreamData:
				if err := writeWebSocketData(stream, browser, msg); err != nil {
					relay.BrowserClosed()
					beginClose(true)
				}
				if timer != nil && relay.Ended() {
					return
				}
			case MessageTypeWebSocketClose, MessageTypeStreamClose, MessageTypeStreamEnd:
				status, reason := decodeStreamClose(msg.Error)
				relay.Finish(status, reason)
				return
			}
		}
	}
}
