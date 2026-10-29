// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package autoheal

import (
	"context"
	"fmt"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	dto "github.com/florianfinn/docklet-next/types/autoheal"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
)

func TestAutoHealRoutesLocalProxyValidationAndAccess(t *testing.T) {
	store, _ := healStore(t)
	require.NoError(t, store.DB.AutoMigrate(&settings.SettingVariable{}))
	s, e := settings.NewSettingsService(t.Context(), store.DB)
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, s.Stop(context.Background())) })
	_, api := humatest.New(t)
	api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
		next(huma.WithContext(c, context.WithValue(c.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	proxyErr := error(nil)
	calls := 0
	m := &Module{settings: s, config: &config.Config{}, proxy: func(_ context.Context, id, method, path string, body []byte, result any) error {
		calls++
		require.Equal(t, "remote", id)
		require.Equal(t, "/api/environments/0/auto-heal", path)
		if method == http.MethodPut {
			require.NotEmpty(t, body)
		}
		if proxyErr == nil {
			*result.(*dto.Settings) = dto.Settings{MaxRestarts: 3, WindowMinutes: 30}
		}
		return proxyErr
	}}
	m.RegisterRoutes(api)
	resp := api.Put("/environments/0/auto-heal", map[string]any{"enabled": true, "maxRestarts": 3, "windowMinutes": 30, "excludedStacks": []string{"app"}, "excludedContainers": []string{"web"}})
	require.Equal(t, 200, resp.Code, resp.Body.String())
	require.Contains(t, api.Get("/environments/0/auto-heal").Body.String(), `"enabled":true`)
	for _, body := range []string{`{"maxRestarts":0,"windowMinutes":30}`, `{"maxRestarts":"3","windowMinutes":30}`, `{"maxRestarts":3,"windowMinutes":30,"extra":true}`} {
		resp = api.Put("/environments/0/auto-heal", strings.NewReader(body))
		require.Equal(t, 400, resp.Code, resp.Body.String())
		require.Contains(t, resp.Body.String(), "autoheal.settings-invalid")
	}
	require.Equal(t, 200, api.Get("/environments/remote/auto-heal").Code)
	require.Equal(t, 200, api.Put("/environments/remote/auto-heal", strings.NewReader(`{"maxRestarts":3,"windowMinutes":30}`)).Code)
	require.Equal(t, 2, calls)
	proxyErr = fmt.Errorf("wrapped: %w", remenv.ErrAgentUpdateRequired)
	resp = api.Get("/environments/remote/auto-heal")
	require.Equal(t, 409, resp.Code)
	require.JSONEq(t, `{"success":false,"code":"agent.update-required"}`, resp.Body.String())
	proxyErr = &remenv.StatusError{StatusCode: 409, Body: []byte(`{"code":"autoheal.settings-env-managed","status":409}`)}
	resp = api.Put("/environments/remote/auto-heal", strings.NewReader(`{"maxRestarts":3,"windowMinutes":30}`))
	require.Equal(t, 409, resp.Code)
	require.Contains(t, resp.Body.String(), "autoheal.settings-env-managed")
}
