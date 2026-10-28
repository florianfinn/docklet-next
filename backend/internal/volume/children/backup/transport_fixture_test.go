// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original local, HTTP and real WebSocket command transport fixture.

package backup

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type backupRouteCall func(context.Context, string, string, []byte) (int, http.Header, []byte)

// All modes consume the registered handler's bytes. Edge traverses the real
// command map, WebSocket transport and agent ingress; no producer DTO is invented.
func backupRouteTransport(t *testing.T, mode string, handler http.Handler) backupRouteCall {
	t.Helper()
	if mode == "hub" {
		return func(ctx context.Context, method, path string, body []byte) (int, http.Header, []byte) {
			req := httptest.NewRequest(method, path, bytes.NewReader(body)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			return response.Code, response.Header(), response.Body.Bytes()
		}
	}
	if mode == "direct" {
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		return func(ctx context.Context, method, path string, body []byte) (int, http.Header, []byte) {
			req, err := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			response, err := server.Client().Do(req)
			require.NoError(t, err)
			defer response.Body.Close()
			payload, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			return response.StatusCode, response.Header, payload
		}
	}
	tunnel := backupRouteTunnel(t, handler, edge.NewTunnelRegistry(), uuid.New().String(), uuid.New().String())
	return func(ctx context.Context, method, path string, body []byte) (int, http.Header, []byte) {
		status, headers, payload, err := edge.ProxyRequest(ctx, tunnel, method, path, "", map[string]string{"Content-Type": "application/json"}, body)
		require.NoError(t, err)
		h := http.Header{}
		for key, value := range headers {
			h.Set(key, value)
		}
		return status, h, payload
	}
}

func backupRouteTunnel(t *testing.T, handler http.Handler, registry *edge.TunnelRegistry, id, credential string) *edge.AgentTunnel {
	t.Helper()
	connected := make(chan struct{})
	var once sync.Once
	server := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) { return id, nil }, func(_ context.Context, _ string, online bool) {
		if online {
			once.Do(func() { close(connected) })
		}
	})
	server.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return "", nil })
	router := echo.New()
	router.GET("/api/tunnel/connect", server.HandleConnect)
	manager := httptest.NewServer(router)
	t.Cleanup(manager.Close)
	client := edge.NewTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: credential}, handler)
	life, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); client.StartWithErrorChan(life, nil) }()
	t.Cleanup(func() {
		cancel()
		registry.Unregister(id)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("tunnel cleanup did not join")
		}
	})
	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("tunnel registration did not finish")
	}
	tunnel, ok := registry.Get(id).Get()
	require.True(t, ok)
	return tunnel
}
