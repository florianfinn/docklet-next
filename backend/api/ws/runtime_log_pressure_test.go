// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original real WS backpressure and bounded raw/mapped source proofs.

package ws

import (
	"context"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/docker"
	"strings"
	"testing"
	"time"
)

func TestRuntimeLogsRealWSSlowReaderReleasesProducer(t *testing.T) {
	joined := make(chan struct{})
	h := &WebSocketHandler{containerLogStreamer: func(ctx context.Context, _ string, _ chan<- string, _ bool, _, _ string, _ bool) error {
		defer close(joined)
		sink := docker.LogSinkFromContext(ctx)
		record := docker.LogRecord{Message: strings.Repeat("x", 60000), Stream: "stdout"}
		for range 5000 {
			if err := sink(record); err != nil {
				return err
			}
		}
		<-ctx.Done()
		return ctx.Err()
	}}
	conn := dialLog(t, logServer(t, h, true)+"containers/pressure/logs?format=json")
	// No browser reads: the actual 256-entry client queue must remove this viewer.
	waitLogSignal(t, joined)
	require.Eventually(t, func() bool { h.logStreamsMu.Lock(); defer h.logStreamsMu.Unlock(); return len(h.logStreams) == 0 }, logTestTimeout, time.Millisecond)
	_ = conn.CloseNow()
}
func TestRuntimeLogsRawAndMappedQueuesCancelWhenFull(t *testing.T) {
	h := &WebSocketHandler{logStreams: make(map[string]*wsLogStream)}
	ls, ctx := newWSLogStreamInternal("full", "json")
	cancel := ls.cancel
	defer cancel()
	close(ls.firstSubscriber)
	started := make(chan struct{})
	joined := make(chan struct{})
	lines := h.startLogSourceInternal(ctx, "full", "example", "container", logStreamParams{}, func(ctx context.Context, _ string, _ chan<- string, _ bool, _, _ string, _ bool) error {
		defer close(joined)
		close(started)
		sink := docker.LogSinkFromContext(ctx)
		for {
			if err := sink(docker.LogRecord{Message: "bounded", Stream: "stdout"}); err != nil {
				return err
			}
		}
	}, ls)
	waitLogSignal(t, started)
	require.Equal(t, 256, cap(lines))
	mapped := mapLogLinesInternal(ctx, lines, func(line sourceLogLine) wshub.LogMessage {
		return normalizeSourceLogLine(line, normalizeContainerLogMessageInternal)
	})
	require.Equal(t, 256, cap(mapped))
	require.Eventually(t, func() bool { return len(lines) == 256 && len(mapped) == 256 }, logTestTimeout, time.Millisecond)
	cancel()
	waitLogSignal(t, joined)
	for range mapped {
	}
}
