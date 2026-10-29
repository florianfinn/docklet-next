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
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
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

func TestSafetyFailureEmitsOnlyTheEnclosingRestoreNotice(t *testing.T) {
	f := newRuntimePlan(t)
	s := f.service
	db := s.deps.DB
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
	s.notifier = backupnotify.Notifier{DB: db}
	// A capture error occurs after the safety row is created. Use the actual safety function.
	s.deps.Store.Create = func(ctx context.Context, r *volume.Backup) error {
		return db.WithContext(ctx).Table("volume_backups").Create(r).Error
	}
	s.deps.HelperImage = func(context.Context, *client.Client) (string, error) {
		return "", errors.New("controlled safety capture failure")
	}
	run := &restore.Run{ID: "restore", VolumeName: "data", Kind: "full", Mode: "replace", Status: "running", Phase: "preparing", StartedAt: time.Now().UTC()}
	require.NoError(t, db.Create(run).Error)
	s.lifecycle.Effects.Prepare = func(context.Context, *restore.Run, restore.StartRequest) (func() error, error) {
		return func() error { return nil }, nil
	}
	s.lifecycle.Effects.Consumers = func(context.Context, string) ([]volume.StoppedContainer, error) { return nil, nil }
	s.lifecycle.Effects.Restart = func(context.Context, []volume.StoppedContainer, user.Actor) error { return nil }
	require.Error(t, s.lifecycle.Execute(t.Context(), run, restore.StartRequest{}))
	var rows []notificationoutbox.Row
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "restore", rows[0].Payload["operation"])
}
