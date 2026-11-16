// Source: getarcaneapp/arcane/backend/internal/system/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Replaced Rustic system recovery with coordinated SQLite archives; retained the separate volume-backup coordinator; notification and Auto Heal contracts and delivery for #296; M16 bind mounts and backup completion; success, retention rows and gate release commit together.

package backup

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"gorm.io/gorm"
)

const systemAdmissionID = "system"

// RunCommitTx saves the run and deletes the expired run rows inside an activity completion transaction.
type RunCommitTx func(*gorm.DB, *backuptypes.SystemBackupRun, []string) error

// expiredRuns selects the scheduled archives that this run's success removes. The running current
// run is not counted by ExpiredRunIDs but takes one of the keep slots.
func (s *Service) expiredRuns(ctx context.Context, policyID string, keep int) ([]*backuptypes.SystemBackupRun, error) {
	if keep < 1 {
		return nil, errors.New("backup.invalid_retention")
	}
	ids, err := s.store.ExpiredRunIDs(ctx, policyID, keep-1)
	if err != nil {
		return nil, err
	}
	var expired []*backuptypes.SystemBackupRun
	for _, id := range ids {
		name, err := backuptypes.ArchiveNameForID(id)
		if err != nil {
			return nil, err
		}
		run, err := s.store.Run(ctx, id)
		if err != nil {
			return nil, err
		}
		if run.Trigger != backuptypes.SystemBackupTriggerScheduled || run.PolicyID != policyID || run.ArchiveName != name {
			continue
		}
		expired = append(expired, run)
	}
	return expired, nil
}

// commitRun completes the run's activity and, in the same transaction, saves the validated archive as
// succeeded and deletes the expired rows; any other effective status saves the run as interrupted.
// It reports whether the success committed. On a failed hook the activity completes as failed instead.
func (s *Service) commitRun(ctx context.Context, run *backuptypes.SystemBackupRun, status activitytypes.Status, code string, detail *string, expired []*backuptypes.SystemBackupRun) (bool, error) {
	ids := make([]string, 0, len(expired))
	for _, row := range expired {
		ids = append(ids, row.ID)
	}
	succeeded := false
	hook := func(tx *gorm.DB, effective activitytypes.Status) error {
		final := *run
		succeeded = effective == activitytypes.StatusSuccess
		if !succeeded {
			final.Status, final.ErrorCode = backuptypes.SystemBackupStatusFailed, backuptypes.SystemBackupErrorInterrupted
			return s.store.CommitRunTx(tx, &final, nil)
		}
		final.Status = backuptypes.SystemBackupStatusSucceeded
		return s.store.CommitRunTx(tx, &final, ids)
	}
	_, err := s.activityService.CompleteActivityWithHook(ctx, run.ActivityID, status, code, detail, hook)
	if errors.Is(err, activity.ErrCompletionHook) {
		failed := "backup.failed"
		_, err = s.activityService.CompleteActivity(ctx, run.ActivityID, activitytypes.StatusFailed, failed, &failed)
		succeeded = false
	}
	if err != nil {
		return false, err
	}
	return succeeded, nil
}

// discardRun records a run whose success did not commit as interrupted and removes its archive and copy,
// so it keeps no retention slot.
func (s *Service) discardRun(ctx context.Context, run *backuptypes.SystemBackupRun) {
	run.Status, run.ErrorCode = backuptypes.SystemBackupStatusFailed, backuptypes.SystemBackupErrorInterrupted
	_ = s.store.SaveRun(ctx, run)
	s.removeArchive(ctx, run)
}

// removeExpiredArchives deletes files and copies of rows deleted with the successful commit. A file left by
// a failed removal has no row and is removed by startup storage reconciliation.
func (s *Service) removeExpiredArchives(ctx context.Context, expired []*backuptypes.SystemBackupRun) error {
	var failed bool
	for _, run := range expired {
		if !s.removeArchive(ctx, run) {
			failed = true
		}
	}
	if failed {
		return ErrSystemBackupRetention
	}
	return nil
}

func (s *Service) removeArchive(ctx context.Context, run *backuptypes.SystemBackupRun) bool {
	if run.CopyState == backuptypes.SystemBackupCopySucceeded && run.CopyTargetID != "" {
		if copyErr := s.removeArchiveCopy(ctx, run); copyErr != nil {
			slog.WarnContext(ctx, "Hub archive copy retention failed", "backupId", run.ID, "code", "backup.copy.write-failed")
		}
	}
	if run.ArchiveName == "" {
		return true
	}
	if err := os.Remove(filepath.Join(s.backupDir(), run.ArchiveName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.WarnContext(ctx, "Hub archive retention failed", "backupId", run.ID, "code", "backup.retention_failed")
		return false
	}
	return true
}
