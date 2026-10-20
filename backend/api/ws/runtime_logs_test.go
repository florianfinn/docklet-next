// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original real WebSocket log source and lifecycle regressions.

package ws

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/docker"
)

type logSourceOptions struct {
	resource, tail, since string
	follow, timestamps    bool
}

func logServer(t *testing.T, h *WebSocketHandler, allow bool) string {
	t.Helper()
	h.logStreams = make(map[string]*wsLogStream)
	h.wsMetrics = wshub.NewWebSocketMetrics()
	h.checkWSOrigin = func(r *http.Request) bool { return r.Header.Get("Origin") != "https://denied.example.com" }
	router := echo.New()
	group := router.Group("/api/environments/:id/ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			p := authz.NewPermissionSet()
			if allow {
				p = authz.SudoPermissionSet()
			}
			c.Set(string(middleware.ContextKeyUserPermissions), p)
			return next(c)
		}
	})
	for _, route := range h.proxiedRoutes() {
		group.GET(route.path, route.handler, middleware.RequireEchoPermission(route.perm))
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http") + "/api/environments/0/ws/"
}
func dialLog(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, nil)
	require.NoError(t, err)
	conn.SetReadLimit(4 << 20)
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}
func readLog(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	require.NoError(t, err)
	return data
}
func waitLogSignal(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("source did not settle")
	}
}

func TestRuntimeLogsRealWSSharedSourceOptionsAndLastRef(t *testing.T) {
	var starts atomic.Int32
	trigger := make(chan struct{})
	joined := make(chan struct{})
	observed := make(chan logSourceOptions, 1)
	h := &WebSocketHandler{}
	h.containerLogStreamer = func(ctx context.Context, id string, out chan<- string, follow bool, tail, since string, timestamps bool) error {
		starts.Add(1)
		observed <- logSourceOptions{id, tail, since, follow, timestamps}
		defer close(joined)
		select {
		case <-trigger:
		case <-ctx.Done():
			return ctx.Err()
		}
		if err := docker.StreamContainerLogs(ctx, io.NopCloser(strings.NewReader("2026-01-02T03:04:05Z  text stays  \n")), out, follow, true); err != nil {
			return err
		}
		<-ctx.Done()
		return ctx.Err()
	}
	url := logServer(t, h, true) + "containers/example%20container/logs?format=json&follow=true&tail=100&timestamps=true&batched=false"
	one := dialLog(t, url)
	two := dialLog(t, url)
	var opts logSourceOptions
	select {
	case opts = <-observed:
	case <-time.After(5 * time.Second):
		t.Fatal("source not started")
	}
	require.Equal(t, logSourceOptions{"example container", "100", "", true, true}, opts)
	require.Eventually(t, func() bool {
		h.logStreamsMu.Lock()
		defer h.logStreamsMu.Unlock()
		for _, s := range h.logStreams {
			if s.refs == 2 {
				return true
			}
		}
		return false
	}, 5*time.Second, time.Millisecond)
	close(trigger)
	for _, conn := range []*websocket.Conn{one, two} {
		var msg wshub.LogMessage
		require.NoError(t, json.Unmarshal(readLog(t, conn), &msg))
		require.Equal(t, uint64(1), msg.Seq)
		require.Equal(t, "combined", msg.Stream)
		require.Equal(t, "info", msg.Level)
		require.Equal(t, " text stays  ", msg.Message)
		require.Equal(t, "example container", msg.ContainerID)
		require.Equal(t, "2026-01-02T03:04:05Z", msg.Timestamp)
	}
	require.Equal(t, int32(1), starts.Load())
	_ = one.CloseNow()
	require.Eventually(t, func() bool {
		h.logStreamsMu.Lock()
		defer h.logStreamsMu.Unlock()
		for _, s := range h.logStreams {
			if s.refs == 1 {
				return true
			}
		}
		return false
	}, 5*time.Second, time.Millisecond)
	select {
	case <-joined:
		t.Fatal("first unsubscribe cancelled the shared producer")
	default:
	}
	_ = two.CloseNow()
	waitLogSignal(t, joined)
	require.Eventually(t, func() bool { h.logStreamsMu.Lock(); defer h.logStreamsMu.Unlock(); return len(h.logStreams) == 0 }, 5*time.Second, time.Millisecond)
}

func TestRuntimeLogsRealWSFormatsAndSourceMetadata(t *testing.T) {
	for _, format := range []string{"text", "json", "batch"} {
		t.Run(format, func(t *testing.T) {
			h := &WebSocketHandler{}
			h.projectLogStreamer = func(ctx context.Context, _ string, out chan<- string, _ bool, _, _ string, _ bool) error {
				// Legacy sources remain compatible, including text and existing JSON arrays.
				for _, line := range []string{"web | 2026-01-02T03:04:06Z same", "worker | [STDERR] 2026-01-02T03:04:05Z same"} {
					select {
					case out <- line:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return nil
			}
			query := ""
			if format != "text" {
				query = "?format=json"
			}
			if format == "batch" {
				query += "&batched=true"
			}
			conn := dialLog(t, logServer(t, h, true)+"projects/example/logs"+query)
			if format == "text" {
				require.Equal(t, "same", string(readLog(t, conn)))
				require.Equal(t, "same", string(readLog(t, conn)))
				return
			}
			var messages []wshub.LogMessage
			if format == "batch" {
				require.NoError(t, json.Unmarshal(readLog(t, conn), &messages))
			} else {
				for range 2 {
					var m wshub.LogMessage
					require.NoError(t, json.Unmarshal(readLog(t, conn), &m))
					messages = append(messages, m)
				}
			}
			require.Len(t, messages, 2)
			require.Equal(t, "web", messages[0].Service)
			require.Equal(t, "worker", messages[1].Service)
			require.Equal(t, "info", messages[1].Level)
			require.Equal(t, "unknown", messages[1].Stream)
			require.Equal(t, uint64(1), messages[0].Seq)
			require.Equal(t, uint64(2), messages[1].Seq)
			require.Greater(t, messages[0].Timestamp, messages[1].Timestamp, "arrival order is not timestamp order")
		})
	}
}
func TestRuntimeLogsAllSourceOptionsParticipateInKey(t *testing.T) {
	base := logStreamParams{format: "json", follow: true, tail: "100", timestamps: true}
	key := func(env, kind, id string, p logStreamParams) string {
		return buildLogStreamKeyInternal(env, kind, id, p.format, p.batched, p.follow, p.tail, p.since, p.timestamps)
	}
	original := key("0", "project", "example", base)
	for _, change := range []func(*logStreamParams){func(p *logStreamParams) { p.format = "text" }, func(p *logStreamParams) { p.follow = false }, func(p *logStreamParams) { p.tail = "10" }, func(p *logStreamParams) { p.since = "example" }, func(p *logStreamParams) { p.timestamps = false }, func(p *logStreamParams) { p.batched = true }} {
		p := base
		change(&p)
		require.NotEqual(t, original, key("0", "project", "example", p))
	}
	require.NotEqual(t, original, key("remote", "project", "example", base))
	require.NotEqual(t, original, key("0", "container", "example", base))
	require.NotEqual(t, original, key("0", "project", "other", base))
}
func TestRuntimeLogsSourceKeySeparatesDelimitedOptions(t *testing.T) {
	one := buildLogStreamKeyInternal("0", "project", "example", "json", false, true, "100|extra", "", true)
	two := buildLogStreamKeyInternal("0", "project", "example", "json", false, true, "100", "extra|", true)
	require.NotEqual(t, one, two)
}

func TestRuntimeLogsDeniedPermissionAndOriginStartNoSource(t *testing.T) {
	for _, permission := range []bool{false, true} {
		t.Run(map[bool]string{false: "permission", true: "origin"}[permission], func(t *testing.T) {
			var starts atomic.Int32
			h := &WebSocketHandler{containerLogStreamer: func(context.Context, string, chan<- string, bool, string, string, bool) error {
				starts.Add(1)
				return nil
			}}
			opts := &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"https://denied.example.com"}}}
			conn, response, err := websocket.Dial(t.Context(), logServer(t, h, permission)+"containers/example/logs", opts)
			if conn != nil {
				_ = conn.CloseNow()
			}
			require.Error(t, err)
			require.Equal(t, http.StatusForbidden, response.StatusCode)
			require.Zero(t, starts.Load())
		})
	}
}
