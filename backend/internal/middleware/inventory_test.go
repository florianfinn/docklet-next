// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original cached inventory regression tests.

package middleware

import (
	"net/http"
	"net/http/httptest"
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
