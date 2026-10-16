// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original bounded Hub archive upload for issue #32.

package backup

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
	"uuid"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	sharedbackup "github.com/florianfinn/docklet-next/backend/internal/backup"
)

func (s *Service) UploadArchive(ctx context.Context, actor usertypes.Actor, reader io.Reader) (*backuptypes.SystemBackupOperation, error) {
	if s == nil || s.store.CreateRun == nil || s.activityService == nil {
		return nil, ErrSystemBackupUnavailable
	}
	if err := s.databaseProviderError(); err != nil {
		return nil, err
	}
	id := "system-" + uuid.New().String()
	answer, err := s.activityService.StartGatedActivity(ctx, activity.GateRequest{
		EnvironmentID: "0", Operation: "upload:" + id, Scope: sharedbackup.SystemAdmissionScope,
		Type: activitytypes.TypeResourceAction, ResourceType: "system_backup", ResourceID: id,
		StartedBy: &actor,
	})
	if err != nil {
		var conflict *activity.GateConflict
		if errors.As(err, &conflict) {
			return nil, ErrSystemBackupAlreadyRunning
		}
		return nil, errors.New("backup.failed")
	}
	workCtx := answer.WorkContext
	run := &backuptypes.SystemBackupRun{ID: id, ActivityID: answer.Run.ID,
		CreatedAt: time.Now().UTC(), Status: backuptypes.SystemBackupStatusRunning,
		Trigger: backuptypes.SystemBackupTriggerUploaded, CopyState: backuptypes.SystemBackupCopyNone}
	if err := s.store.CreateRun(ctx, run); err != nil {
		code := "backup.failed"
		_, _ = s.activityService.CompleteActivity(workCtx, answer.Run.ID, activitytypes.StatusFailed, code, &code)
		return nil, errors.New(code)
	}
	deadline, cancel := context.WithTimeout(workCtx, archiveDeadline)
	defer cancel()
	name, info, err := s.receiveArchive(deadline, id, reader)
	now := time.Now().UTC()
	run.CompletedAt = &now
	status, code := activitytypes.StatusSuccess, "backup.uploaded"
	if err != nil {
		run.Status, run.ErrorCode = backuptypes.SystemBackupStatusFailed, uploadErrorCode(err)
		status, code = activitytypes.StatusFailed, run.ErrorCode
	} else {
		run.Status = backuptypes.SystemBackupStatusSucceeded
		run.ArchiveName, run.Size, run.ArchiveVersion, run.MigrationVersion = name, info.size, archiveVersion, info.migration
	}
	if saveErr := s.store.SaveRun(context.WithoutCancel(workCtx), run); saveErr != nil {
		if name != "" {
			_ = os.Remove(filepath.Join(s.backupDir(), name))
		}
		status, code, err = activitytypes.StatusFailed, "backup.failed", errors.New("backup.failed")
	}
	_, completeErr := s.activityService.CompleteActivity(workCtx, answer.Run.ID, status, code, nil)
	if completeErr != nil {
		return nil, errors.New("backup.failed")
	}
	if err != nil {
		return nil, err
	}
	return &backuptypes.SystemBackupOperation{BackupID: id, ActivityID: answer.Run.ID, Status: string(status)}, nil
}

func uploadErrorCode(err error) string {
	switch {
	case errors.Is(err, errArchiveLimit):
		return "backup.size_limit"
	case errors.Is(err, errArchiveMajor):
		return "backup.incompatible_server"
	case errors.Is(err, errMigration):
		return "backup.incompatible_migration"
	case errors.Is(err, errArchiveTimeout):
		return "backup.timeout"
	case errors.Is(err, ErrSystemBackupToolsUnavailable):
		return "backup.tools_unavailable"
	case errors.Is(err, ErrSystemBackupUnavailable):
		return "backup.storage_unavailable"
	case errors.Is(err, errInvalidArchive):
		return "backup.invalid_archive"
	default:
		return "backup.failed"
	}
}

func (s *Service) receiveArchive(ctx context.Context, id string, reader io.Reader) (string, archiveInfo, error) {
	return s.receiveArchiveWithLimit(ctx, id, reader, backuptypes.MaxSystemArchiveBytes)
}

func (s *Service) receiveArchiveWithLimit(ctx context.Context, id string, reader io.Reader, limit int64) (string, archiveInfo, error) {
	var info archiveInfo
	name, err := backuptypes.ArchiveNameForID(id)
	if err != nil {
		return "", info, errInvalidArchive
	}
	file, err := os.CreateTemp(s.backupDir(), ".partial-")
	if err != nil {
		return "", info, ErrSystemBackupUnavailable
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return "", info, ErrSystemBackupUnavailable
	}
	writer := &limitedArchiveWriter{file: file, limit: limit}
	_, err = io.Copy(writer, &contextReader{ctx: ctx, reader: reader})
	if err != nil {
		if ctx.Err() != nil {
			return "", info, archiveContextError(ctx)
		}
		if errors.Is(err, errArchiveLimit) {
			return "", info, errArchiveLimit
		}
		return "", info, errInvalidArchive
	}
	if err := archiveContextError(ctx); err != nil {
		return "", info, err
	}
	if err := file.Sync(); err != nil {
		return "", info, ErrSystemBackupUnavailable
	}
	newest, err := newestMigration()
	if err != nil {
		return "", info, ErrSystemBackupUnavailable
	}
	info, err = validateArchive(ctx, file.Name(), newest)
	if err != nil {
		return "", info, err
	}
	if err := file.Close(); err != nil {
		return "", info, ErrSystemBackupUnavailable
	}
	if err := os.Rename(file.Name(), filepath.Join(s.backupDir(), name)); err != nil {
		return "", info, ErrSystemBackupUnavailable
	}
	if err := syncArchiveDir(s.backupDir()); err != nil {
		_ = os.Remove(filepath.Join(s.backupDir(), name))
		return "", info, err
	}
	return name, info, nil
}
