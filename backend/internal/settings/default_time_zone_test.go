// Source: docklet-next/issues/410
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original process-zone and user-zone read proofs.

package settings

import (
	"context"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	dto "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
)

// The reset action names the process TZ while an override is saved; agents never
// report a hub default.
func TestSettingsReadReportsProcessZoneBesideOverride(t *testing.T) {
	s, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	cfg := &config.Config{Timezone: "Europe/Berlin"}
	s.BindZoneProvider(cfg)
	_, err = s.UpdateSettings(t.Context(), dto.Update{TimeZone: new("Asia/Tokyo")})
	require.NoError(t, err)
	h := &SettingsHandler{settingsService: s, cfg: cfg}
	values := runtimeSettingKeysInternal(h.appendRuntimeSettingsInternal(nil, true))
	require.Equal(t, "Asia/Tokyo", values["effectiveTimeZone"])
	require.Equal(t, "Europe/Berlin", values["defaultTimeZone"])

	for _, agent := range []*config.Config{{AgentMode: true, Timezone: "Asia/Tokyo"}, {EdgeAgent: true, Timezone: "Asia/Tokyo"}} {
		h := &SettingsHandler{settingsService: s, cfg: agent}
		require.NotContains(t, runtimeSettingKeysInternal(h.appendRuntimeSettingsInternal(nil, true)), "defaultTimeZone")
	}
}

// Simple users see schedule times in the hub zone, not a UTC fallback.
func TestUserSettingsReadCarriesHubZone(t *testing.T) {
	s, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	cfg := &config.Config{Timezone: "Europe/Berlin"}
	s.BindZoneProvider(cfg)
	_, err = s.UpdateSettings(t.Context(), dto.Update{TimeZone: new("Asia/Tokyo")})
	require.NoError(t, err)
	h := &SettingsHandler{settingsService: s, cfg: cfg}
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, access.Synthesize("viewer", nil))
	out, err := h.GetSettings(ctx, &GetSettingsInput{EnvironmentID: "0"})
	require.NoError(t, err)
	values := runtimeSettingKeysInternal(out.Body)
	require.Equal(t, "Asia/Tokyo", values["effectiveTimeZone"])
	require.NotContains(t, values, "defaultTimeZone")
	require.NotContains(t, values, "timeZone")
}

func TestProcessZoneMapsUnnamedZonesToUTC(t *testing.T) {
	for _, tz := range []string{"Local", "Factory", " Local "} {
		cfg := &config.Config{Timezone: tz}
		require.Equal(t, "UTC", cfg.GetLocation().String(), tz)
		require.Equal(t, "UTC", cfg.ProcessLocation().String(), tz)
	}
	require.Equal(t, "Europe/Berlin", (&config.Config{Timezone: "Europe/Berlin"}).ProcessLocation().String())
}
