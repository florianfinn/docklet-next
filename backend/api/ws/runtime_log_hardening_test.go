// Source: docklet-next/docs/design/runtime-control.md
// Revision: ed78cd8cf48b88e8225b656b0976d248829f85eb
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original real WS structured stderr, late viewer and warning regressions.

package ws

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/docker"
)

func hardeningLogFrame(stream byte, text string) []byte {
	var header [8]byte
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(text)))
	return append(header[:], []byte(text)...)
}

func TestRuntimeLogsRealWSStructuredStderr(t *testing.T) {
	const message = "  [STDERR] application ERROR text  "
	for _, kind := range []string{"container", "compose"} {
		t.Run(kind, func(t *testing.T) {
			h := &WebSocketHandler{}
			frame := hardeningLogFrame(2, "2026-01-02T03:04:05.123456789Z "+message+"\n")
			route := "containers/example/logs?format=json&follow=false&timestamps=true"
			if kind == "container" {
				h.containerLogStreamer = func(ctx context.Context, _ string, out chan<- string, follow bool, _, _ string, _ bool) error {
					return docker.StreamContainerLogs(ctx, io.NopCloser(bytes.NewReader(frame)), out, follow, false)
				}
			} else {
				daemon := composeHardeningDaemon(t, frame)
				t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(daemon.URL, "http://"))
				h.projectLogStreamer = func(ctx context.Context, _ string, _ chan<- string, follow bool, tail, since string, timestamps bool) error {
					return projects.ComposeLogs(ctx, "example", io.Discard, follow, tail, since, timestamps)
				}
				route = "projects/example/logs?format=json&follow=false&timestamps=true"
			}
			conn := dialLog(t, logServer(t, h, true)+route)
			var got wshub.LogMessage
			require.NoError(t, json.Unmarshal(readLog(t, conn), &got))
			require.Equal(t, "stderr", got.Stream)
			require.Equal(t, "info", got.Level, "stderr carries identity, not error severity")
			require.Equal(t, message, got.Message)
			require.Equal(t, "2026-01-02T03:04:05.123456789Z", got.Timestamp)
			if kind == "compose" {
				require.Equal(t, "web", got.Service)
			} else {
				require.Equal(t, "example", got.ContainerID)
			}
		})
	}
}

func composeHardeningDaemon(t *testing.T, frame []byte) *httptest.Server {
	t.Helper()
	const id = "11112222333344445555666677778888"
	version := regexp.MustCompile(`^/v[0-9.]+`)
	var inspected atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch version.ReplaceAllString(r.URL.Path, "") {
		case "/_ping":
			w.Header().Set("Api-Version", "1.47")
		case "/containers/json":
			_, _ = w.Write([]byte(`[{"Id":"` + id + `","Names":["/example-web-1"],"Image":"example-image","State":"running","Labels":{"com.docker.compose.project":"example","com.docker.compose.service":"web","com.docker.compose.container-number":"1","com.docker.compose.oneoff":"False"}}]`))
		case "/containers/" + id + "/json":
			inspected.Store(true)
			_, _ = w.Write([]byte(`{"Id":"` + id + `","Name":"/example-web-1","Created":"2026-01-02T00:00:00Z","State":{"Running":true},"Config":{"Tty":false,"Image":"example-image","Labels":{"com.docker.compose.service":"web"}},"HostConfig":{},"NetworkSettings":{},"Mounts":[]}`))
		case "/containers/" + id + "/logs":
			if !inspected.Load() {
				t.Error("Compose opened logs before inspecting the same full container ID")
			}
			w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
			_, _ = w.Write(frame)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRuntimeLogsRealWSLateViewerReceivesOnlyFutureLines(t *testing.T) {
	var starts atomic.Int32
	next := make(chan string)
	joined := make(chan struct{})
	h := &WebSocketHandler{containerLogStreamer: func(ctx context.Context, _ string, _ chan<- string, _ bool, _, _ string, _ bool) error {
		starts.Add(1)
		defer close(joined)
		sink := docker.LogSinkFromContext(ctx)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case text := <-next:
				if err := sink(docker.LogRecord{Message: text, Stream: "stdout"}); err != nil {
					return err
				}
			}
		}
	}}
	url := logServer(t, h, true) + "containers/example/logs?format=json&tail=100"
	one := dialLog(t, url)
	send := func(text string) {
		t.Helper()
		select {
		case next <- text:
		case <-time.After(5 * time.Second):
			t.Fatal("source not consuming")
		}
	}
	send("initial tail")
	var initial wshub.LogMessage
	require.NoError(t, json.Unmarshal(readLog(t, one), &initial))
	require.Equal(t, uint64(1), initial.Seq)
	// A pong proves ServeClient's registration send completed. The hub finishes
	// that registration before it can process the next broadcast. Refcounts alone
	// advance earlier and cannot synchronize a late viewer's first delivery.
	readyRead := func(conn *websocket.Conn) func() []byte {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		t.Cleanup(cancel)
		type result struct {
			body []byte
			err  error
		}
		read := make(chan result, 1)
		go func() { _, body, err := conn.Read(ctx); read <- result{body, err} }()
		require.NoError(t, conn.Ping(ctx))
		return func() []byte { t.Helper(); got := <-read; cancel(); require.NoError(t, got.err); return got.body }
	}
	two := dialLog(t, url)
	nextTwo := readyRead(two)
	waitRefs := func(want int) {
		t.Helper()
		require.Eventually(t, func() bool {
			h.logStreamsMu.Lock()
			defer h.logStreamsMu.Unlock()
			for _, source := range h.logStreams {
				if source.refs == want {
					return true
				}
			}
			return false
		}, 5*time.Second, time.Millisecond)
	}
	waitRefs(2)
	send("future line")
	for _, data := range [][]byte{readLog(t, one), nextTwo()} {
		var got wshub.LogMessage
		require.NoError(t, json.Unmarshal(data, &got))
		require.Equal(t, "future line", got.Message)
		require.Equal(t, uint64(2), got.Seq, "late viewer must not receive the old tail")
	}
	_ = two.CloseNow()
	waitRefs(1)
	two = dialLog(t, url)
	nextTwo = readyRead(two)
	waitRefs(2)
	send("after reconnect")
	var got wshub.LogMessage
	require.NoError(t, json.Unmarshal(nextTwo(), &got))
	require.Equal(t, "after reconnect", got.Message)
	require.Equal(t, uint64(3), got.Seq)
	require.Equal(t, int32(1), starts.Load())
	_ = one.CloseNow()
	_ = two.CloseNow()
	waitLogSignal(t, joined)
}

type logWarningCapture struct{ records chan slog.Record }

func (h *logWarningCapture) Enabled(context.Context, slog.Level) bool { return true }
func (h *logWarningCapture) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn {
		h.records <- r.Clone()
	}
	return nil
}
func (h *logWarningCapture) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *logWarningCapture) WithGroup(string) slog.Handler      { return h }

func TestRuntimeLogSourceWarningBoundedAndCancellationQuiet(t *testing.T) {
	capture := &logWarningCapture{records: make(chan slog.Record, 4)}
	previous := slog.Default()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() { slog.SetDefault(previous) })
	for _, sourceErr := range []error{errors.New("private upstream body with credential=example"), context.Canceled, context.DeadlineExceeded} {
		h := &WebSocketHandler{logStreams: make(map[string]*wsLogStream)}
		ls, ctx := newWSLogStreamInternal("example", "json")
		ls.refs = 1 // Model the viewer that owns this source until forwarding completes.
		lines := h.startLogSourceInternal(ctx, "example", strings.Repeat("界", 300), "container", logStreamParams{}, func(context.Context, string, chan<- string, bool, string, string, bool) error { return sourceErr }, ls)
		close(ls.firstSubscriber)
		var failures int
		for line := range lines {
			if line.failure {
				failures++
			}
		}
		ls.cancel()
		if errors.Is(sourceErr, context.Canceled) || errors.Is(sourceErr, context.DeadlineExceeded) {
			require.Zero(t, failures)
			continue
		}
		require.Equal(t, 1, failures)
	}
	require.Len(t, capture.records, 1)
	record := <-capture.records
	require.Equal(t, "log source failed", record.Message)
	attrs := make(map[string]string)
	record.Attrs(func(a slog.Attr) bool { attrs[a.Key] = a.Value.String(); return true })
	require.Len(t, attrs, 2)
	require.Equal(t, "container", attrs["kind"])
	require.LessOrEqual(t, len(attrs["resourceId"]), 256)
	require.NotContains(t, attrs["resourceId"], "credential")
}
