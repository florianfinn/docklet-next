// Source: docklet-next/issues/595
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original hub-management boundary proofs for manual container and host checks.

package updater

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestContainerAndHostCheckRoutesNeverForward(t *testing.T) {
	for _, mode := range []string{"Direct", "Edge"} {
		for _, suffix := range []string{"/updater/containers/example-container/check", "/jobs/update-check-host/run"} {
			t.Run(mode+suffix, func(t *testing.T) {
				resolved, handled := 0, 0
				registry := edge.NewTunnelRegistry()
				proxy := middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) {
					resolved++
					return "", nil, false, errors.New("must remain hub-owned")
				}, func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
					return authz.SudoPermissionSet(), &user.Actor{ID: "example-operator", Username: "operator"}, true
				}, nil, registry, nil)
				router := echo.New()
				group := router.Group("/api", proxy)
				group.POST("/environments/:id/*", func(c *echo.Context) error { handled++; return c.NoContent(204) })
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/environments/example-host"+suffix, http.NoBody))
				require.Equal(t, 204, recorder.Code, recorder.Body.String())
				require.Equal(t, 1, handled)
				require.Zero(t, resolved)
			})
		}
	}
}
