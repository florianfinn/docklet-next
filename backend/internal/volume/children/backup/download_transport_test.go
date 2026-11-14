// Source: docklet-next/issues/386
// Revision: dda7c807a3cdd6a46d2d3735c43f57447dedaa61
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original transport proofs for the streamed download: preparation deadline, hub forwarding, idle proxies (#386 review R1 B3).

package backup

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

// A restore that outlives the preparation budget ends the committed stream as an engine timeout and still
// removes the helper and the scratch volume before the admission is released.
func TestStreamedDownloadPreparationDeadlineEndsTheStream(t *testing.T) {
	r := newStreamRig(t, "h1")
	r.f.service.deps.DownloadPrepareBudget = 150 * time.Millisecond
	r.mkdirHold = make(chan struct{})
	defer close(r.mkdirHold)
	response := r.get(t.Context())
	require.Equal(t, http.StatusOK, response.StatusCode)
	_, err := readStream(response).finish(t)
	require.Error(t, err, "a timed-out preparation must not end like a complete body")
	r.requireReleased()
	require.Equal(t, activitytypes.StatusFailed, r.activityStatus())
	require.Equal(t, "volume.backup.engine-timeout", r.activityError())
	_, _, removed := r.f.effects()
	require.Contains(t, removed, "helper")
	require.Contains(t, removed, "scratch")
}

// The hub forwards complete JSON and framed SSE bodies unchanged and aborts a body the agent cut after its 200.
func TestHubForwardingAbortsOnlyFailedBodies(t *testing.T) {
	events := []string{"data: 1\n\n", "data: 2\n\n", "data: 3\n\n"}
	for _, proto := range []string{"h1", "h2"} {
		for _, kind := range []string{"json", "sse", "cut"} {
			t.Run(proto+"/"+kind, func(t *testing.T) {
				agent := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if kind == "json" {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"success":true,"data":{"count":1}}`)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					for i, event := range events {
						_, _ = io.WriteString(w, event)
						w.(http.Flusher).Flush()
						if kind == "cut" && i == 0 {
							panic(http.ErrAbortHandler)
						}
					}
				})
				hub := proxyHub(t, streamServer(t, agent, proto), proto)
				request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, hub.URL+"/api/environments/example-storage/stream", nil)
				require.NoError(t, err)
				response, err := hub.Client().Do(request)
				require.NoError(t, err)
				defer response.Body.Close()
				require.Equal(t, http.StatusOK, response.StatusCode)
				data, err := readStream(response).finish(t)
				switch kind {
				case "json":
					require.NoError(t, err)
					require.Equal(t, "application/json", response.Header.Get("Content-Type"))
					require.Equal(t, `{"success":true,"data":{"count":1}}`, string(data))
				case "sse":
					require.NoError(t, err)
					require.Equal(t, strings.Join(events, ""), string(data))
				default:
					require.Error(t, err, "a body cut upstream must not end like a complete response")
					require.Equal(t, events[0], string(data))
				}
			})
		}
	}
}

// idleProxy forwards TCP to target and closes a connection whose response direction stays silent for idle,
// like a reverse proxy read timeout.
func idleProxy(t *testing.T, target string, idle time.Duration) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			downstream, err := listener.Accept()
			if err != nil {
				return
			}
			upstream, err := net.Dial("tcp", target)
			if err != nil {
				_ = downstream.Close()
				continue
			}
			go func() { _, _ = io.Copy(upstream, downstream) }()
			go func() {
				defer downstream.Close()
				defer upstream.Close()
				buffer := make([]byte, 32<<10)
				for {
					_ = upstream.SetReadDeadline(time.Now().Add(idle))
					n, err := upstream.Read(buffer)
					if n > 0 {
						if _, writeErr := downstream.Write(buffer[:n]); writeErr != nil {
							return
						}
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}()
	return "http://" + listener.Addr().String()
}

// Keep-alive blocks carry a restore that is longer than a proxy's idle budget (4 keep-alive intervals); the
// silent control proves the proxy cuts a stream without them.
func TestStreamedDownloadSurvivesAnIdleBudgetProxy(t *testing.T) {
	for _, mode := range []string{"keep-alive", "silent"} {
		t.Run(mode, func(t *testing.T) {
			r := newStreamRig(t, "h1")
			idle := 4 * r.f.service.deps.DownloadKeepAlive
			if mode == "silent" {
				r.f.service.deps.DownloadKeepAlive = time.Hour
			}
			r.mkdirHold = make(chan struct{})
			time.AfterFunc(6*idle, func() { close(r.mkdirHold) })
			r.url = idleProxy(t, strings.TrimPrefix(r.hub.URL, "http://"), idle) + "/api/environments/example-storage/volumes/backups/row/download"
			r.client = &http.Client{}
			response := r.get(t.Context())
			require.Equal(t, http.StatusOK, response.StatusCode)
			data, err := readStream(response).finish(t)
			r.requireReleased()
			if mode == "silent" {
				require.Error(t, err, "the idle proxy must cut a stream without keep-alive blocks")
				return
			}
			require.NoError(t, err)
			require.Equal(t, r.archive, gunzip(t, data))
			require.Equal(t, activitytypes.StatusSuccess, r.activityStatus())
		})
	}
}
