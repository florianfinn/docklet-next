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
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	nt "github.com/florianfinn/docklet-next/types/notification"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVolumeBackupDeterminedFailureNotifiesWhenCancelledWhileSaved(t *testing.T) {
	for _, agent := range []bool{false, true} {
		for _, cancelled := range []bool{false, true} {
			t.Run(fmt.Sprintf("agent=%t/cancel=%t", agent, cancelled), func(t *testing.T) {
				f := newRuntimePlan(t)
				db := f.service.deps.DB
				require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				calls := 0
				f.service.notifier = backupnotify.Notifier{DB: db, Agent: agent, Dispatch: func(context.Context, nt.DispatchRequest) error { calls++; return nil }}
				saved := false
				f.service.deps.Store.Save = func(c context.Context, row *volume.Backup) error {
					saved = true
					if cancelled {
						cancel()
					}
					return db.WithContext(c).Table("volume_backups").Save(row).Error
				}
				row := &volume.Backup{ID: "run", VolumeName: "data", Trigger: "manual", Status: "running"}
				require.Error(t, f.service.completeBackupInternal(ctx, row, errors.New("controlled failure")))
				require.True(t, saved)
				var n int64
				require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
				// The failure was determined before the save; cancelling the save does not interrupt it.
				if agent {
					require.Equal(t, 1, calls)
				} else {
					require.EqualValues(t, 1, n)
				}
			})
		}
	}
}
