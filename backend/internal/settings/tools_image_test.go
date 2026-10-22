// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 60d2471b62b12e1af64c54060a50102a2e39d1c1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original per-environment helper override tests for issue #179.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	settingstypes "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
)

func TestToolsImageEnvironmentOverrideWinsOverSyncedSetting(t *testing.T) {
	for _, suffix := range []string{"", "_FILE", "__FILE"} {
		t.Run("TOOLS_IMAGE"+suffix, func(t *testing.T) {
			for _, key := range []string{"TOOLS_IMAGE", "TOOLS_IMAGE_FILE", "TOOLS_IMAGE__FILE"} {
				t.Setenv(key, "")
			}
			configured := "registry.example.com/tools:latest"
			value := configured
			if suffix != "" {
				value = filepath.Join(t.TempDir(), "tools-image")
				require.NoError(t, os.WriteFile(value, []byte(configured), 0600))
			}
			t.Setenv("TOOLS_IMAGE"+suffix, value)
			svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
			require.NoError(t, err)
			require.NoError(t, svc.EnsureDefaultSettings(t.Context()))
			_, err = svc.UpdateSettings(t.Context(), settingstypes.Update{ToolsImage: new("registry.example.com/hub-tools:0.1.0")})
			require.NoError(t, err)
			got, err := svc.GetSettings(t.Context())
			require.NoError(t, err)
			require.Equal(t, configured, got.ToolsImage.Value)
			require.Equal(t, configured, svc.GetSettingsConfig().ToolsImage.Value)
			require.True(t, svc.IsEnvOverrideActive("toolsImage"))
			agentConfig, err := svc.loadDatabaseConfigFromEnv(t.Context(), svc.db)
			require.NoError(t, err)
			require.Equal(t, configured, agentConfig.ToolsImage.Value)
		})
	}
}

func TestToolsImageReleaseDefaultAndPersistedReference(t *testing.T) {
	require.Empty(t, DefaultSettingsConfig().ToolsImage.Value)
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	require.NoError(t, svc.EnsureDefaultSettings(t.Context()))
	require.NoError(t, svc.UpdateSetting(t.Context(), "toolsImage", "registry.example.com/tools:0.2.0"))
	require.Equal(t, "registry.example.com/tools:0.2.0", svc.GetSettingsConfig().ToolsImage.Value)
}

// Adapted from the independent R1 review: restart against the same database
// after an earlier release persisted its default without an operator override.
func TestReview179ToolsImageDefaultFollowsHubRelease(t *testing.T) {
	for _, key := range []string{"TOOLS_IMAGE", "TOOLS_IMAGE_FILE", "TOOLS_IMAGE__FILE"} {
		t.Setenv(key, "")
	}
	for _, old := range []string{"", "ghcr.io/getarcaneapp/tools:latest", "ghcr.io/florianfinn/docklet-tools:0.1.0"} {
		t.Run(old, func(t *testing.T) {
			db := setupSettingsTestDB(t)
			first, err := newSettingsServiceForTestInternal(t, t.Context(), db)
			require.NoError(t, err)
			require.NoError(t, first.EnsureDefaultSettings(t.Context()))
			if old != "" {
				require.NoError(t, db.Model(&SettingVariable{}).Where("key = ?", "toolsImage").Update("value", old).Error)
			}
			upgraded, err := newSettingsServiceForTestInternal(t, t.Context(), db)
			require.NoError(t, err)
			require.NoError(t, upgraded.EnsureDefaultSettings(t.Context()))
			require.NoError(t, upgraded.LoadDatabaseSettings(t.Context()))
			var persisted SettingVariable
			require.NoError(t, db.Where("key = ?", "toolsImage").First(&persisted).Error)
			require.Empty(t, persisted.Value, "a shipped default is never an explicit override")
			require.Empty(t, upgraded.GetSettingsConfig().ToolsImage.Value, "settings transport must preserve the empty override")
			require.Equal(t, volumehelper.DefaultToolsImage, volumehelper.ToolsImage(upgraded.GetSettingsConfig().ToolsImage.Value), "resolve the running release default at read time")
		})
	}
}

func TestToolsImageSyncPreservesEmptyAndExplicitOverride(t *testing.T) {
	for _, key := range []string{"TOOLS_IMAGE", "TOOLS_IMAGE_FILE", "TOOLS_IMAGE__FILE"} {
		t.Setenv(key, "")
	}
	hub, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	require.NoError(t, hub.EnsureDefaultSettings(t.Context()))
	require.NoError(t, hub.LoadDatabaseSettings(t.Context()))
	agent, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	require.NoError(t, agent.EnsureDefaultSettings(t.Context()))
	for _, override := range []string{"", "registry.example.com/tools:latest", ""} {
		require.NoError(t, hub.UpdateSetting(t.Context(), "toolsImage", override))
		// The settings transport carries the configured value, never a resolved default.
		wire, err := json.Marshal(settingstypes.Update{ToolsImage: new(hub.GetSettingsConfig().ToolsImage.Value)})
		require.NoError(t, err)
		var input settingstypes.Update
		require.NoError(t, json.Unmarshal(wire, &input))
		require.Equal(t, override, *input.ToolsImage)
		_, err = agent.UpdateSettings(t.Context(), input)
		require.NoError(t, err)
		require.Equal(t, override, agent.GetSettingsConfig().ToolsImage.Value)
		agentConfig, err := agent.loadDatabaseConfigFromEnv(t.Context(), agent.db)
		require.NoError(t, err)
		require.Equal(t, override, agentConfig.ToolsImage.Value, "agent-mode loading retains only the synced override")
		require.Equal(t, volumehelper.ToolsImage(override), volumehelper.ToolsImage(agent.GetSettingsConfig().ToolsImage.Value))
		require.NoError(t, agent.EnsureDefaultSettings(t.Context()))
		require.NoError(t, agent.LoadDatabaseSettings(t.Context()))
		require.Equal(t, override, agent.GetSettingsConfig().ToolsImage.Value)
	}
}

func TestToolsImageReferenceValidation(t *testing.T) {
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	for _, value := range []string{"", "registry.example.com/tools:latest", "registry.example.com:5000/tools:0.1.0", "registry.example.com/tools@sha256:" + strings.Repeat("a", 64)} {
		require.NoError(t, svc.UpdateSetting(t.Context(), "toolsImage", value), value)
	}
	for _, value := range []string{"bad image", "https://registry.example.com/tools:latest", "UPPERCASE", "registry.example.com/tools@sha256:abc", "registry.example.com/tools:"} {
		require.ErrorContains(t, svc.UpdateSetting(t.Context(), "toolsImage", value), "valid Docker image reference", value)
		_, err := svc.UpdateSettings(t.Context(), settingstypes.Update{ToolsImage: new(value)})
		require.ErrorContains(t, err, "valid Docker image reference", value)
	}
}

func TestToolsImageInvalidEnvironmentReferenceFailsInitialization(t *testing.T) {
	for _, key := range []string{"TOOLS_IMAGE", "TOOLS_IMAGE_FILE", "TOOLS_IMAGE__FILE"} {
		t.Setenv(key, "")
	}
	t.Setenv("TOOLS_IMAGE", "https://registry.example.com/tools:latest")
	_, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.ErrorContains(t, err, "valid Docker image reference")
}
