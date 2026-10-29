// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package backup

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLiveAgentBackupFailureDispatchesAfterSaveWithoutOutbox(t *testing.T) {
	for _, kind := range []string{"manual", "scheduled", "partial", "safety", "cancelled", "retention"} {
		t.Run(kind, func(t *testing.T) {
			f := newRuntimePlan(t)
			db := f.service.deps.DB
			require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
			calls := 0
			f.service.notifier = backupnotify.Notifier{DB: db, Agent: true, Dispatch: func(ctx context.Context, p dto.DispatchRequest) error {
				calls++
				var saved volume.Backup
				require.NoError(t, db.Table("volume_backups").First(&saved, "id=?", "run").Error)
				require.Contains(t, []string{"failed", "partial"}, saved.Status)
				require.Equal(t, dto.DispatchKindBackupFailed, p.Kind)
				require.Equal(t, "backup", p.BackupFailed.Operation)
				return errors.New("controlled unreachable manager")
			}}
			f.service.deps.Store.Save = func(ctx context.Context, row *volume.Backup) error {
				return db.WithContext(ctx).Table("volume_backups").Save(row).Error
			}
			row := &volume.Backup{ID: "run", VolumeName: "data", Trigger: kind, Status: "running"}
			if kind == "partial" {
				row.LocalSnapshotID = "point"
			}
			if kind == "retention" {
				row.ErrorCode = "volume.backup.retention-failed"
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if kind == "cancelled" {
				cancel()
			}
			require.Error(t, f.service.completeBackupInternal(ctx, row, errors.New("controlled failure")))
			if kind == "safety" || kind == "cancelled" || kind == "retention" {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
			var n int64
			require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
			require.Zero(t, n)
		})
	}
}
