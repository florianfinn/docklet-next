// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original tailnet ingress allowlist and forwarded-header proofs over the real hub dispatcher.

package bootstrap

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet/tailnettest"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	tunnelpb "github.com/florianfinn/docklet-next/backend/proto/tunnel/v1"
	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

type ingressHarness struct {
	direct  string // the shared hub listener, forwarded headers trusted from loopback
	ingress string // the tailnet ingress listener (loopback stands in for tsnet)
}

func newIngressHarness(t *testing.T) ingressHarness {
	t.Helper()
	cfg := &config.Config{AppUrl: "http://example.com", Environment: config.AppEnvironmentTest, TrustedProxies: "127.0.0.0/8", EdgeMTLSMode: "disabled"}
	deps := api.HandlerDeps{
		Project:     project.New(&project.ProjectService{}, nil),
		Container:   container.New(&container.ContainerService{}, nil, nil, nil),
		System:      system.New(&system.SystemService{}, nil, nil, nil, nil),
		Environment: environment.New(&environment.EnvironmentService{}, nil, nil, nil, cfg, nil),
	}
	router, tunnel := newRouter(RouterParams{Context: t.Context(), Lifecycle: fxtest.NewLifecycle(t), Config: cfg, HandlerDeps: deps, AuthMiddleware: auth.NewAuthMiddleware(nil, cfg), TunnelRegistry: edge.NewTunnelRegistry()})
	shared, grpcServer := configureTunnelServerInternal(t.Context(), cfg, router, tunnel, nil, "")
	t.Cleanup(grpcServer.Stop)
	sharedHandler, protocols := configureHTTPProtocolsInternal(false, shared)

	directListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	directServer := &http.Server{Handler: sharedHandler, Protocols: protocols, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = directServer.Serve(directListener) }()
	t.Cleanup(func() { _ = directServer.Close() })

	// Same wiring as NewHTTPServer: the manager serves the allowlisted shared dispatcher on its node listener.
	fleet := tailnettest.NewFleet()
	manager := tailnet.NewManager(tailnet.ManagerConfig{StateDir: filepath.Join(t.TempDir(), "ts"), MTLSDisabled: true, Factory: fleet.Factory})
	t.Cleanup(func() { manager.Close(context.Background()) })
	ingressHandler, _ := configureHTTPProtocolsInternal(false, shared)
	manager.SetIngressHandler(tailnet.IngressHandler(ingressHandler, cfg.AuthProxyHeader))
	manager.Apply(t.Context(), true, tailnet.Credential{Mode: tailnettypes.ModeManual, AuthKey: tailnettest.Key("auth", "example")})
	require.Eventually(t, func() bool { return len(fleet.Nodes()) == 1 && fleet.Nodes()[0].Addr() != "" }, 2*time.Second, 5*time.Millisecond)
	return ingressHarness{direct: directListener.Addr().String(), ingress: fleet.Nodes()[0].Addr()}
}

// rawRequest sends an exact request line so encoded aliases and double slashes reach the server unchanged.
func rawRequest(t *testing.T, addr, method, target string, headers ...string) (int, string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	request := fmt.Sprintf("%s %s HTTP/1.1\r\nHost: example.com\r\nContent-Length: 0\r\nConnection: close\r\n%s\r\n", method, target, strings.Join(append(headers, ""), "\r\n"))
	_, err = io.WriteString(conn, request)
	require.NoError(t, err)
	reply, err := io.ReadAll(conn)
	require.NoError(t, err)
	var code int
	_, err = fmt.Sscanf(string(reply), "HTTP/1.1 %d", &code)
	require.NoError(t, err, string(reply))
	return code, string(reply)
}

func TestTailnetIngressAdmitsOnlyTheAgentSurface(t *testing.T) {
	h := newIngressHarness(t)
	for _, tc := range []struct {
		name, method, target string
	}{
		{"UI root", http.MethodGet, "/"},
		{"UI asset", http.MethodGet, "/assets/index.js"},
		{"settings API", http.MethodGet, "/api/settings"},
		{"tailnet settings API", http.MethodPut, "/api/settings/connections/tailnet"},
		{"encoded dot alias", http.MethodGet, "/api/tunnel/%2e%2e/x"},
		{"encoded letter alias", http.MethodPost, "/api/tunnel/%70oll"},
		{"double slash", http.MethodPost, "//api/tunnel/poll"},
		{"trailing slash", http.MethodPost, "/api/tunnel/poll/"},
		{"wrong method on poll", http.MethodGet, "/api/tunnel/poll"},
		{"wrong method on pair", http.MethodGet, "/api/environments/pair"},
		{"host proxy path", http.MethodGet, "/api/environments/0/containers"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := rawRequest(t, h.ingress, tc.method, tc.target)
			require.Equal(t, http.StatusNotFound, code, body)
			require.Contains(t, body, "404 page not found")
		})
	}
	t.Run("allowlisted requests reach the existing handlers", func(t *testing.T) {
		for _, tc := range []struct{ method, target string }{
			{http.MethodPost, "/api/environments/pair"},
			{http.MethodPost, "/api/tunnel/poll"},
			{http.MethodGet, "/api/tunnel/connect"},
			{http.MethodPost, "/api/tunnel/mtls/enroll"},
			{http.MethodPost, "/api/events"},
			{http.MethodPost, "/api/notifications/dispatch"},
		} {
			directCode, _ := rawRequest(t, h.direct, tc.method, tc.target, "X-Forwarded-For: 192.0.2.200")
			code, body := rawRequest(t, h.ingress, tc.method, tc.target)
			require.NotContains(t, body, "404 page not found", tc.target)
			require.Equal(t, directCode, code, "%s %s must behave as on the shared listener", tc.method, tc.target)
		}
	})
}

func TestTailnetIngressAdmitsOnlyTheTunnelGRPCMethod(t *testing.T) {
	h := newIngressHarness(t)
	conn, err := grpc.NewClient("passthrough:///"+h.ingress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	call := func(method string) error {
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		stream, err := conn.NewStream(ctx, &tunnelpb.TunnelService_ServiceDesc.Streams[0], method)
		if err != nil {
			return err
		}
		return stream.RecvMsg(&tunnelpb.ManagerMessage{})
	}
	// The agent's compatibility path reaches the tunnel server, which refuses the missing token.
	err = call("/api/tunnel/connect")
	require.Equal(t, codes.Unauthenticated, status.Code(err), err)
	for _, method := range []string{tunnelpb.TunnelService_Connect_FullMethodName, "/tunnel.v1.TunnelService/Other", "/api/tunnel/poll", "/grpc.health.v1.Health/Watch"} {
		err := call(method)
		require.Error(t, err, method)
		require.Contains(t, err.Error(), "404", "%s must be refused by the allowlist before gRPC dispatch", method)
	}
}

// Forwarded headers from a tailnet peer never change the client identity: the real per-IP limiter keys on the peer.
func TestTailnetIngressIgnoresForwardedHeadersInTheRateLimiter(t *testing.T) {
	h := newIngressHarness(t)
	send := func(addr string, i int) int {
		code, _ := rawRequest(t, addr, http.MethodGet, "/api/tunnel/connect", fmt.Sprintf("X-Forwarded-For: 192.0.2.%d", i%250+1), fmt.Sprintf("X-Real-Ip: 198.51.100.%d", i%250+1))
		return code
	}
	// Control: the shared listener trusts loopback proxies, so distinct forwarded addresses are distinct keys.
	for i := range 40 {
		require.NotEqual(t, http.StatusTooManyRequests, send(h.direct, i))
	}
	limited := 0
	for i := range 40 {
		if send(h.ingress, i) == http.StatusTooManyRequests {
			limited++
		}
	}
	require.Positive(t, limited, "spoofed forwarded headers must not escape the per-peer limit (burst 30)")
}
