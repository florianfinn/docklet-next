// Source: getarcaneapp/arcane/backend/pkg/libarcane/ws/proxy.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Log only sorted forwarded header names; session-bound viewer lifetime and durable validation; terminal exit delivery before cancellation.

package ws

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
)

// ProxyHTTP upgrades the incoming client connection and bridges it to remoteWS.
//
// checkOrigin must be the same Origin validator the local WebSocket endpoints
// use. It is required: this upgrade is reached with the caller's session cookie
// already validated, so accepting any Origin would let an attacker-controlled
// page open a terminal or log stream in a remote environment.
func ProxyHTTP(w http.ResponseWriter, r *http.Request, remoteWS string, header http.Header, checkOrigin func(*http.Request) bool) error {
	if checkOrigin == nil {
		return errors.New("websocket proxy requires an origin validator")
	}

	clientConn, err := Accept(w, r, checkOrigin)
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to upgrade client connection", "remoteWs", remoteWS, "err", err)
		return err
	}
	defer func() { _ = clientConn.CloseNow() }()
	terminal := IsTerminalRequest(r)
	relay := NewTerminalRelay(r.Context(), clientConn, terminal)
	if terminal {
		defer relay.Finish(websocket.StatusPolicyViolation, "container.shell-unavailable")
	}
	viewerCtx, viewerCancel := context.WithCancel(r.Context())
	defer viewerCancel()
	lease, err := sessionlife.Register(viewerCtx, func() {
		viewerCancel()
		if !terminal {
			_ = clientConn.CloseNow()
		}
	})
	if err != nil {
		return err
	}
	defer lease.Release()
	// This is a pure bridge; frame-size policing is the remote endpoint's job.
	clientConn.SetReadLimit(-1)

	headerNames := make([]string, 0, len(header))
	for name := range header {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	slog.DebugContext(r.Context(), "attempting websocket dial", "remoteWs", remoteWS, "headerNames", headerNames)
	dialCtx, dialCancel := context.WithTimeout(viewerCtx, 45*time.Second)
	remoteConn, resp, err := websocket.Dial(dialCtx, remoteWS, &websocket.DialOptions{
		HTTPHeader: header,
		HTTPClient: &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}},
	})
	dialCancel()
	// Ensure the response body is drained & closed to avoid leaking resources.
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	if err != nil {
		slog.ErrorContext(dialCtx, "failed to dial remote websocket", "remoteWs", remoteWS, "err", err, "respStatus", func() int {
			if resp != nil {
				return resp.StatusCode
			}
			return 0
		}())
		relay.Finish(websocket.StatusBadGateway, "container.shell-unavailable")
		return err
	}
	defer func() { _ = remoteConn.CloseNow() }()
	remoteConn.SetReadLimit(-1)

	slog.DebugContext(dialCtx, "websocket proxy established", "remoteWs", remoteWS)

	// When either pump ends, close both legs and join the pumps and durable
	// cadence before returning from this viewer lifetime.
	pumpParent := viewerCtx
	if terminal {
		pumpParent = context.WithoutCancel(viewerCtx)
	}
	pumpCtx, pumpCancel := context.WithCancel(pumpParent)
	defer pumpCancel()

	errc := make(chan struct{}, 2)

	// client -> remote
	go func() {
		defer func() { errc <- struct{}{} }()
		for {
			mt, msg, readErr := clientConn.Read(pumpCtx)
			if readErr != nil {
				relayCloseInternal(r.Context(), remoteConn, readErr)
				return
			}
			if writeErr := remoteConn.Write(pumpCtx, mt, msg); writeErr != nil {
				return
			}
		}
	}()

	// remote -> client
	go func() {
		defer func() { errc <- struct{}{} }()
		for {
			mt, msg, readErr := remoteConn.Read(pumpCtx)
			if readErr != nil {
				var closeError websocket.CloseError
				if errors.As(readErr, &closeError) {
					relay.Finish(closeError.Code, closeError.Reason)
				} else if terminal {
					relay.Finish(websocket.StatusPolicyViolation, "container.shell-unavailable")
				}
				return
			}
			if writeErr := relay.Write(pumpCtx, mt, msg); writeErr != nil {
				return
			}
		}
	}()

	checkDone := make(chan struct{})
	go func() {
		defer close(checkDone)
		ticker := sessionlife.NewTicker(pumpCtx, 54*time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-pumpCtx.Done():
				return
			case <-ticker.C:
				if sessionlife.Check(pumpCtx) != nil {
					viewerCancel()
					if !terminal {
						_ = clientConn.CloseNow()
						_ = remoteConn.CloseNow()
					}
					return
				}
			}
		}
	}()
	select {
	case <-errc:
	case <-viewerCtx.Done():
		if terminal {
			relay.Finish(websocket.StatusPolicyViolation, "container.shell-unavailable")
		}
		pumpCancel()
		_ = clientConn.CloseNow()
		_ = remoteConn.CloseNow()
		<-errc
	}
	if terminal {
		relay.Finish(websocket.StatusPolicyViolation, "container.shell-unavailable")
	}
	pumpCancel()
	_ = clientConn.CloseNow()
	_ = remoteConn.CloseNow()
	<-errc
	<-checkDone
	return nil
}

// relayCloseInternal forwards a peer's close status to the other side of the
// bridge so the terminating reason survives the proxy hop.
func relayCloseInternal(ctx context.Context, conn *websocket.Conn, err error) {
	var ce websocket.CloseError
	if !errors.As(err, &ce) {
		return
	}
	code := ce.Code
	if code == websocket.StatusNoStatusRcvd {
		code = websocket.StatusNormalClosure
	}
	if closeErr := conn.Close(code, ce.Reason); closeErr != nil {
		slog.DebugContext(ctx, "failed to relay websocket close", "status", int(code), "err", closeErr)
	}
}
