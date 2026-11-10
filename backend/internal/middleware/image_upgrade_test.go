// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original image and registry safety contract for issue #229; self-update ceiling moved to the hub owner with explicit route handoff proof.

package middleware

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSelfUpdateRoutesReachHubOwnerBeforeDirectOrEdgeDispatch(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(204) }))
			defer agent.Close()
			target := agent.URL
			if mode == "edge" {
				target = "edge://example-host"
			}
			router := echo.New()
			router.Use(NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) {
				return target, new("manager-token"), true, nil
			}, func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
				return authz.SudoPermissionSet(), &user.Actor{ID: "operator"}, true
			}, nil, nil, nil))
			var ownerCalls atomic.Int32
			router.Any("/api/environments/:id/*", func(c *echo.Context) error { ownerCalls.Add(1); return c.NoContent(204) })
			// The actual hub owner enforces the ceiling in the upgrade transport tests.
			// Middleware must delegate both recovery routes without contacting an agent.
			for _, tc := range []struct{ method, path, body string }{
				{http.MethodPost, "/system/upgrade", `{"targetVersion":"2.1.0"}`},
				{http.MethodPost, "/system/upgrade", `{}`},
				{http.MethodPost, "/system/upgrade", `{"targetVersion":"unknown"}`},
				{http.MethodGet, "/system/upgrade/check", ``},
			} {
				req := httptest.NewRequest(tc.method, "/api/environments/example-host"+tc.path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				assert.Equal(t, 204, rec.Code, rec.Body.String())
			}
			require.EqualValues(t, 4, ownerCalls.Load())
			require.Zero(t, calls.Load(), "hub owner must receive requests before remote effects")
		})
	}
}
