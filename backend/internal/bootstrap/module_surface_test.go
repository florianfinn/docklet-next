// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original exhaustive production proxy registration gate proof.

package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	p "github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	authDTO "github.com/florianfinn/docklet-next/types/auth"
	"github.com/stretchr/testify/require"
)

func TestM27M1EveryProductionProxyOperationIsGated(t *testing.T) {
	x := newM27ModuleFixture(t, "direct", false)
	viewer, err := x.f.users.CreateUser(t.Context(), &user.User{Username: "surface-viewer"})
	require.NoError(t, err)
	login, err := x.f.svc.CompleteLogin(t.Context(), viewer, authDTO.SessionMeta{}, "local")
	require.NoError(t, err)
	entered := 0
	var operations []*huma.Operation
	x.hubAPI.OpenAPI().OnAddOperation = append(x.hubAPI.OpenAPI().OnAddOperation, func(_ *huma.OpenAPI, op *huma.Operation) {
		if len(op.Tags) != 1 || op.Tags[0] != "Proxy" {
			return
		}
		operations = append(operations, op)
		// A sentinel at the handler boundary avoids invoking real effects if the
		// gate regresses. It runs after the production operation middleware.
		op.Middlewares = append(op.Middlewares, func(ctx huma.Context, _ func(huma.Context)) {
			entered++
			ctx.SetStatus(http.StatusNoContent)
		})
	})
	m := &p.Module{Settings: x.hubSettings, Modules: x.manager}
	m.RegisterRoutes(x.hubAPI, false)
	require.NotEmpty(t, operations)
	for _, op := range operations {
		t.Run(op.OperationID, func(t *testing.T) {
			// Host-local routes exercise the actual operation gate; remote
			// forwarding has its own Direct/Edge M2 witnesses.
			path := "/api" + strings.NewReplacer("{id}", "0", "{instanceId}", "example-instance", "{operationId}", "example-operation").Replace(op.Path)
			admin := x.request(op.Method, path, "{}")
			require.Zero(t, entered, "disabled operations must not reach a handler or its Docker effects")
			require.Equal(t, 404, admin.Code, admin.Body.String())
			require.Equal(t, m27UnknownResponse(op.Method, path).Body.String(), admin.Body.String())
			r := httptest.NewRequest(op.Method, path, strings.NewReader("{}"))
			r.Header.Set("Content-Type", "application/json")
			r.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: login.BrowserToken})
			w := httptest.NewRecorder()
			x.hub.ServeHTTP(w, r)
			require.Equal(t, 403, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "access.admin-required")
			require.Zero(t, entered)
		})
	}
	require.Zero(t, x.calls.Load())
	require.Zero(t, x.hubCalls.Load())
}
