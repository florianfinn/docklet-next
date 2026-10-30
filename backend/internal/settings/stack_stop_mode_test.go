// Source: docklet-next/issues/316
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original hub-only validated stack stop mode proof.
package settings

import (
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	dto "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStackStopModeDefaultEnumAndHubScope(t *testing.T) {
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	require.NoError(t, svc.EnsureDefaultSettings(t.Context()))
	require.Equal(t, "down", svc.GetSettingsConfig().StackStopMode.Value)
	for _, value := range []string{"stop", "down"} {
		_, err = svc.UpdateSettings(t.Context(), dto.Update{StackStopMode: new(value)})
		require.NoError(t, err)
		require.Equal(t, value, svc.GetSettingsConfig().StackStopMode.Value)
	}
	for _, value := range []string{"", "STOP", "remove", "down,stop"} {
		_, err = svc.UpdateSettings(t.Context(), dto.Update{StackStopMode: new(value)})
		require.ErrorIs(t, err, common.ErrValidation)
	}
	for _, cfg := range []*config.Config{{}, {AgentMode: true}, {EdgeAgent: true}} {
		handler := &SettingsHandler{settingsService: svc, cfg: cfg}
		for _, env := range []string{"0", "example-edge"} {
			if env == "0" && !cfg.AgentMode && !cfg.EdgeAgent {
				continue
			}
			_, err = handler.UpdateSettings(t.Context(), &UpdateSettingsInput{EnvironmentID: env, Body: dto.Update{StackStopMode: new("stop")}})
			require.Error(t, err, "must refuse before proxy dispatch")
		}
		local, err := handler.GetSettings(t.Context(), &GetSettingsInput{EnvironmentID: "0"})
		require.NoError(t, err)
		found := false
		for _, setting := range local.Body {
			if setting.Key == "stackStopMode" {
				found = true
			}
		}
		require.Equal(t, !cfg.AgentMode && !cfg.EdgeAgent, found)
	}
}
