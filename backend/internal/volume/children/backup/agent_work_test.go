// Source: docklet-next/issues/368
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original agent restart busy-check proofs over the real backup store.

package backup

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

func TestBackupWorkRunningCountsEveryRestartBlocker(t *testing.T) {
	for _, source := range []string{"backup", "bind-snapshot", "restore", "writer-fence", "read-error", "finished"} {
		t.Run(source, func(t *testing.T) {
			s := newRuntimePlan(t).service
			running, err := s.WorkRunning(t.Context())
			require.NoError(t, err)
			require.False(t, running, "an idle host accepts a restart")
			switch source {
			case "backup":
				require.NoError(t, s.deps.DB.Table("volume_backups").Create(&volume.Backup{ID: "child", VolumeName: "data", Status: "running"}).Error)
			case "bind-snapshot":
				require.NoError(t, s.deps.DB.Table("volume_backups").Create(&volume.Backup{ID: "bind", VolumeName: volume.BindKey("/srv/example"), Status: "running"}).Error)
			case "restore":
				require.NoError(t, s.deps.DB.Create(&restore.Run{ID: "restore", VolumeName: "data", Status: "running"}).Error)
			case "writer-fence":
				s.writerFences["data"] = true
			case "read-error":
				require.NoError(t, s.deps.DB.Migrator().DropTable("volume_restore_runs"))
			case "finished":
				require.NoError(t, s.deps.DB.Table("volume_backups").Create(&volume.Backup{ID: "done", VolumeName: "data", Status: "succeeded"}).Error)
			}
			running, err = s.WorkRunning(t.Context())
			if source == "finished" {
				require.NoError(t, err)
				require.False(t, running, "finished work does not block a restart")
				return
			}
			require.True(t, running)
			if source == "read-error" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
