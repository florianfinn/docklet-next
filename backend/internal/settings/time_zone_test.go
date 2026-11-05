// Source: docklet-next/issues/319
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original persisted hub zone validation and process ownership tests.

package settings

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
)

func TestHubTimeZoneLiveSaveAndRestart(t *testing.T) {
	db := setupSettingsTestDB(t)
	s, err := newSettingsServiceForTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	cfg := &config.Config{Timezone: "Europe/Berlin"}
	s.BindZoneProvider(cfg)
	h := &SettingsHandler{settingsService: s, cfg: cfg}
	for _, zone := range []string{"UTC", "Asia/Tokyo", "Local", ""} {
		_, err = h.UpdateSettings(t.Context(), &UpdateSettingsInput{EnvironmentID: "0", Body: dto.Update{TimeZone: new(zone)}})
		require.NoError(t, err)
		want := zone
		if want == "" {
			want = "Europe/Berlin"
		}
		loc, err := time.LoadLocation(want)
		require.NoError(t, err)
		want = loc.String()
		require.Equal(t, want, cfg.Location().String())
		require.Equal(t, zone, s.GetSettingsConfig().TimeZone.Value)
		require.Equal(t, want, runtimeSettingKeysInternal(h.appendRuntimeSettingsInternal(nil, true))["effectiveTimeZone"])
	}
	_, err = h.UpdateSettings(t.Context(), &UpdateSettingsInput{EnvironmentID: "0", Body: dto.Update{TimeZone: new("Mars/Base")}})
	var problem *imageDTO.OperationError
	require.ErrorAs(t, err, &problem)
	require.Equal(t, 400, problem.GetStatus())
	require.Equal(t, "settings.invalid-time-zone", problem.Problem.Code)
	require.Equal(t, "Europe/Berlin", cfg.Location().String())
	_, err = s.UpdateSettings(t.Context(), dto.Update{TimeZone: new("Asia/Tokyo")})
	require.NoError(t, err)
	restarted, err := newSettingsServiceForTestInternal(t, context.WithoutCancel(t.Context()), db)
	require.NoError(t, err)
	restartedCfg := &config.Config{Timezone: "Europe/Berlin"}
	restarted.BindZoneProvider(restartedCfg)
	require.Equal(t, "Asia/Tokyo", restartedCfg.Location().String())
}

func TestRemoteZonesRemainOwnedByAgentTZ(t *testing.T) {
	for _, cfg := range []*config.Config{{AgentMode: true, Timezone: "Asia/Tokyo"}, {EdgeAgent: true, Timezone: "America/New_York"}} {
		s, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
		require.NoError(t, err)
		s.BindZoneProvider(cfg)
		_, err = s.UpdateSettings(t.Context(), dto.Update{TimeZone: new("Europe/Berlin")})
		require.NoError(t, err)
		require.Equal(t, cfg.Timezone, cfg.Location().String())
		h := &SettingsHandler{settingsService: s, cfg: cfg}
		_, err = h.UpdateSettings(t.Context(), &UpdateSettingsInput{EnvironmentID: "0", Body: dto.Update{TimeZone: new("UTC")}})
		require.Error(t, err)
		require.NotContains(t, runtimeSettingKeysInternal(h.appendRuntimeSettingsInternal([]dto.PublicSetting{{Key: "timeZone", Value: "Europe/Berlin"}}, true)), "timeZone")
	}
}
