// Source: docklet-next/issues/329
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package settings

import (
	"github.com/florianfinn/docklet-next/backend/internal/common"
	dto "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBackupDefaultRetentionSetting(t *testing.T) {
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	require.NoError(t, svc.EnsureDefaultSettings(t.Context()))
	require.Equal(t, "7", svc.GetSettingsConfig().BackupDefaultRetentionCount.Value)
	for _, value := range []string{"0", "7", "3650"} {
		_, err = svc.UpdateSettings(t.Context(), dto.Update{BackupDefaultRetentionCount: new(value)})
		require.NoError(t, err)
		require.Equal(t, value, svc.GetSettingsConfig().BackupDefaultRetentionCount.Value)
	}
	for _, value := range []string{"-1", "3651", "4000", "", "1.5", "all"} {
		_, err = svc.UpdateSettings(t.Context(), dto.Update{BackupDefaultRetentionCount: new(value)})
		require.ErrorIs(t, err, common.ErrValidation)
		require.Equal(t, "3650", svc.GetSettingsConfig().BackupDefaultRetentionCount.Value)
	}
}
