// Source: getarcaneapp/arcane/backend/internal/settings/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed template catalogue; functional cut: removed GitOps and saved Git repositories; functional cut: removed Swarm; functional cut: removed OIDC and passkeys; SQLite temp-file persistence fixture; Complete helper image default assertions.

package settings

import (
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/types/features"
	"github.com/florianfinn/docklet-next/types/settings"
	"net/http"
	"testing"
)
import "github.com/stretchr/testify/require"

func TestSettingsService_FeatureDefaultsAndUnknownFeature(t *testing.T) {
	var svc *SettingsService
	require.True(t, svc.IsFeatureEnabled(t.Context(), features.VulnerabilityManagement))
	require.NoError(t, svc.RequireFeature(t.Context(), features.VulnerabilityManagement))
	require.False(t, svc.IsFeatureEnabled(t.Context(), features.ID("unknown")))

	svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	require.True(t, svc.IsFeatureEnabled(t.Context(), features.VulnerabilityManagement))
	require.NoError(t, svc.EnsureDefaultSettings(t.Context()))
	var stored SettingVariable
	require.NoError(t, svc.db.Where("key = ?", features.VulnerabilityManagementSettingKey).First(&stored).Error)
	require.Equal(t, "true", stored.Value)
	require.Contains(t, svc.ListSettings(SettingVisibilityPublic), SettingVariable{Key: features.VulnerabilityManagementSettingKey, Value: "true"})
}

func TestSettingsService_FeaturePersistenceAndEnvironmentIsolation(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDB(t)
	svc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)
	other, err := newSettingsServiceForTestInternal(t, ctx, setupSettingsTestDB(t))
	require.NoError(t, err)

	var notifications int
	unsubscribe := svc.SubscribeSettingsChanges([]string{features.VulnerabilityManagementSettingKey}, func([]libarcane.SettingUpdate) { notifications++ })
	defer unsubscribe()
	scheduled := "true"
	ignore := "CVE-2026-12345"
	require.Error(t, svc.UpdateSetting(ctx, "trivyIgnore", ignore))
	_, err = svc.UpdateSettings(ctx, settings.Update{VulnerabilityScanEnabled: &scheduled})
	require.NoError(t, err)
	disabled := "false"
	_, err = svc.UpdateSettings(ctx, settings.Update{FeatureVulnerabilityManagementEnabled: &disabled})
	require.NoError(t, err)
	waitForSettingsNotificationsInternal(t, svc)
	require.Equal(t, 1, notifications)
	require.False(t, svc.IsFeatureEnabled(ctx, features.VulnerabilityManagement))
	require.True(t, other.IsFeatureEnabled(ctx, features.VulnerabilityManagement))
	err = svc.RequireFeature(ctx, features.VulnerabilityManagement)
	require.ErrorIs(t, err, common.ErrFeatureDisabled)
	require.ErrorIs(t, err, common.ErrForbidden)
	require.Contains(t, err.Error(), string(features.VulnerabilityManagement))
	apiErr := common.ToAPIError(err)
	require.Equal(t, http.StatusForbidden, apiErr.HTTPStatus())
	require.Equal(t, common.APIErrorCodeFeatureDisabled, apiErr.Code)

	reloaded, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)
	require.False(t, reloaded.IsFeatureEnabled(ctx, features.VulnerabilityManagement))
	enabled := "true"
	_, err = svc.UpdateSettings(ctx, settings.Update{FeatureVulnerabilityManagementEnabled: &enabled})
	require.NoError(t, err)
	require.True(t, svc.IsFeatureEnabled(ctx, features.VulnerabilityManagement))
	cfg, err := svc.GetSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, scheduled, cfg.VulnerabilityScanEnabled.Value)
	require.Empty(t, cfg.TrivyIgnore.Value)
}

func TestSettingsService_FeatureEnvironmentOverride(t *testing.T) {
	t.Setenv("FEATURE_VULNERABILITY_MANAGEMENT_ENABLED", "false")
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	enabled := "true"
	_, err = svc.UpdateSettings(t.Context(), settings.Update{FeatureVulnerabilityManagementEnabled: &enabled})
	require.NoError(t, err)
	require.True(t, svc.IsEnvOverrideActive(features.VulnerabilityManagementSettingKey))
	require.False(t, svc.IsFeatureEnabled(t.Context(), features.VulnerabilityManagement))
	require.Contains(t, svc.ListSettings(SettingVisibilityPublic), SettingVariable{Key: features.VulnerabilityManagementSettingKey, Value: "false"})
}

func TestSettingsService_RejectInvalidFeatureBoolean(t *testing.T) {
	for _, invalid := range []string{"", "yes", "0", "TRUE"} {
		t.Run(invalid, func(t *testing.T) {
			svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
			require.NoError(t, err)
			_, err = svc.UpdateSettings(t.Context(), settings.Update{FeatureVulnerabilityManagementEnabled: &invalid})
			require.ErrorIs(t, err, common.ErrValidation)
			err = svc.UpdateSetting(t.Context(), features.VulnerabilityManagementSettingKey, invalid)
			require.ErrorIs(t, err, common.ErrValidation)
			err = svc.UpdateSettingValues(t.Context(), []libarcane.SettingUpdate{{Key: features.VulnerabilityManagementSettingKey, Value: invalid}})
			require.ErrorIs(t, err, common.ErrValidation)
			require.True(t, svc.IsFeatureEnabled(t.Context(), features.VulnerabilityManagement))
		})
	}
}

func TestSettingsService_GetSettings_EnvOverride_TrivyResourceLimits(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDB(t)
	t.Setenv("TRIVY_RESOURCE_LIMITS_ENABLED", "false")
	t.Setenv("TRIVY_CPU_LIMIT", "2.5")
	t.Setenv("TRIVY_MEMORY_LIMIT_MB", "2048")

	svc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, svc.EnsureDefaultSettings(ctx))

	settingsCfg, err := svc.GetSettings(ctx)
	require.NoError(t, err)
	require.False(t, settingsCfg.TrivyResourceLimitsEnabled.IsTrue())
	require.Equal(t, "2.5", settingsCfg.TrivyCpuLimit.Value)
	require.Equal(t, 2048, settingsCfg.TrivyMemoryLimitMb.AsInt())
}
