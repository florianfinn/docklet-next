// Source: docklet-next/issues/675
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original startup and generic save cron regression coverage.

package settings

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCronStartupRecoveryAndGenericSave(t *testing.T) {
	db := setupSettingsTestDB(t)
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	keys := []string{"eventCleanupInterval", "expiredSessionsCleanupInterval", "dockerClientRefreshInterval", "scheduledPruneInterval", "environmentHealthInterval", "autoHealInterval", "imageAutoPatchInterval"}
	require.NoError(t, svc.EnsureDefaultSettings(t.Context()))
	for _, key := range keys {
		require.NoError(t, db.Model(&SettingVariable{}).Where("key = ?", key).Update("value", "invalid cron").Error)
	}
	require.NoError(t, svc.EnsureDefaultSettings(t.Context()))
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			var stored SettingVariable
			require.NoError(t, db.Where("key = ?", key).First(&stored).Error)
			require.NotEqual(t, "invalid cron", stored.Value)
			require.Error(t, svc.UpdateSetting(t.Context(), key, "invalid cron"))
		})
	}
}

func TestGenericCronSaveRejectsAdditionalLiveKeys(t *testing.T) {
	db := setupSettingsTestDB(t)
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	require.NoError(t, svc.EnsureDefaultSettings(t.Context()))
	for _, key := range []string{"autoHealInterval", "imageAutoPatchInterval"} {
		require.Error(t, svc.UpdateSetting(t.Context(), key, "invalid cron"), key)
	}
}
