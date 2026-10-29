// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package backup

import (
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutoHealVolumeOwnershipIncludesEveryLocalSource(t *testing.T) {
	for _, role := range []string{"hub", "agent"} {
		for _, source := range []string{"writer-fence", "startup-fence", "workspace", "backup", "restore-source", "restore-destination", "read-error", "lease"} {
			t.Run(role+"/"+source, func(t *testing.T) {
				s := newRuntimePlan(t).service
				held, e := s.HeldForAutoHeal(t.Context(), []string{"data"})
				require.NoError(t, e)
				require.False(t, held)
				switch source {
				case "lease":
					lease, ok, err := s.deps.Engine.TryAcquireRun(t.Context(), "volume-backup", "data")
					require.NoError(t, err)
					require.True(t, ok)
					defer lease.Release(t.Context())
				case "writer-fence":
					s.writerFences["data"] = true
				case "startup-fence":
					s.writerDiscoveryPending = true
				case "workspace":
					unlock, ok := s.deps.Locks.TryLock("data")
					require.True(t, ok)
					defer unlock()
				case "backup":
					require.NoError(t, s.deps.DB.Table("volume_backups").Create(&volume.Backup{ID: "child", VolumeName: "data", Status: "running"}).Error)
				case "restore-source", "restore-destination":
					row := restore.Run{ID: "restore", VolumeName: "data", Status: "running"}
					if source == "restore-destination" {
						row.VolumeName = "original"
						row.NewVolumeName = "data"
					}
					require.NoError(t, s.deps.DB.Create(&row).Error)
				case "read-error":
					require.NoError(t, s.deps.DB.Migrator().DropTable("volume_backups"))
				}
				held, e = s.HeldForAutoHeal(t.Context(), []string{"data"})
				require.True(t, held)
				if source == "read-error" {
					require.Error(t, e)
				} else {
					require.NoError(t, e)
				}
			})
		}
	}
}
