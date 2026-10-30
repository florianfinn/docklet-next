// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M13 closing-verification regressions.

package backup

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	nt "github.com/florianfinn/docklet-next/types/notification"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVolumeBackupLinkFailureFollowsTheTrackedWork(t *testing.T) {
	for _, agent := range []bool{false, true} {
		for _, cancelled := range []bool{false, true} {
			t.Run(fmt.Sprintf("agent=%t/cancelled=%t", agent, cancelled), func(t *testing.T) {
				f := newRuntimePlan(t)
				s := f.service
				db := s.deps.DB
				require.NoError(t, db.AutoMigrate(activity.Models()...))
				require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
				s.deps.Activity = activity.NewActivityService(db, nil)
				calls := 0
				s.notifier = backupnotify.Notifier{DB: db, Agent: agent, Dispatch: func(context.Context, nt.DispatchRequest) error { calls++; return nil }}
				require.NoError(t, db.AutoMigrate(&policies.VolumeBackupPolicy{}))
				s.deps.Store.Create = func(ctx context.Context, row *volume.Backup) error {
					return db.WithContext(ctx).Table("volume_backups").Create(row).Error
				}
				saved := false
				s.deps.Store.Save = func(ctx context.Context, row *volume.Backup) error {
					if row.ActivityID != nil && row.Status == "running" {
						saved = true
						if cancelled {
							require.True(t, s.deps.Activity.RequestCancel(*row.ActivityID))
						}
						return errors.New("controlled link failure")
					}
					return db.WithContext(ctx).Table("volume_backups").Save(row).Error
				}
				_, err := s.StartBackup(t.Context(), "0", "data", user.SystemUser, volume.CreateBackupRequest{})
				require.Error(t, err)
				require.True(t, saved)
				var rows []notificationoutbox.Row
				require.NoError(t, db.Find(&rows).Error)
				if cancelled {
					require.Empty(t, rows, "tracked work cancelled before its outcome determination")
					require.Zero(t, calls)
				} else if agent {
					require.Equal(t, 1, calls)
				} else {
					require.Len(t, rows, 1)
				}
			})
		}
	}
}
