// Source: getarcaneapp/arcane/backend/internal/settings/model_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed OIDC and passkeys.

package settings

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettings_ToSettingVariableSlice_Visibility(t *testing.T) {
	settings := &Settings{
		AuthLocalEnabled:      SettingVariable{Value: "true"},
		DockerHost:            SettingVariable{Value: "unix:///var/run/docker.sock"},
		EnableGravatar:        SettingVariable{Value: "true"},
		AvatarMaxUploadSizeMb: SettingVariable{Value: "2"},
	}

	publicKeys := settingKeysFromSliceInternal(settings.ToSettingVariableSlice(SettingVisibilityPublic, true))
	require.Contains(t, publicKeys, "authLocalEnabled")
	require.NotContains(t, publicKeys, "dockerHost")
	require.NotContains(t, publicKeys, "enableGravatar")

	nonAdminKeys := settingKeysFromSliceInternal(settings.ToSettingVariableSlice(SettingVisibilityNonAdmin, true))
	require.Contains(t, nonAdminKeys, "dockerHost")
	require.Contains(t, nonAdminKeys, "enableGravatar")
	require.Contains(t, nonAdminKeys, "avatarMaxUploadSizeMb")
	require.NotContains(t, nonAdminKeys, "baseServerUrl")
	require.NotContains(t, nonAdminKeys, "defaultShell")

	allKeys := settingKeysFromSliceInternal(settings.ToSettingVariableSlice(SettingVisibilityAll, true))
	require.Contains(t, allKeys, "authLocalEnabled")
}

func settingKeysFromSliceInternal(settings []SettingVariable) map[string]string {
	keys := make(map[string]string, len(settings))
	for _, setting := range settings {
		keys[setting.Key] = setting.Value
	}

	return keys
}
