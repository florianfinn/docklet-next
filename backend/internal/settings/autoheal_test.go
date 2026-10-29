// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package settings

import (
	"context"
	dto "github.com/florianfinn/docklet-next/types/autoheal"
	settingsDTO "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestAutoHealGenericWritesAreAtomicAndIntervalRemainsInternal(t *testing.T) {
	for _, u := range []settingsDTO.Update{{AutoHealEnabled: new("true")}, {AutoHealInterval: new("0 * * * * *")}, {AutoHealExcludedStacks: new("app")}, {AutoHealExcludedContainers: new("web")}, {AutoHealMaxRestarts: new("1")}, {AutoHealRestartWindow: new("10")}} {
		s, e := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
		require.NoError(t, e)
		before := s.GetStringSetting(t.Context(), "hubName", "system")
		u.HubName = new("dark")
		_, e = s.UpdateSettings(t.Context(), u)
		require.Error(t, e)
		require.Equal(t, before, s.GetStringSetting(t.Context(), "hubName", "system"))
		require.NoError(t, s.UpdateSetting(t.Context(), "autoHealInterval", "0 */6 * * * *"))
		require.Equal(t, "0 */6 * * * *", s.GetStringSetting(t.Context(), "autoHealInterval", ""))
	}
}
func TestAutoHealEffectiveEnvironmentPresenceByRole(t *testing.T) {
	for _, role := range []string{"hub", "direct", "edge", "managed-hub"} {
		for _, presence := range []string{"unset", "false", "empty", "file", "double-file", "unreadable"} {
			t.Run(role+"/"+presence, func(t *testing.T) {
				t.Setenv("AGENT_MODE", map[bool]string{true: "true", false: "false"}[role == "direct"])
				t.Setenv("EDGE_AGENT", map[bool]string{true: "true", false: "false"}[role == "edge"])
				t.Setenv("UI_CONFIGURATION_DISABLED", map[bool]string{true: "true", false: "false"}[role == "managed-hub"])
				for _, n := range []string{"AUTO_HEAL_ENABLED", "AUTO_HEAL_INTERVAL", "AUTO_HEAL_EXCLUDED_CONTAINERS", "AUTO_HEAL_EXCLUDED_STACKS", "AUTO_HEAL_MAX_RESTARTS", "AUTO_HEAL_RESTART_WINDOW"} {
					for _, suffix := range []string{"", "_FILE", "__FILE"} {
						key := n + suffix
						old, ok := os.LookupEnv(key)
						require.NoError(t, os.Unsetenv(key))
						t.Cleanup(func() {
							if ok {
								_ = os.Setenv(key, old)
							} else {
								_ = os.Unsetenv(key)
							}
						})
					}
				}
				switch presence {
				case "false":
					t.Setenv("AUTO_HEAL_ENABLED", "false")
				case "empty":
					t.Setenv("AUTO_HEAL_EXCLUDED_STACKS", "")
				case "file", "double-file":
					p := filepath.Join(t.TempDir(), "value")
					require.NoError(t, os.WriteFile(p, []byte("false"), 0600))
					suffix := "_FILE"
					if presence == "double-file" {
						suffix = "__FILE"
					}
					t.Setenv("AUTO_HEAL_ENABLED"+suffix, p)
				case "unreadable":
					t.Setenv("AUTO_HEAL_ENABLED_FILE", filepath.Join(t.TempDir(), "missing"))
				}
				want := role != "hub" && presence != "unset" && presence != "unreadable"
				require.Equal(t, want, AutoHealManagedByEnvironment())
				s, e := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
				require.NoError(t, e)
				var before []SettingVariable
				require.NoError(t, s.db.Order("key").Find(&before).Error)
				_, e = s.UpdateAutoHeal(context.Background(), dto.Update{Enabled: true, MaxRestarts: 3, WindowMinutes: 30})
				if want {
					require.Error(t, e)
					var after []SettingVariable
					require.NoError(t, s.db.Order("key").Find(&after).Error)
					require.Equal(t, before, after)
				} else {
					require.NoError(t, e)
				}
			})
		}
	}
}
