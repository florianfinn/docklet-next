// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original real Direct/Edge viewer bridge tests with a synthetic agent.

package ws

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/docker"
)

func TestRuntimeLogsRealDirectAndEdgeViewerBridges(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, kind := range []string{"projects", "containers"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				joined := make(chan struct{})
				stream := func(ctx context.Context, _ string, out chan<- string, follow bool, tail, since string, timestamps bool) error {
					defer close(joined)
					if !follow || tail != "100" || since != "" || !timestamps {
						return io.ErrUnexpectedEOF
					}
					if err := docker.StreamContainerLogs(ctx, io.NopCloser(strings.NewReader("2026-01-02T03:04:05Z example output\n")), out, follow, true); err != nil {
						return err
					}
					<-ctx.Done()
					return ctx.Err()
				}
				handler := &WebSocketHandler{projectLogStreamer: stream, containerLogStreamer: stream}
				target := logServer(t, handler, true) + kind + "/example/logs?format=json&follow=true&tail=100&timestamps=true&batched=false"
				var viewer string
				if mode == "edge" {
					viewer = edgeLogViewer(t, target)
				} else {
					viewer = directLogViewer(t, target)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				conn, _, err := websocket.Dial(ctx, viewer, &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": []string{"session=example"}, "Authorization": []string{"Bearer example"}, "X-Api-Key": []string{"example"}}})
				require.NoError(t, err)
				defer conn.CloseNow()
				var message wshub.LogMessage
				require.NoError(t, json.Unmarshal(readLog(t, conn), &message))
				require.Equal(t, "example output", message.Message)
				require.Equal(t, "combined", message.Stream)
				require.Equal(t, uint64(1), message.Seq)
				_ = conn.CloseNow()
				waitLogSignal(t, joined)
			})
		}
	}
}
func directLogViewer(t *testing.T, target string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = wshub.ProxyHTTP(w, r, target, nil, func(*http.Request) bool { return true })
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http")
}
func edgeLogViewer(t *testing.T, target string) string {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	targetURL, err := url.Parse(target)
	require.NoError(t, err)
	var workers sync.WaitGroup
	agentDone := make(chan struct{})
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(agentDone)
		socket, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		tunnel := edge.NewTunnelConn(socket)
		defer tunnel.Close()
		var upstream *websocket.Conn
		defer func() {
			if upstream != nil {
				_ = upstream.CloseNow()
			}
			workers.Wait()
		}()
		for {
			message, err := tunnel.Receive()
			if err != nil {
				return
			}
			switch message.Type {
			case edge.MessageTypeStreamOpen:
				if message.Path != targetURL.Path || message.Query != targetURL.RawQuery {
					t.Error("log path/options changed at Edge boundary")
					return
				}
				for name := range message.Headers {
					switch http.CanonicalHeaderKey(name) {
					case "Cookie", "Authorization", "X-Api-Key":
						t.Error("viewer credentials forwarded")
						return
					}
				}
				upstream, _, err = websocket.Dial(ctx, target, nil)
				if err != nil {
					t.Error(err)
					return
				}
				connection := upstream
				id := message.ID
				workers.Go(func() {
					for {
						kind, body, err := connection.Read(ctx)
						if err != nil {
							return
						}
						if err = tunnel.Send(&edge.TunnelMessage{ID: id, Type: edge.MessageTypeStreamData, Body: body, WSMessageType: int(kind)}); err != nil {
							return
						}
					}
				})
			case edge.MessageTypeStreamClose, edge.MessageTypeWebSocketClose:
				if upstream != nil {
					_ = upstream.CloseNow()
				}
			}
		}
	}))
	connection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(agent.URL, "http"), nil)
	require.NoError(t, err)
	tunnel := edge.NewAgentTunnelWithConn("example-edge", edge.NewTunnelConn(connection))
	receiverDone := make(chan struct{})
	go func() {
		defer close(receiverDone)
		for {
			message, err := tunnel.Conn.Receive()
			if err != nil {
				return
			}
			if request, ok := tunnel.Pending.Load(message.ID); ok {
				select {
				case request.(*edge.PendingRequest).ResponseCh <- message:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	router := echo.New()
	router.GET("/logs", func(c *echo.Context) error {
		return edge.ProxyWebSocketRequest(c, tunnel, targetURL.Path, func(*http.Request) bool { return true })
	})
	proxy := httptest.NewServer(router)
	t.Cleanup(func() {
		cancel()
		_ = tunnel.CloseWithReason("")
		proxy.Close()
		agent.Close()
		select {
		case <-receiverDone:
		case <-time.After(5 * time.Second):
			t.Error("Edge receiver did not join")
		}
		waitLogSignal(t, agentDone)
	})
	return "ws" + strings.TrimPrefix(proxy.URL, "http") + "/logs?" + targetURL.RawQuery
}
