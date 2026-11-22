// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original hub tailnet ingress allowlist in front of the shared HTTP/gRPC dispatcher.

package tailnet

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
)

type ingressRoute struct {
	method string
	path   string
	grpc   bool
}

// ingressRoutes is the complete agent surface reachable from the tailnet; the
// raw request path must match exactly, so encoded aliases and double slashes fail.
var ingressRoutes = []ingressRoute{
	{method: http.MethodPost, path: "/api/environments/pair"},
	{method: http.MethodGet, path: "/api/tunnel/connect"},
	{method: http.MethodPost, path: "/api/tunnel/connect", grpc: true},
	{method: http.MethodPost, path: "/api/tunnel/poll"},
	{method: http.MethodPost, path: "/api/tunnel/mtls/enroll"},
	{method: http.MethodPost, path: "/api/events"},
	{method: http.MethodPost, path: "/api/notifications/dispatch"},
}

// forwardedHeaders are never accepted from tailnet peers: the client IP of this ingress is the direct peer.
var forwardedHeaders = []string{
	"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-Port",
	"X-Forwarded-Prefix", "X-Real-Ip", "X-Original-Forwarded-For", "True-Client-Ip", "Cf-Connecting-Ip",
}

func allowedIngressRequest(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	rawPath, _, _ := strings.Cut(r.RequestURI, "?")
	if rawPath != r.URL.Path || r.URL.RawPath != "" {
		return false
	}
	grpc := strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))), "application/grpc")
	for _, route := range ingressRoutes {
		if route.method == r.Method && route.path == rawPath && route.grpc == grpc {
			return true
		}
	}
	return false
}

// IngressHandler admits only the allowlisted agent requests and delegates them
// unchanged (minus forwarded headers and extra identity headers) to next.
func IngressHandler(next http.Handler, extraStripped ...string) http.Handler {
	stripped := append([]string{}, forwardedHeaders...)
	for _, name := range extraStripped {
		if name = strings.TrimSpace(name); name != "" {
			stripped = append(stripped, name)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedIngressRequest(r) {
			http.NotFound(w, r)
			return
		}
		for _, name := range stripped {
			r.Header.Del(name)
		}
		next.ServeHTTP(w, r)
	})
}

type peerKey struct{}

// PeerAddress returns the tailnet peer address of a request that arrived over the tailnet ingress.
func PeerAddress(ctx context.Context) (string, bool) {
	addr, ok := ctx.Value(peerKey{}).(string)
	return addr, ok && addr != ""
}

// WithPeerAddress marks a context as served by the tailnet ingress for the given peer.
func WithPeerAddress(ctx context.Context, addr string) context.Context {
	return context.WithValue(ctx, peerKey{}, addr)
}

// ingressServer is one lifecycle-owned HTTP server over a tailnet listener.
type ingressServer struct {
	server   *http.Server
	listener net.Listener
	cancel   context.CancelFunc
	done     chan struct{}
}

// startIngress serves handler (already allowlisted) over listener with HTTP/1 and h2c.
func startIngress(listener net.Listener, handler http.Handler) *ingressServer {
	baseCtx, cancel := context.WithCancel(context.Background()) //nolint:forbidigo // Ingress requests need a root outside the request that enabled the route.
	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{
		Handler:           handler,
		Protocols:         &protocols,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return baseCtx },
		ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
			return WithPeerAddress(httpx.WithConn(ctx, conn), conn.RemoteAddr().String())
		},
	}
	ingress := &ingressServer{server: server, listener: listener, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(ingress.done)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Warn("Tailnet ingress stopped", "error", err)
		}
	}()
	return ingress
}

// stop closes the listener and in-flight streams before returning.
func (i *ingressServer) stop(ctx context.Context) {
	if i == nil {
		return
	}
	i.cancel()
	_ = i.server.Shutdown(ctx)
	_ = i.server.Close()
	<-i.done
}
