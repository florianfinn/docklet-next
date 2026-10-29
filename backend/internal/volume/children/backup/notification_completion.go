// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; M12 backup foundation; M12 persisted volume lifecycle and legacy restore cut.; Remove legacy archive and centralized volume execution; preserve historical metadata deletion and safety pins.

package backup

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/volume"
	"time"
)

func (s *Service) completeBackupInternal(ctx context.Context, entry *volume.Backup, err error) error {
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		if entry.ErrorCode == "" {
			entry.ErrorCode = "volume.backup.interrupted"
			var problem *backuptypes.Problem
			if errors.As(err, &problem) {
				entry.ErrorCode = problem.Code
			}
		}
		entry.Status = backuptypes.VolumeBackupStatusFailed
		if entry.LocalSnapshotID != "" {
			entry.Status = backuptypes.VolumeBackupStatusPartial
		}
		entry.Error = entry.ErrorCode
	} else {
		entry.Status = backuptypes.VolumeBackupStatusSucceeded
		entry.Error = ""
		entry.ErrorCode = ""
	}
	// The owning worker remains admitted while terminal persistence is unavailable.
	for saveErr := s.deps.Store.Save(context.WithoutCancel(ctx), entry); saveErr != nil; saveErr = s.deps.Store.Save(context.WithoutCancel(ctx), entry) {
		time.Sleep(100 * time.Millisecond)
	}
	s.notifier.Terminal(ctx, backupnotify.Run{ID: entry.ID, Operation: "backup", Volume: entry.VolumeName, Status: entry.Status, Reason: entry.ErrorCode, ActivityID: backupnotify.ActivityID(entry.ActivityID), Suppress: entry.Trigger == backuptypes.VolumeBackupTriggerSafety})
	if err == nil {
		err = jobcontext.Progress(ctx, scheduler.TargetOutcome{ResourceType: "volume_backup", ID: entry.VolumeName, Status: scheduler.Succeeded})
	}
	return err
}
