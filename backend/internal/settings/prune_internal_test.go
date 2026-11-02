// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original regression for startup pruning of internal backup rows (M16 candidate F05).

package settings

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettingsService_PruneUnknownSettings_KeepsInternalBackupState(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDB(t)
	svc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)
	keys := []string{"hubHostId", "backupTargetAuthority", "backupTargetSetVersion", "backupTargetContentHash", "volumeBackupRepositoryCheck"}
	for _, key := range keys {
		require.NoError(t, svc.db.WithContext(ctx).Create(&SettingVariable{Key: key, Value: "synced-" + key}).Error)
	}

	require.NoError(t, svc.PruneUnknownSettings(ctx))

	var kept []string
	require.NoError(t, svc.db.WithContext(ctx).Model(&SettingVariable{}).Where("key IN ?", keys).Order("key").Pluck("key", &kept).Error)
	require.Equal(t, []string{"backupTargetAuthority", "backupTargetContentHash", "backupTargetSetVersion", "hubHostId", "volumeBackupRepositoryCheck"}, kept)
}
