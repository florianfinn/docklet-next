// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original terminal bridge with bounded agent exit draining.

package ws

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
)

type terminalFrame struct {
	kind websocket.MessageType
	data []byte
	err  error
}

func proxyTerminalHTTP(w http.ResponseWriter, r *http.Request, address string, headers http.Header, origin func(*http.Request) bool) error {
	if origin == nil {
		return errors.New("websocket proxy requires an origin validator")
	}
	browser, err := Accept(w, r, origin)
	if err != nil {
		return err
	}
	defer browser.CloseNow()
	relay := NewTerminalRelay(r.Context(), browser, true)
	defer relay.Finish(websocket.StatusNormalClosure, "")
	viewer, stopViewer := context.WithCancel(r.Context())
	defer stopViewer()
	lease, err := sessionlife.Register(viewer, stopViewer)
	if err != nil {
		return err
	}
	defer lease.Release()
	dial, cancelDial := context.WithTimeout(viewer, 45*time.Second)
	agent, response, err := websocket.Dial(dial, address, &websocket.DialOptions{HTTPHeader: headers, HTTPClient: &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}}})
	cancelDial()
	if response != nil && response.Body != nil {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	if err != nil {
		return err
	}
	defer agent.CloseNow()
	browser.SetReadLimit(-1)
	agent.SetReadLimit(-1)
	pump, cancelPump := context.WithCancel(context.WithoutCancel(viewer))
	browserDone, agentDone := make(chan struct{}), make(chan struct{})
	browserEnd := make(chan bool, 1)
	frames := make(chan terminalFrame)
	go func() {
		defer close(browserDone)
		for {
			kind, data, err := browser.Read(pump)
			if err != nil {
				browserEnd <- true
				return
			}
			// A revoked viewer may finish the close protocol but never sends new input to the agent.
			if viewer.Err() != nil && !IsTerminalClose(kind, data) {
				continue
			}
			writeCtx, cancel := context.WithTimeout(pump, 5*time.Second)
			err = agent.Write(writeCtx, kind, data)
			cancel()
			if err != nil {
				browserEnd <- false
				return
			}
			if IsTerminalClose(kind, data) {
				browserEnd <- false
				return
			}
		}
	}()
	go func() {
		defer close(agentDone)
		for {
			kind, data, err := agent.Read(pump)
			select {
			case frames <- terminalFrame{kind, data, err}:
			case <-pump.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	defer func() {
		relay.Finish(websocket.StatusNormalClosure, "")
		cancelPump()
		_ = browser.CloseNow()
		_ = agent.CloseNow()
		<-browserDone
		<-agentDone
	}()
	ticker := sessionlife.NewTicker(pump, 54*time.Second)
	defer ticker.Stop()
	var deadline <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	closing := false
	beginClose := func(send bool) {
		if closing {
			return
		}
		closing = true
		timer = time.NewTimer(5 * time.Second)
		deadline = timer.C
		if send {
			ctx, cancel := context.WithTimeout(pump, time.Second)
			_ = agent.Write(ctx, websocket.MessageText, TerminalCloseControl)
			cancel()
		}
	}
	viewerDone := viewer.Done()
	for {
		select {
		case gone := <-browserEnd:
			if gone {
				relay.BrowserClosed()
			}
			beginClose(gone)
			browserEnd = nil
		case <-viewerDone:
			beginClose(true)
			viewerDone = nil
		case <-deadline:
			return nil
		case <-ticker.C:
			if sessionlife.Check(viewer) != nil {
				beginClose(true)
			}
		case frame := <-frames:
			if frame.err != nil {
				var closed websocket.CloseError
				if errors.As(frame.err, &closed) {
					relay.Finish(closed.Code, closed.Reason)
				}
				return nil
			}
			if err := relay.Write(pump, frame.kind, frame.data); err != nil {
				relay.BrowserClosed()
				beginClose(true)
			}
			if closing && relay.Ended() {
				return nil
			}
		}
	}
}
