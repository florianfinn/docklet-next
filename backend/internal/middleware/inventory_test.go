// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original cached inventory regression tests.

package middleware

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestInventoryStaleAndReconnect(t *testing.T) {
	m := &EnvironmentMiddleware{snapshots: &hoststate.Store{}}
	read := func(status int, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/environments/example/projects?limit=100", nil), rec)
		require.NoError(t, m.inventoryRead(c, "example", func() error { return c.JSONBlob(status, []byte(body)) }))
		return rec
	}
	fresh := read(200, `{"success":true,"data":[{"id":"first"}]}`)
	observed := fresh.Header().Get("X-Docklet-Observed-At")
	require.NotEmpty(t, observed)
	for _, code := range []string{"agent.unavailable", "agent.invalid-token", "agent.update-required"} {
		stale := read(502, `{"success":false,"code":"`+code+`"}`)
		require.Equal(t, 200, stale.Code)
		require.JSONEq(t, fresh.Body.String(), stale.Body.String())
		require.Equal(t, observed, stale.Header().Get("X-Docklet-Observed-At"))
		require.Equal(t, code, stale.Header().Get("X-Docklet-Host-Reason"))
	}
	unauthorized := read(403, `{"success":false,"code":"forbidden"}`)
	require.Equal(t, 403, unauthorized.Code)
	reconnected := read(200, `{"success":true,"data":[{"id":"second"}]}`)
	require.Contains(t, reconnected.Body.String(), "second")
	require.Empty(t, reconnected.Header().Get("X-Docklet-Host-Reason"))
	require.NotEqual(t, observed, reconnected.Header().Get("X-Docklet-Observed-At"))
}

func TestInventoryCompatibilityBlocksOrdinaryRead(t *testing.T) {
	reads := 0
	incompatible := false
	store := &hoststate.Store{}
	router := echo.New()
	router.Use(NewEnvProxyMiddlewareWithParamAndRegistry("0", "id",
		func(context.Context, string) (string, *string, bool, error) {
			token := "example"
			return "http://agent.example.com", &token, true, nil
		},
		nil, nil, nil, nil, EnvironmentProxySecurity{
			Snapshots: store,
			AgentAccess: func(ctx context.Context, _ string, _ *string, _, _ string) (context.Context, context.CancelFunc, error) {
				if incompatible {
					return nil, nil, remenv.ErrAgentUpdateRequired
				}
				return ctx, func() {}, nil
			},
			HTTPClient: &http.Client{Transport: inventoryTransport(func(r *http.Request) (*http.Response, error) {
				reads++
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"success":true,"data":[{"id":"app"}]}`)), Request: r}, nil
			})},
		}))
	router.GET("/api/environments/:id/projects", func(c *echo.Context) error { return c.NoContent(500) })
	request := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/environments/host/projects", nil))
		return rec
	}
	fresh := request()
	require.Equal(t, 200, fresh.Code)
	require.Equal(t, 1, reads)
	incompatible = true
	stale := request()
	require.Equal(t, 1, reads, "incompatible host must receive no ordinary agent read")
	require.Equal(t, 200, stale.Code)
	require.JSONEq(t, fresh.Body.String(), stale.Body.String())
	require.Equal(t, "agent.update-required", stale.Header().Get("X-Docklet-Host-Reason"))
	require.Equal(t, fresh.Header().Get("X-Docklet-Observed-At"), stale.Header().Get("X-Docklet-Observed-At"))
}

type inventoryTransport func(*http.Request) (*http.Response, error)

func (f inventoryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
