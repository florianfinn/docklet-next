// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Dedicated policy/schedule reads stay on hub without contacting offline executors.

package middleware

import (
	"context"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestImagePolicyRoutesBypassGenericAgentForwarding(t *testing.T) {
	for _, path := range []string{"/updater/schedules", "/updater/policies/project/example-stack", "/updater/system-image-hints", "/updater/projects/example-stack/check"} {
		mw := newTestEnvironmentMiddleware()
		calls := 0
		mw.resolver = func(context.Context, string) (string, *string, bool, error) {
			calls++
			return "", nil, false, nil
		}
		router := echo.New()
		group := attachMiddleware(router, mw)
		method := http.MethodGet
		if path == "/updater/projects/example-stack/check" {
			method = http.MethodPost
		}
		group.Add(method, "/environments/:id"+path, func(c *echo.Context) error { return c.NoContent(204) })
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, "/api/environments/example-offline"+path, http.NoBody))
		require.Equal(t, 204, rec.Code, rec.Body.String())
		require.Zero(t, calls)
	}
}
