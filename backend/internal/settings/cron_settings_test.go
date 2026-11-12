// Source: docklet-next/issues/407
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original settings-service proof for the docker client refresh cron key.

package settings

import (
	"testing"

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
