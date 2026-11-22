// Source: docklet-next/issues/570
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original settings consumer proof for the volume-owned read-only resolver.; M36 #570: Docklet backup volume and runtime names.

package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBackupVolumeSettingsReader(t *testing.T) {
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	calls := 0
	var readErr error
	svc.SetBackupVolumeNameReader(func(context.Context) (string, error) { calls++; return "retained-backups", readErr })
	h := &SettingsHandler{settingsService: svc, cfg: &config.Config{BackupVolumeName: "ignored-backups"}}
	public, err := h.GetPublicSettings(t.Context(), &GetPublicSettingsInput{EnvironmentID: "0"})
	require.NoError(t, err)
	require.NotContains(t, runtimeSettingKeysInternal(public.Body), "backupVolumeName")
	require.Zero(t, calls)
	result, err := h.GetSettings(t.Context(), &GetSettingsInput{EnvironmentID: "0"})
	require.NoError(t, err)
	require.Equal(t, "retained-backups", runtimeSettingKeysInternal(result.Body)["backupVolumeName"])
	require.Equal(t, 1, calls)
	// An unreadable selection omits the display-only value; the rest of the list stays available.
	readErr = errors.New("synthetic inspect failure")
	result, err = h.GetSettings(t.Context(), &GetSettingsInput{EnvironmentID: "0"})
	require.NoError(t, err)
	keys := runtimeSettingKeysInternal(result.Body)
	require.NotContains(t, keys, "backupVolumeName")
	require.Greater(t, len(result.Body), 1)
}
