// Source: docklet-next/issues/694
// Revision: b80bae99612a0d7f81481fee8f65d4be4dd2caa4
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original lossless finite read tests over direct, Direct-agent and Edge bridges.

package ws

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/docker"
)

const finiteLines = 20000

func finiteSource() string {
	var b strings.Builder
	for i := range finiteLines {
		fmt.Fprintf(&b, "2026-01-02T03:%02d:%02dZ line %d\n", i/3600%60, i/60%60, i)
	}
	return b.String()
}

// A finite read delivers every record in order to a slow viewer and ends with a normal close,
// for the direct handler and through the Direct-agent and Edge bridges, containers and projects.
func TestRuntimeLogsFiniteReadIsLosslessForSlowViewers(t *testing.T) {
	source := finiteSource()
	for _, mode := range []string{"local", "direct", "edge"} {
		for _, kind := range []string{"containers", "projects"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				stream := func(ctx context.Context, _ string, out chan<- string, follow bool, _, _, _ string, _ bool) error {
					return docker.StreamContainerLogs(ctx, io.NopCloser(strings.NewReader(source)), out, follow, true)
				}
				handler := &WebSocketHandler{projectLogStreamer: stream, containerLogStreamer: stream}
				target := logServer(t, handler, true) + kind + "/example/logs?format=json&follow=false&tail=all&timestamps=true&batched=false"
				viewer := target
				switch mode {
				case "direct":
					viewer = directLogViewer(t, target)
				case "edge":
					viewer = edgeLogViewer(t, target)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
				defer cancel()
				conn, _, err := websocket.Dial(ctx, viewer, &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": []string{"session=example"}}})
				require.NoError(t, err)
				defer conn.CloseNow()
				conn.SetReadLimit(4 << 20)
				// A slow start lets the source run far ahead of the viewer.
				time.Sleep(300 * time.Millisecond)
				received := 0
				for {
					// The test Edge agent double forwards data but not the upstream close; the real
					// tunnel client sends a stream close after the last record.
					if mode == "edge" && received == finiteLines {
						break
					}
					_, data, err := conn.Read(ctx)
					if err != nil {
						require.Equal(t, websocket.StatusNormalClosure, websocket.CloseStatus(err), "a finite read ends with a normal close after %d records: %v", received, err)
						break
					}
					var message wshub.LogMessage
					require.NoError(t, json.Unmarshal(data, &message))
					require.Equal(t, uint64(received+1), message.Seq, "records arrive without gaps")
					require.Equal(t, fmt.Sprintf("line %d", received), message.Message)
					received++
				}
				require.Equal(t, finiteLines, received, "every record of the window is delivered")
			})
		}
	}
}
