// Source: docklet-next/issues/355
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original retention limits and post-save effect regression guards.

package settings

import (
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	dto "github.com/florianfinn/docklet-next/types/autoheal"
	settingsDTO "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestM19ActivityRangeValidationAndSyncKeys(t *testing.T) {
	for _, key := range []string{"activityEventLimit", "activityRunLimit"} {
		for _, value := range []string{"0", "-1", "1.2", "x", "5001"} {
			err := validateSettingValueInternal(key, value)
			var p *settingsDTO.Problem
			require.ErrorAs(t, err, &p)
			require.Equal(t, "settings.activity-limit-invalid", p.Code)
			require.Equal(t, "/"+key, p.Field)
		}
	}
	for key, values := range map[string][]string{"activityEventLimit": {"50", "5000"}, "activityRunLimit": {"10", "1000"}} {
		for _, value := range values {
			require.NoError(t, validateSettingValueInternal(key, value))
		}
	}
	keys := libarcane.AgentSyncedSettingKeys()
	for _, key := range append(libarcane.TimeoutSettingKeys(), "activityEventLimit", "activityRunLimit") {
		require.Contains(t, keys, key)
	}
	for _, key := range []string{"vulnerabilityScanSchedule", "vulnerabilityNotifyCritical", "vulnerabilityNotifyHigh", "vulnerabilityNotifyFailed", "trivyIgnoreUnfixed"} {
		require.NotContains(t, keys, key)
	}
	keys[0] = "changed"
	require.NotContains(t, libarcane.AgentSyncedSettingKeys(), "changed")
}
func TestM19AutoHealValidityAndOmittedCooldown(t *testing.T) {
	s, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	_, err = s.UpdateSettings(t.Context(), settingsDTO.Update{AutoHealCooldownMinutes: new("9"), HubName: new("changed")})
	require.Error(t, err)
	require.NotEqual(t, "changed", s.GetSettingsConfig().HubName.Value)
	value, err := s.UpdateAutoHeal(t.Context(), dto.Update{MaxRestarts: 3, WindowMinutes: 30, CooldownMinutes: new(17)})
	require.NoError(t, err)
	require.Equal(t, 17, value.CooldownMinutes)
	value, err = s.UpdateAutoHeal(t.Context(), dto.Update{MaxRestarts: 3, WindowMinutes: 30})
	require.NoError(t, err)
	require.Equal(t, 17, value.CooldownMinutes)
	require.Empty(t, value.InvalidFields)
	require.NoError(t, s.UpdateSetting(t.Context(), "autoHealCooldownMinutes", "0"))
	require.NoError(t, s.UpdateSetting(t.Context(), "autoHealMaxRestarts", "11"))
	value, err = s.AutoHealSettings(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"maxRestarts", "cooldownMinutes"}, value.InvalidFields)
	require.Equal(t, value.InvalidFields, (dto.Update{MaxRestarts: value.MaxRestarts, WindowMinutes: value.WindowMinutes, CooldownMinutes: &value.CooldownMinutes}).InvalidFieldNames())
}

func TestM19RetentionAgentEnvironmentOverrideWins(t *testing.T) {
	t.Setenv("AGENT_MODE", "true")
	t.Setenv("ACTIVITY_EVENT_LIMIT", "60")
	s, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	_, err = s.UpdateSettings(t.Context(), settingsDTO.Update{ActivityEventLimit: new("50")})
	require.NoError(t, err)
	require.Equal(t, 60, s.GetIntSetting(t.Context(), "activityEventLimit", 0))
	var stored SettingVariable
	require.NoError(t, s.db.Where("key='activityEventLimit'").First(&stored).Error)
	require.Equal(t, "50", stored.Value)
}

// Environment-managed limits use the write range; 0 would prune every event (#357 S1R1-3).
func TestM19ActivityLimitEnvironmentValuesAreValidated(t *testing.T) {
	ctx := t.Context()
	svc, err := newSettingsServiceForTestInternal(t, ctx, setupSettingsTestDB(t))
	require.NoError(t, err)
	t.Setenv("ACTIVITY_EVENT_LIMIT", "0")
	t.Setenv("ACTIVITY_RUN_LIMIT", "-1")
	loaded, err := svc.loadDatabaseConfigFromEnv(ctx, svc.db)
	require.NoError(t, err)
	require.Equal(t, []string{"500", "100"}, []string{loaded.ActivityEventLimit.Value, loaded.ActivityRunLimit.Value})
	t.Setenv("ACTIVITY_EVENT_LIMIT", "800")
	t.Setenv("ACTIVITY_RUN_LIMIT", "20")
	loaded, err = svc.loadDatabaseConfigFromEnv(ctx, svc.db)
	require.NoError(t, err)
	require.Equal(t, []string{"800", "20"}, []string{loaded.ActivityEventLimit.Value, loaded.ActivityRunLimit.Value})
}
