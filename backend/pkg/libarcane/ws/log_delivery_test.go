// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original forwarder wire metadata and full-queue loss assertions.

package ws

import (
	"context"
	"encoding/json/v2"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLogDeliveryFullBroadcastQueueExposesNextSeqOverWS(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	hub := NewHub(1024)
	input := make(chan LogMessage, 256)
	forwarded := make(chan struct{})
	go func() { defer close(forwarded); ForwardLogJSON(ctx, hub, input) }()
	for seq := uint64(1); seq <= 1100; seq++ {
		input <- LogMessage{Seq: seq, Message: "bounded", Stream: "stdout", Timestamp: NowRFC3339()}
	}
	close(input)
	<-forwarded
	require.Equal(t, 1024, cap(hub.broadcast))
	require.Equal(t, 1024, len(hub.broadcast))
	serverDone := make(chan error, 1)
	// Drain the frozen production broadcast queue over real frames. A separate
	// endpoint test covers actual Hub/client slow-reader removal and source join.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.CloseNow()
		require.Equal(t, 256, cap(NewClient(conn, clientSendBuffer).send))
		for range 1024 {
			if err = conn.Write(ctx, websocket.MessageText, <-hub.broadcast); err != nil {
				serverDone <- err
				return
			}
		}
		next := make(chan LogMessage, 1)
		next <- LogMessage{Seq: 1101, Message: "next", Stream: "stderr", Truncated: true, Timestamp: NowRFC3339()}
		close(next)
		ForwardLogJSON(ctx, hub, next)
		serverDone <- conn.Write(ctx, websocket.MessageText, <-hub.broadcast)
	}))
	defer server.Close()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.CloseNow()
	var previous uint64
	for range 1025 {
		_, data, err := conn.Read(ctx)
		require.NoError(t, err)
		var msg LogMessage
		require.NoError(t, json.Unmarshal(data, &msg))
		if previous < 1024 {
			require.Equal(t, previous+1, msg.Seq)
		} else {
			require.Equal(t, uint64(1101), msg.Seq)
			require.True(t, msg.Truncated)
			require.Equal(t, "stderr", msg.Stream)
		}
		previous = msg.Seq
	}
	require.NoError(t, <-serverDone)
}
func TestLogDeliveryBatchRemainsFiftyAndOptionalMetadataOmitted(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	hub := NewHub(1024)
	input := make(chan LogMessage, 51)
	for seq := uint64(1); seq <= 51; seq++ {
		input <- LogMessage{Seq: seq, Message: "text", Timestamp: NowRFC3339()}
	}
	close(input)
	ForwardLogJSONBatched(ctx, hub, input, 50, time.Hour)
	require.Len(t, hub.broadcast, 2)
	for _, size := range []int{50, 1} {
		frame := <-hub.broadcast
		var messages []map[string]any
		require.NoError(t, json.Unmarshal(frame, &messages))
		require.Len(t, messages, size)
		for _, message := range messages {
			require.NotContains(t, message, "stream")
			require.NotContains(t, message, "truncated")
		}
	}
}
