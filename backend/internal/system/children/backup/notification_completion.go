// Source: getarcaneapp/arcane/backend/internal/system/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Replaced Rustic system recovery with coordinated SQLite archives; retained the separate volume-backup coordinator; notification and Auto Heal contracts and delivery for #296.

package backup

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"os"
	"path/filepath"
	"time"
)

func (s *Service) finishRun(workCtx context.Context, run *backuptypes.SystemBackupRun) error {
	archive, size, err := s.snapshotArchive(workCtx, run.ActivityID, run.ID)
	var migration int64
	if err == nil {
		info, validateErr := validateArchive(workCtx, filepath.Join(s.backupDir(), archive), mustNewestMigration())
		err, migration = validateErr, info.migration
	}
	now := time.Now().UTC()
	run.CompletedAt = &now
	status := activitytypes.StatusSuccess
	code := "backup.completed"
	var detail *string
	if err != nil {
		run.Status = backuptypes.SystemBackupStatusFailed
		run.ErrorCode = backuptypes.SystemBackupErrorFailed
		status, code = activitytypes.StatusFailed, "backup.failed"
		if workCtx.Err() != nil {
			status, code = activitytypes.StatusCancelled, "backup.interrupted"
			run.ErrorCode = backuptypes.SystemBackupErrorInterrupted
		}
		detail = &code
	} else {
		run.Status = backuptypes.SystemBackupStatusSucceeded
		run.ArchiveName, run.Size, run.MigrationVersion, run.ArchiveVersion = archive, size, migration, archiveVersion
	}
	saveErr := s.store.SaveRun(context.WithoutCancel(workCtx), run)
	if saveErr != nil {
		if archive != "" {
			_ = os.Remove(filepath.Join(s.backupDir(), archive))
		}
		status, code = activitytypes.StatusFailed, "backup.failed"
		detail = &code
	}
	if saveErr == nil && status != activitytypes.StatusCancelled {
		s.notifier.Terminal(workCtx, backupnotify.Run{ID: run.ID, Operation: "backup", Volume: "hub-database", Status: string(run.Status), Reason: code, ActivityID: run.ActivityID})
	}
	var retentionErr error
	if err == nil && saveErr == nil && run.Trigger == backuptypes.SystemBackupTriggerScheduled {
		policy, loadErr := s.store.Policy(workCtx, run.PolicyID)
		if loadErr != nil || policy == nil {
			retentionErr = ErrSystemBackupRetention
		} else {
			retentionErr = s.applyRetention(workCtx, run.PolicyID, policy.RetentionCount)
		}
		if retentionErr != nil {
			code = "backup.retention_failed"
		}
	}
	_, activityErr := s.activityService.CompleteActivity(workCtx, run.ActivityID, status, code, detail)
	if err != nil {
		return errors.New(code)
	}
	if saveErr != nil {
		return errors.New("backup.failed")
	}
	if activityErr != nil {
		return errors.New("backup.failed")
	}
	if retentionErr != nil {
		return ErrSystemBackupRetention
	}
	return activityErr
}
