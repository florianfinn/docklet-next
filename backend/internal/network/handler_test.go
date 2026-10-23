// Source: docklet-next/docs/design/network-management.md
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package network

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func networkRouter(t *testing.T, svc *NetworkService, withUser bool) http.Handler {
	t.Helper()
	router := echo.New()
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
			if withUser {
				ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &actor)
			}
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("networks", "test"))
	RegisterNetworks(api, svc, svc.dockerService, nil, handlerutil.NewActivityAppContext(t.Context()))
	return router
}
func networkRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestNetworkCreateWireFamiliesAndIPAM(t *testing.T) {
	for _, tc := range []struct {
		name, options string
		v4            *bool
		v6            bool
	}{
		{"omitted", `{}`, nil, false},
		{"explicit IPv4", `{"enableIPv4":true}`, new(true), false},
		{"IPv6 only", `{"enableIPv4":false,"enableIPv6":true,"ipam":{"config":[{"subnet":"2001:db8::/64","gateway":"2001:db8::1","ipRange":"2001:db8::/80","auxAddress":{"reserved":"2001:db8::2"}}]}}`, new(false), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &networkDockerFixture{}
			svc := f.service(t)
			rec := networkRequest(networkRouter(t, svc, true), "POST", "/api/environments/0/networks", `{"name":"example","options":`+tc.options+`}`)
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.Equal(t, tc.v4, f.created.EnableIPv4)
			if tc.v6 {
				require.NotNil(t, f.created.EnableIPv6)
				require.True(t, *f.created.EnableIPv6)
				require.Equal(t, "2001:db8::2", f.created.IPAM.Config[0].AuxAddress["reserved"].String())
			}
			require.Equal(t, []string{"create"}, f.writes)
		})
	}
}

func TestNetworkCreateWireRejectsInvalidInput(t *testing.T) {
	for _, body := range []string{
		`{"name":"","options":{}}`,
		`{"name":"default","options":{}}`,
		`{"name":"example","options":{"driver":"overlay"}}`,
		`{"name":"example","options":{"ipam":{"config":[{"subnet":"invalid"}]}}}`,
		`{"name":"example","options":{"ipam":{"config":[{"subnet":"192.0.2.0/24","gateway":"invalid"}]}}}`,
		`{"name":"example","options":{"ipam":{"config":[{"subnet":"192.0.2.0/24","ipRange":"invalid"}]}}}`,
		`{"name":"example","options":{"ipam":{"config":[{"subnet":"192.0.2.0/24","auxAddress":{"reserved":"invalid"}}]}}}`,
	} {
		t.Run(body, func(t *testing.T) {
			f := &networkDockerFixture{}
			svc := f.service(t)
			rec := networkRequest(networkRouter(t, svc, true), "POST", "/api/environments/0/networks", body)
			require.Equal(t, 400, rec.Code, rec.Body.String())
			require.Empty(t, f.writes)
			var problem struct{ Code string }
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			require.Equal(t, "network.invalid-network", problem.Code)
		})
	}
}
func TestPruneHandlerRequiresActingUser(t *testing.T) {
	f := &networkDockerFixture{}
	svc := f.service(t)
	rec := networkRequest(networkRouter(t, svc, false), "POST", "/api/environments/0/networks/prune", `{}`)
	require.Equal(t, 401, rec.Code, rec.Body.String())
	require.Empty(t, f.writes)
	rec = networkRequest(networkRouter(t, svc, true), "POST", "/api/environments/0/networks/prune", `{}`)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Equal(t, []string{"prune"}, f.writes)
}
