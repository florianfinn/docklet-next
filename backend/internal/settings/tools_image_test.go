// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 60d2471b62b12e1af64c54060a50102a2e39d1c1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original per-environment helper override tests for issue #179.
package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
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
			require.NoError(t, svc.UpdateSetting(t.Context(), "toolsImage", "registry.example.com/hub-tools:0.1.0"))
			got, err := svc.GetSettings(t.Context())
			require.NoError(t, err)
			require.Equal(t, configured, got.ToolsImage.Value)
			require.Equal(t, configured, svc.GetSettingsConfig().ToolsImage.Value)
			require.True(t, svc.IsEnvOverrideActive("toolsImage"))
		})
	}
}

func TestToolsImageReleaseDefaultAndPersistedReference(t *testing.T) {
	require.Equal(t, volumehelper.DefaultToolsImage, DefaultSettingsConfig().ToolsImage.Value)
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	require.NoError(t, svc.EnsureDefaultSettings(t.Context()))
	require.NoError(t, svc.UpdateSetting(t.Context(), "toolsImage", "registry.example.com/tools:0.2.0"))
	require.Equal(t, "registry.example.com/tools:0.2.0", svc.GetSettingsConfig().ToolsImage.Value)
}
