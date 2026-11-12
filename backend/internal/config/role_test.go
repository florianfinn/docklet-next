// Source: docklet-next/issues/299
// Revision: 8696c990fba68be0dd3ca9ac4de61434f6c56131
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original role reader tests.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadRoleMatchesLoadWithoutGlobals(t *testing.T) {
	t.Setenv("EDGE_AGENT", "true")
	t.Setenv("UI_CONFIGURATION_DISABLED", "true")
	role := LoadRole()
	require.Equal(t, Role{AgentMode: true, EdgeAgent: true, UIConfigurationDisabled: true, Environment: string(Load().Environment)}, role)
}

// Notification, Auto Heal and settings-loading paths run per transaction or concurrently; Load writes globals (#299).
func TestHotPathsDoNotCallLoad(t *testing.T) {
	for _, path := range []string{"../activity/runtime_notifications.go", "../settings/autoheal.go", "../settings/service_loading.go", "../settings/service_defaults.go", "../volume/children/backup/service.go"} {
		data, err := os.ReadFile(filepath.FromSlash(path))
		require.NoError(t, err)
		require.False(t, strings.Contains(string(data), "config.Load()"), path)
	}
}
