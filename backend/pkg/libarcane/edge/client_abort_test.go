// Source: docklet-next/issues/330
// Revision: a37d77d05c92bde3a4eab0cf6fa47ab36411e011
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original regression for aborted local handlers on tunneled requests (M16 acceptance DV1 D).

package edge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// An aborted handler gets no response, as with net/http, instead of crashing the agent; other panics still propagate
// (shared by rows A3, A4, A5; row A8).
func TestServeLocalTreatsAbortHandlerAsNoResponse(t *testing.T) {
	aborting := &TunnelClient{handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })}
	require.True(t, aborting.serveLocalInternal(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)))

	serving := &TunnelClient{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })}
	rec := httptest.NewRecorder()
	require.False(t, serving.serveLocalInternal(rec, httptest.NewRequest(http.MethodGet, "/", nil)))
	require.Equal(t, http.StatusNoContent, rec.Code)

	failure := errors.New("example failure")
	failing := &TunnelClient{handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(failure) })}
	require.PanicsWithValue(t, failure, func() {
		failing.serveLocalInternal(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})
}

type abortTestConn struct{ sent []*TunnelMessage }

func (c *abortTestConn) Send(m *TunnelMessage) error {
	sent := *m
	c.sent = append(c.sent, &sent)
	return nil
}
func (c *abortTestConn) Receive() (*TunnelMessage, error)  { return nil, nil }
func (c *abortTestConn) IsExpectedReceiveError(error) bool { return false }
func (c *abortTestConn) Close() error                      { return nil }
func (c *abortTestConn) IsClosed() bool                    { return false }
func (c *abortTestConn) Transport() string                 { return EdgeTransportWebSocket }

// Each dispatcher that runs the router sends nothing for an aborted handler except the command acknowledgement that
// precedes it, and still answers a served request (class rows A3, A4, A5).
func TestDispatchersSendNothingForAbortedHandler(t *testing.T) {
	for _, path := range []string{"request", "streaming", "command"} {
		for _, abort := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/abort=%t", path, abort), func(t *testing.T) {
				handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if abort {
						panic(http.ErrAbortHandler)
					}
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"success":true}`))
				})
				client := NewTunnelClient(&Config{}, handler)
				conn := &abortTestConn{}
				msg := &TunnelMessage{ID: "example", Method: http.MethodGet, Path: "/api/environments/0/images", Command: "image.list"}
				switch path {
				case "request":
					client.handleRequest(context.Background(), conn, msg)
				case "streaming":
					client.handleRequestStreaming(context.Background(), conn, msg)
				case "command":
					client.handleCommandRequest(context.Background(), conn, msg)
				}
				types := make([]TunnelMessageType, 0, len(conn.sent))
				for _, m := range conn.sent {
					types = append(types, m.Type)
				}
				switch {
				case abort && path == "command":
					require.Equal(t, []TunnelMessageType{MessageTypeCommandAck}, types)
				case abort:
					require.Empty(t, types)
				default:
					require.NotEmpty(t, types)
					require.NotEqual(t, []TunnelMessageType{MessageTypeCommandAck}, types)
				}
			})
		}
	}
}
