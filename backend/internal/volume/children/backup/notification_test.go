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
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLiveBackupFailureNoticeAndSafetySuppression(t *testing.T) {
	for _, kind := range []string{"manual", "scheduled", "partial", "safety", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			f := newRuntimePlan(t)
			db := f.service.deps.DB
			require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
			f.service.notifier = backupnotify.Notifier{DB: db}
			saved := false
			f.service.deps.Store.Save = func(context.Context, *volume.Backup) error { saved = true; return nil }
			entry := &volume.Backup{ID: "run", VolumeName: "data", Trigger: kind, Status: "running"}
			if kind == "partial" {
				entry.LocalSnapshotID = "point"
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if kind == "cancelled" {
				cancel()
			}
			require.Error(t, f.service.completeBackupInternal(ctx, entry, errors.New("controlled failure")))
			require.True(t, saved)
			var rows []notificationoutbox.Row
			require.NoError(t, db.Find(&rows).Error)
			if kind == "safety" || kind == "cancelled" {
				require.Empty(t, rows)
			} else {
				require.Len(t, rows, 1)
			}
		})
	}
}
