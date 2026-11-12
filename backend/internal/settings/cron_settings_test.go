// Source: docklet-next/issues/407
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original settings-service and route proofs for refused cron values.

package settings

import (
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	dto "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
)

func TestSettingsService_UpdateSettingRejectsInvalidDockerClientRefreshInterval(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDB(t)
	svc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)

	require.NoError(t, svc.UpdateSetting(ctx, "dockerClientRefreshInterval", "0 */10 * * * *"))
	for _, value := range []string{"@daily", "0 3 * * *", "not a cron schedule"} {
		require.ErrorContains(t, svc.UpdateSetting(ctx, "dockerClientRefreshInterval", value), "invalid cron expression for dockerClientRefreshInterval", value)
	}

	var setting SettingVariable
	require.NoError(t, svc.db.WithContext(ctx).Where("key = ?", "dockerClientRefreshInterval").First(&setting).Error)
	require.Equal(t, "0 */10 * * * *", setting.Value)
}

// The settings route answers a refused cron value as a 400 validation problem, not 500.
func TestSettingsRouteRefusesInvalidCronAsValidation(t *testing.T) {
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	h := &SettingsHandler{settingsService: svc, cfg: &config.Config{}}
	for _, body := range []dto.Update{{DockerClientRefreshInterval: new("@daily")}, {EnvironmentHealthInterval: new("0 3 * * *")}} {
		_, err = h.UpdateSettings(t.Context(), &UpdateSettingsInput{EnvironmentID: "0", Body: body})
		var status huma.StatusError
		require.True(t, errors.As(err, &status), "%v", err)
		require.Equal(t, http.StatusBadRequest, status.GetStatus())
		require.Contains(t, err.Error(), "invalid cron expression")
	}
}
