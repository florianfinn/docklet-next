// Source: docklet-next/issues/338
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original sustained-load setting bounds and defaults tests.

package settings

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLoadAlertDefaultsAndValidation(t *testing.T) {
	defaults := DefaultSettingsConfig()
	require.True(t, defaults.LoadAlertsEnabled.IsTrue())
	require.Equal(t, 90, defaults.LoadAlertCpuPercent.AsInt())
	require.Equal(t, 90, defaults.LoadAlertMemoryPercent.AsInt())
	require.Equal(t, 10, defaults.LoadAlertMinutes.AsInt())
	for _, tc := range []struct {
		key, value string
		valid      bool
	}{{"loadAlertsEnabled", "true", true}, {"loadAlertsEnabled", "false", true}, {"loadAlertsEnabled", "yes", false}, {"loadAlertCpuPercent", "49", false}, {"loadAlertCpuPercent", "50", true}, {"loadAlertMemoryPercent", "100", true}, {"loadAlertMemoryPercent", "101", false}, {"loadAlertMinutes", "4", false}, {"loadAlertMinutes", "5", true}, {"loadAlertMinutes", "60", true}, {"loadAlertMinutes", "61", false}, {"loadAlertMinutes", "5.5", false}} {
		err := validateSettingValueInternal(tc.key, tc.value)
		require.Equal(t, tc.valid, err == nil, tc.key+"="+tc.value)
	}
}
