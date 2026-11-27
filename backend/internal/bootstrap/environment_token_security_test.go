// Source: docklet-next/issues/671
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original disabled-token ingress and re-enable regression.

package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisabledEnvironmentTokenIngress(t *testing.T) {
	f := newSignInRoutingFixture(t)
	hosts := environment.NewEnvironmentService(f.db, nil, nil, nil, nil, f.keys)
	token := strings.Repeat("a", 32)
	var keyID string
	require.NoError(t, f.db.Table("api_keys").Where("environment_id = ?", "example").Pluck("id", &keyID).Error)
	require.NotEmpty(t, keyID)
	require.NoError(t, f.db.Model(&environment.Environment{}).Where("id = ?", "example").Updates(map[string]any{"access_token": token, "api_key_id": keyID}).Error)
	e := echo.New()
	gate := auth.NewAuthMiddleware(f.svc, f.cfg).WithApiKeyValidator(f.keys).WithEnvironmentAccessTokenResolver(hosts)
	e.GET("/api/environments/example/containers", func(c *echo.Context) error { return c.NoContent(200) }, gate.Add())
	notifications := notification.NewNotificationService(f.db, f.cfg, hosts, nil)
	cfg := huma.DefaultConfig("test", "1.0.0")
	cfg.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(typ reflect.Type, hint string) string {
		return typ.PkgPath() + "." + huma.DefaultSchemaNamer(typ, hint)
	})
	api := humaecho.NewWithGroup(e, e.Group("/api"), cfg)
	notification.RegisterNotifications(api, notifications, f.cfg)
	server := httptest.NewServer(e)
	defer server.Close()
	for i, enabled := range []bool{true, false, true} {
		t.Run([]string{"initial", "disabled", "re-enabled"}[i], func(t *testing.T) {
			require.NoError(t, f.db.Model(&environment.Environment{}).Where("id = ?", "example").Update("enabled", enabled).Error)
			for _, header := range []string{middleware.HeaderAgentToken, middleware.HeaderApiKey} {
				req, err := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/api/environments/example/containers", nil)
				require.NoError(t, err)
				req.Header.Set(header, token)
				resp, err := server.Client().Do(req)
				require.NoError(t, err)
				resp.Body.Close()
				want := http.StatusUnauthorized
				if enabled {
					want = 200
				}
				assert.Equal(t, want, resp.StatusCode, header)
			}
			req, err := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/api/notifications/dispatch", strings.NewReader(`{"kind":"prune_report","pruneReport":{"result":{}}}`))
			require.NoError(t, err)
			req.Header.Set(middleware.HeaderApiKey, token)
			req.Header.Set("Content-Type", "application/json")
			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			resp.Body.Close()
			if enabled {
				assert.Equal(t, 200, resp.StatusCode)
			} else {
				assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			}
			_, err = hosts.ResolveEnvironmentByAccessToken(t.Context(), token)
			if enabled {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, environment.ErrInvalidEnvironmentAccessToken)
			}

		})
	}
}
