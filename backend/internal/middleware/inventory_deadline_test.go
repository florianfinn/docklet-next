// Source: docklet-next/issues/302
// Revision: a3d05d7da5287182dbc14d2c4fc5946283884a5f
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original bounded inventory read tests.

package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

// A frozen Direct agent accepts the request and never answers. Ordinary inventory reads
// must still answer within inventoryReadTimeout, so they release the browser's connection.
func TestInventoryReadFromFrozenDirectAgentIsBounded(t *testing.T) {
	previous := inventoryReadTimeout
	inventoryReadTimeout = 100 * time.Millisecond
	t.Cleanup(func() { inventoryReadTimeout = previous })
	frozen, otherBounded, otherSent := true, false, false
	router := echo.New()
	router.Use(NewEnvProxyMiddlewareWithParamAndRegistry("0", "id",
		func(context.Context, string) (string, *string, bool, error) {
			token := "example"
			return "http://agent.example.com", &token, true, nil
		},
		nil, nil, nil, nil, EnvironmentProxySecurity{
			Snapshots: &hoststate.Store{},
			AgentAccess: func(ctx context.Context, _ string, _ *string, _, _ string) (context.Context, context.CancelFunc, error) {
				child, cancel := context.WithCancel(ctx)
				return child, cancel, nil
			},
			HTTPClient: &http.Client{Transport: inventoryTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"success":true,"data":[{"id":"app"}]}`
				if strings.HasSuffix(r.URL.Path, "/system/info") {
					otherSent = true
					deadline, ok := r.Context().Deadline()
					otherBounded = ok && time.Until(deadline) <= inventoryReadTimeout
					body = `{"success":true}`
				} else if frozen {
					select {
					case <-r.Context().Done():
						return nil, r.Context().Err()
					case <-time.After(3 * time.Second):
						body = `{"success":true,"data":[{"id":"late"}]}`
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})},
		}))
	router.GET("/api/environments/:id/containers", func(c *echo.Context) error { return c.NoContent(500) })
	router.GET("/api/environments/:id/system/info", func(c *echo.Context) error { return c.NoContent(500) })
	request := func(method, path string) (*httptest.ResponseRecorder, time.Duration) {
		rec := httptest.NewRecorder()
		started := time.Now()
		router.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec, time.Since(started)
	}

	cold, elapsed := request(http.MethodGet, "/api/environments/host/containers")
	require.Less(t, elapsed, 2*time.Second)
	require.Equal(t, http.StatusBadGateway, cold.Code)
	require.Contains(t, cold.Body.String(), `"code":"agent.unavailable"`)

	frozen = false
	fresh, _ := request(http.MethodGet, "/api/environments/host/containers")
	require.Equal(t, 200, fresh.Code)
	frozen = true
	stale, elapsed := request(http.MethodGet, "/api/environments/host/containers")
	require.Less(t, elapsed, 2*time.Second)
	require.Equal(t, 200, stale.Code)
	require.JSONEq(t, fresh.Body.String(), stale.Body.String())
	require.Equal(t, "agent.unavailable", stale.Header().Get("X-Docklet-Host-Reason"))

	other, _ := request(http.MethodGet, "/api/environments/host/system/info")
	require.True(t, otherSent, "the non-inventory read reached the agent: %d %s", other.Code, other.Body.String())
	require.False(t, otherBounded, "other proxied operations keep their own budgets")
}

// The agent sends its headers and a JSON prefix, then freezes. The bound ends the body
// transfer, which must answer like a failure before headers, not as a truncated success.
func TestInventoryReadStalledAfterHeadersIsUnavailable(t *testing.T) {
	// A complete envelope still fails when its transfer never finishes.
	for name, prefix := range map[string]string{"partial": `{"success":true,"data":[`, "complete": `{"success":true,"data":[]}`} {
		t.Run(name, func(t *testing.T) { stalledAfterHeaders(t, prefix) })
	}
}

func stalledAfterHeaders(t *testing.T, prefix string) {
	previous := inventoryReadTimeout
	inventoryReadTimeout = 100 * time.Millisecond
	t.Cleanup(func() { inventoryReadTimeout = previous })
	var stalled atomic.Bool
	stalled.Store(true)
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !stalled.Load() {
			_, _ = w.Write([]byte(`{"success":true,"data":[{"id":"app"}]}`))
			return
		}
		_, _ = w.Write([]byte(prefix))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	t.Cleanup(agent.Close)
	mw := newTestEnvironmentMiddleware()
	mw.snapshots = &hoststate.Store{}
	token := "example"
	mw.resolver = func(context.Context, string) (string, *string, bool, error) { return agent.URL, &token, true, nil }
	router := echo.New()
	group := attachMiddleware(router, mw)
	group.Any("/environments/:id/*", func(c *echo.Context) error { return c.NoContent(418) })
	request := func() (*httptest.ResponseRecorder, time.Duration) {
		rec := httptest.NewRecorder()
		started := time.Now()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/environments/example/containers", nil))
		return rec, time.Since(started)
	}

	cold, elapsed := request()
	require.Less(t, elapsed, 2*time.Second)
	require.Equal(t, http.StatusBadGateway, cold.Code, cold.Body.String())
	require.JSONEq(t, `{"success":false,"code":"agent.unavailable"}`, cold.Body.String())

	stalled.Store(false)
	fresh, _ := request()
	require.Equal(t, 200, fresh.Code, fresh.Body.String())
	stalled.Store(true)
	stale, elapsed := request()
	require.Less(t, elapsed, 2*time.Second)
	require.Equal(t, 200, stale.Code)
	require.JSONEq(t, fresh.Body.String(), stale.Body.String())
	require.Equal(t, "agent.unavailable", stale.Header().Get("X-Docklet-Host-Reason"))
}
