// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original proofs that every Edge hub request family uses the injected manager dialer.

package edge

import (
	"context"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// unreachableManager resolves nowhere: only the injected dialer can reach the hub.
const unreachableManager = "http://hub.invalid:3552"

type socketHub struct {
	path  string
	dials atomic.Int64
	mu    sync.Mutex
	seen  []string
}

func (h *socketHub) record(r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seen = append(h.seen, r.Method+" "+r.URL.Path)
}

func (h *socketHub) saw(entry string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.seen {
		if s == entry {
			return true
		}
	}
	return false
}

func (h *socketHub) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	h.dials.Add(1)
	var d net.Dialer
	return d.DialContext(ctx, "unix", h.path)
}

// hostileProxyEnv makes any proxied request fail, so a path that ignored the dialer cannot succeed via a proxy.
func hostileProxyEnv(t *testing.T) {
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy"} {
		t.Setenv(name, "http://127.0.0.1:1")
	}
	t.Setenv("NO_PROXY", "")
}

// newSocketHub serves HTTP/1, h2c and gRPC on a Unix socket that normal routing cannot reach.
func newSocketHub(t *testing.T, tlsServer bool) *socketHub {
	t.Helper()
	hub := &socketHub{path: filepath.Join(t.TempDir(), "hub.sock")}
	grpcServer := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		method, _ := grpc.MethodFromServerStream(stream)
		hub.mu.Lock()
		hub.seen = append(hub.seen, "GRPC "+method)
		hub.mu.Unlock()
		return status.Error(codes.Unauthenticated, "example refusal")
	}))
	t.Cleanup(grpcServer.Stop)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && r.Header.Get("Content-Type") == "application/grpc" {
			grpcServer.ServeHTTP(w, r)
			return
		}
		hub.record(r)
		switch r.URL.Path {
		case "/api/tunnel/connect":
			conn, err := websocket.Accept(w, r, nil)
			if err == nil {
				_ = conn.Close(websocket.StatusNormalClosure, "example")
			}
		case "/api/tunnel/poll":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"connectRequested":false,"pollIntervalSeconds":60}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	listener, err := net.Listen("unix", hub.path)
	require.NoError(t, err)
	if tlsServer {
		server := httptest.NewUnstartedServer(handler)
		_ = server.Listener.Close()
		server.Listener = listener
		server.StartTLS()
		t.Cleanup(server.Close)
		caFile := filepath.Join(t.TempDir(), "ca.pem")
		require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600))
		t.Setenv("EXAMPLE_CA_FILE", caFile)
		return hub
	}
	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{Handler: handler, Protocols: &protocols, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return hub
}

func dialerConfig(hub *socketHub, transport string) *Config {
	return &Config{EdgeAgent: true, EdgeTransport: transport, ManagerApiUrl: unreachableManager, AgentToken: "test-only", ManagerDialer: hub.dial}
}

func TestManagerHTTPClientUsesTheDialerAndIgnoresProxyEnv(t *testing.T) {
	hostileProxyEnv(t)
	client, err := NewManagerHTTPClient(&Config{ManagerApiUrl: unreachableManager, ManagerDialer: (&socketHub{}).dial}, time.Second)
	require.NoError(t, err)
	require.Nil(t, client.Transport.(*http.Transport).Proxy, "tailnet manager calls never use proxy env")
}

func TestPollRequestsReachTheHubOnlyThroughTheDialer(t *testing.T) {
	hostileProxyEnv(t)
	hub := newSocketHub(t, false)
	client := NewTunnelClient(dialerConfig(hub, "poll"), http.NotFoundHandler())
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); _ = client.connectAndServePoll(ctx) }()
	require.Eventually(t, func() bool { return hub.saw("POST /api/tunnel/poll") }, 3*time.Second, 5*time.Millisecond)
	cancel()
	<-done
	require.Positive(t, hub.dials.Load())
}

func TestWebSocketConnectReachesTheHubOnlyThroughTheDialer(t *testing.T) {
	hostileProxyEnv(t)
	hub := newSocketHub(t, false)
	client := NewTunnelClient(dialerConfig(hub, "websocket"), http.NotFoundHandler())
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_ = client.connectAndServeWebSocketInternal(ctx)
	require.True(t, hub.saw("GET /api/tunnel/connect"), "the WebSocket handshake must reach the hub")
	require.Positive(t, hub.dials.Load())
}

func TestGRPCConnectReachesTheHubOnlyThroughTheDialer(t *testing.T) {
	hostileProxyEnv(t)
	hub := newSocketHub(t, false)
	client := NewTunnelClient(dialerConfig(hub, "grpc"), http.NotFoundHandler())
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_ = client.connectAndServeGRPC(ctx)
	require.Eventually(t, func() bool { return hub.saw("GRPC /api/tunnel/connect") }, 3*time.Second, 5*time.Millisecond)
	require.Positive(t, hub.dials.Load())
}

func TestMTLSEnrollmentReachesTheHubOnlyThroughTheDialer(t *testing.T) {
	hostileProxyEnv(t)
	hub := newSocketHub(t, true)
	cfg := &Config{ManagerApiUrl: "https://hub.invalid:3552", AgentToken: "test-only", ManagerDialer: hub.dial, EdgeMTLSServerName: "example.com", EdgeMTLSCAFile: os.Getenv("EXAMPLE_CA_FILE")}
	dir := t.TempDir()
	err := enrollAgentMTLSAssetsInternal(t.Context(), cfg, dir, filepath.Join(dir, "agent.crt"), filepath.Join(dir, "agent.key"))
	require.ErrorContains(t, err, "status 500", "the request completed against the hub")
	require.True(t, hub.saw("POST /api/tunnel/mtls/enroll"))
	require.Positive(t, hub.dials.Load())
}
