// Source: docklet-next/issues/289
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 producer and transport regression coverage; M16 bind mounts and backup completion.

package middleware

import (
	"context"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTargetProbeClientRequestIsNeverProxied(t *testing.T) {
	for _, remote := range []string{"https://agent.example.com", "edge://agent"} {
		t.Run(remote, func(t *testing.T) {
			mw := newTestEnvironmentMiddleware()
			resolved := false
			mw.resolver = func(context.Context, string) (string, *string, bool, error) {
				resolved = true
				return remote, nil, true, nil
			}
			router := echo.New()
			group := attachMiddleware(router, mw)
			// Hub has no receiver; a direct client reaches its local refusal and never the agent.
			group.POST("/environments/:id/volumes/backups/targets/probe", func(c *echo.Context) error { return c.NoContent(404) })
			result := httptest.NewRecorder()
			router.ServeHTTP(result, httptest.NewRequest("POST", "/api/environments/remote/volumes/backups/targets/probe", http.NoBody))
			require.Equal(t, 404, result.Code)
			require.False(t, resolved, "client probe must be rejected before resolving a remote executor")
		})
	}
}
