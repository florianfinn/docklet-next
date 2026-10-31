// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original real WebSocket to Docker HTTP shell protocol tests.

package container

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

type shellDocker struct {
	running     atomic.Bool
	inspections atomic.Int32
	orphan      bool
	stopped     bool
	failure     string
	commands    chan []string
	resizes     chan string
	tick        chan time.Time
}

func (f *shellDocker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1.53")
	w.Header().Set("Content-Type", "application/json")
	switch {
	case path == "/containers/example/json":
		_, _ = fmt.Fprintf(w, `{"Id":"example","State":{"Running":%t}}`, !f.stopped)
	case path == "/containers/example/exec":
		var req struct{ Cmd []string }
		_ = json.UnmarshalRead(r.Body, &req)
		f.commands <- req.Cmd
		if f.failure == "create" {
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `{"message":"synthetic create failure"}`)
			return
		}
		_, _ = io.WriteString(w, `{"Id":"example-exec"}`)
	case path == "/exec/example-exec/start":
		if f.failure == "attach" {
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `{"message":"synthetic attach failure"}`)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = rw.Flush()
		_, _ = conn.Write([]byte("ready"))
		buffer := make([]byte, 4096)
		for {
			n, err := conn.Read(buffer)
			if n > 0 {
				if strings.Contains(string(buffer[:n]), "exit\n") {
					if f.orphan {
						f.tick <- time.Now()
					} else {
						f.running.Store(false)
						_, _ = conn.Write([]byte("bye"))
						f.tick <- time.Now()
						return
					}
				}
				_, _ = conn.Write(buffer[:n])
			}
			if err != nil {
				return
			}
		}
	case path == "/exec/example-exec/resize":
		f.resizes <- r.URL.RawQuery
	case path == "/exec/example-exec/json":
		f.inspections.Add(1)
		_, _ = fmt.Fprintf(w, `{"ID":"example-exec","Running":%t,"ExitCode":23}`, f.running.Load())
	default:
		w.WriteHeader(404)
	}
}
func shellFixture(t *testing.T, fixture *shellDocker, query dto.ShellQuery) (*websocket.Conn, <-chan struct{}) {
	t.Helper()
	fixture.running.Store(true)
	fixture.tick = make(chan time.Time, 1)
	fixture.commands = make(chan []string, 4)
	fixture.resizes = make(chan string, 4)
	dockerHTTP := httptest.NewServer(fixture)
	t.Cleanup(dockerHTTP.Close)
	cli, err := client.New(client.WithHost(strings.Replace(dockerHTTP.URL, "http://", "tcp://", 1)), client.WithAPIVersion("1.53"), client.WithHTTPClient(dockerHTTP.Client()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	svc := &ContainerService{dockerService: (&docker.DockerClientService{}).WithClient(cli)}
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		conn, err := wshub.Accept(w, r, func(*http.Request) bool { return true })
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := sessionlife.WithCadence(r.Context(), func(time.Duration, bool) sessionlife.Cadence {
			tick := fixture.tick
			return sessionlife.Cadence{C: tick, Stop: func() {}}
		})
		svc.ServeTerminal(ctx, conn, "example", query)
	}))
	t.Cleanup(server.Close)
	conn, _, err := websocket.Dial(t.Context(), strings.Replace(server.URL, "http", "ws", 1), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn, done
}
func readShell(t *testing.T, conn *websocket.Conn) (websocket.MessageType, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	kind, data, err := conn.Read(ctx)
	require.NoError(t, err)
	return kind, data
}
func readShellExit(t *testing.T, conn *websocket.Conn) dto.ShellExit {
	t.Helper()
	for {
		kind, data := readShell(t, conn)
		if kind == websocket.MessageText {
			var exit dto.ShellExit
			require.NoError(t, json.Unmarshal(data, &exit))
			require.Equal(t, "exit", exit.Type)
			return exit
		}
	}
}
func TestShellResizeRealWebSocketDockerHTTP(t *testing.T) {
	f := &shellDocker{}
	conn, done := shellFixture(t, f, dto.ShellQuery{Shell: "auto"})
	kind, data := readShell(t, conn)
	require.Equal(t, websocket.MessageBinary, kind)
	require.Equal(t, "ready", string(data))
	require.NoError(t, conn.Write(t.Context(), websocket.MessageBinary, []byte("hello")))
	kind, data = readShell(t, conn)
	require.Equal(t, websocket.MessageBinary, kind)
	require.Equal(t, "hello", string(data))
	require.NoError(t, conn.Write(t.Context(), websocket.MessageText, []byte(`{"type":"resize","cols":132,"rows":43}`)))
	require.NoError(t, conn.Write(t.Context(), websocket.MessageBinary, []byte("barrier")))
	_, data = readShell(t, conn)
	require.Equal(t, "barrier", string(data))
	select {
	case size := <-f.resizes:
		require.Equal(t, "h=43&w=132", size)
	default:
		t.Fatal("resize was not applied before subsequent stdin")
	}
	require.NoError(t, conn.Write(t.Context(), websocket.MessageText, []byte(`{"type":"resize","cols":0,"rows":43}`)))
	exit := readShellExit(t, conn)
	require.Equal(t, "container.shell-protocol", exit.Reason)
	_, _, err := conn.Read(t.Context())
	require.Equal(t, websocket.StatusPolicyViolation, websocket.CloseStatus(err))
	<-done
	require.GreaterOrEqual(t, f.inspections.Load(), int32(2))
}
func TestShellEarlyFailuresAndEOF(t *testing.T) {
	for _, failure := range []string{"create", "attach", "stopped", "EOF"} {
		t.Run(failure, func(t *testing.T) {
			f := &shellDocker{failure: failure, stopped: failure == "stopped"}
			conn, done := shellFixture(t, f, dto.ShellQuery{Shell: "sh"})
			want := "container.shell-unavailable"
			if failure == "stopped" {
				want = "container.shell-not-running"
			}
			if failure == "EOF" {
				readShell(t, conn)
				require.NoError(t, conn.Write(t.Context(), websocket.MessageBinary, []byte("exit\n")))
				want = ""
			}
			exit := readShellExit(t, conn)
			require.Equal(t, want, exit.Reason)
			if failure == "EOF" {
				require.NotNil(t, exit.Code)
				require.Equal(t, 23, *exit.Code)
			}
			_, _, err := conn.Read(t.Context())
			status := websocket.StatusPolicyViolation
			if want == "" {
				status = websocket.StatusNormalClosure
			}
			require.Equal(t, status, websocket.CloseStatus(err))
			<-done
		})
	}
}
func TestShellOrphanInspectedAndCleanupJoined(t *testing.T) {
	f := &shellDocker{orphan: true}
	conn, done := shellFixture(t, f, dto.ShellQuery{Shell: "bash"})
	readShell(t, conn)
	require.NoError(t, conn.Write(t.Context(), websocket.MessageText, []byte(`{"type":"unexpected"}`)))
	exit := readShellExit(t, conn)
	require.Equal(t, "container.shell-orphaned", exit.Reason)
	_, _, err := conn.Read(t.Context())
	require.Equal(t, websocket.StatusPolicyViolation, websocket.CloseStatus(err))
	<-done
	require.GreaterOrEqual(t, f.inspections.Load(), int32(2))
}
func TestShellCommandSelection(t *testing.T) {
	for _, shell := range []string{"auto", "sh", "bash"} {
		t.Run(shell, func(t *testing.T) {
			f := &shellDocker{failure: "create"}
			conn, done := shellFixture(t, f, dto.ShellQuery{Shell: shell})
			readShellExit(t, conn)
			command := <-f.commands
			require.Equal(t, []string{"/bin/sh", "-c"}, command[:2])
			if shell == "auto" {
				require.Equal(t, "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi", command[2])
			} else {
				require.Equal(t, "exec "+shell, command[2])
			}
			_, _, _ = conn.Read(t.Context())
			<-done
		})
	}
}
