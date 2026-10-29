// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package restore

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRestoreTerminalNoticeAndSilentHistory(t *testing.T) {
	l, _ := lifecycleFixture(t) // existing fake effects and migrated restore store
	require.NoError(t, l.DB.AutoMigrate(&notificationoutbox.Row{}))
	n := backupnotify.Notifier{DB: l.DB}
	l.Notify = n.Terminal
	for _, kind := range []string{"full", "files", "upload"} {
		row := &Run{ID: kind, VolumeName: "data", Kind: kind, Phase: "done", Status: "failed", ErrorCode: "volume.restore.failed", StartedAt: time.Now().UTC()}
		l.persistTerminal(t.Context(), row)
	}
	var rows []notificationoutbox.Row
	require.NoError(t, l.DB.Find(&rows).Error)
	require.Len(t, rows, 3)
	require.NoError(t, l.Reconcile(t.Context(), false, nil))
	require.NoError(t, l.DB.Find(&rows).Error)
	require.Len(t, rows, 3, "startup repair is silent")
}

func TestRestoreRetentionAndCancellationStaySilent(t *testing.T) {
	for _, mode := range []string{"retention", "cancelled", "new-volume"} {
		t.Run(mode, func(t *testing.T) {
			l, r := lifecycleFixture(t)
			require.NoError(t, l.DB.AutoMigrate(&notificationoutbox.Row{}))
			l.Notify = backupnotify.Notifier{DB: l.DB}.Terminal
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			r.Status = "partial"
			r.ErrorCode = "volume.backup.retention-failed"
			if mode == "cancelled" {
				r.Status = "failed"
				r.ErrorCode = "volume.restore.failed"
				cancel()
			}
			if mode == "new-volume" {
				r.Status = "failed"
				r.ErrorCode = "volume.restore.failed"
				r.Mode = "new"
				r.NewVolumeName = "restored"
			}
			l.persistTerminal(ctx, r)
			var n int64
			require.NoError(t, l.DB.Model(&notificationoutbox.Row{}).Count(&n).Error)
			if mode == "new-volume" {
				require.EqualValues(t, 1, n)
			} else {
				require.Zero(t, n)
			}
		})
	}
}
