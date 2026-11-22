// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Unix-socket fake hub reachable only through an injected dialer (tests only).

package tailnettest

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// UnreachableManager resolves nowhere; only the injected dialer reaches the fake hub.
const UnreachableManager = "http://hub.invalid:3552"

// SocketHub is a fake hub on a Unix socket that records request lines.
type SocketHub struct {
	path  string
	Dials atomic.Int64
	mu    sync.Mutex
	seen  []string
}

// NewSocketHub serves handler (or 200 with an empty data envelope) and records every request.
func NewSocketHub(t *testing.T, handler http.Handler) *SocketHub {
	t.Helper()
	hub := &SocketHub{path: filepath.Join(t.TempDir(), "hub.sock")}
	listener, err := net.Listen("unix", hub.path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.mu.Lock()
		hub.seen = append(hub.seen, r.Method+" "+r.URL.Path)
		hub.mu.Unlock()
		if handler != nil {
			handler.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"message":"ok"}}`))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return hub
}

// Dial ignores the address and connects to the hub socket.
func (h *SocketHub) Dial(ctx context.Context, _, _ string) (net.Conn, error) {
	h.Dials.Add(1)
	var d net.Dialer
	return d.DialContext(ctx, "unix", h.path)
}

// Saw reports whether a request line was received.
func (h *SocketHub) Saw(entry string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.seen {
		if s == entry {
			return true
		}
	}
	return false
}

// HostileProxyEnv points every proxy variable at a closed port.
func HostileProxyEnv(t *testing.T) {
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(name, "http://127.0.0.1:1")
	}
	t.Setenv("NO_PROXY", "")
}

// Key builds a synthetic Tailscale key or OAuth secret of the given kind (auth or client).
func Key(kind, label string) string { return "tskey-" + kind + "-" + label }
