// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original exact hub restart routing and zero-forward proof.

package middleware

import (
	"context"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestHostRestartRoutesStayHubOwnedWithZeroForwards(t *testing.T) {
	for _, route := range []struct{ method, suffix string }{{"POST", "/agent/restart"}, {"GET", "/agent/restarts/example-run"}} {
		t.Run(route.method+route.suffix, func(t *testing.T) {
			var forwards atomic.Int32
			agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwards.Add(1); w.WriteHeader(418) }))
			defer agent.Close()
			mw := newTestEnvironmentMiddleware()
			resolves := 0
			mw.resolver = func(context.Context, string) (string, *string, bool, error) {
				resolves++
				return agent.URL, new("example"), true, nil
			}
			router := echo.New()
			group := attachMiddleware(router, mw)
			group.Add(route.method, "/environments/:id"+route.suffix, func(c *echo.Context) error { return c.NoContent(204) })
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(route.method, "/api/environments/example-host"+route.suffix, nil))
			require.Equal(t, 204, rec.Code, rec.Body.String())
			require.Zero(t, resolves)
			require.Zero(t, forwards.Load())
		})
	}
	for _, route := range []struct{ method, suffix string }{
		{"GET", "/agent/restart"}, {"PUT", "/agent/restart"}, {"POST", "/agent/restart/extra"}, {"POST", "/agent/restarts/example-run"},
		{"DELETE", "/agent/restarts/example-run"}, {"GET", "/agent/restarts/"}, {"GET", "/agent/restarts/example-run/extra"}, {"GET", "/agent/restarts/example-run/"},
	} {
		require.False(t, isManagementPathInternal(route.method, route.suffix), route.method+" "+route.suffix)
	}
}
