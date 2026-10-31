// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original hub-owned restart authorization boundary test.

package environment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/stretchr/testify/require"
)

func TestHostRestartHTTPRequiresGlobalAdmin(t *testing.T) {
	for _, tc := range []struct {
		name        string
		permissions *authz.PermissionSet
		status      int
	}{
		{"anonymous", nil, http.StatusForbidden},
		{"delegated-host-manager", &authz.PermissionSet{Global: map[string]struct{}{authz.PermEnvironmentsRead: {}, authz.PermEnvironmentsUpdate: {}, authz.PermEnvironmentsPair: {}}}, http.StatusForbidden},
		{"environment-token", authz.EnvironmentPermissionSet("example-host"), http.StatusForbidden},
		{"global-admin", authz.SudoPermissionSet(), http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			api := humago.New(mux, huma.DefaultConfig("Restart", "0.0.0"))
			api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, tc.permissions)))
			})
			// The nil controller yields 503 only after the caller clears the gate.
			registerHostRestart(api, &EnvironmentHandler{})
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/environments/example-host/agent/restart", nil))
			require.Equal(t, tc.status, response.Code, response.Body.String())
		})
	}
}
