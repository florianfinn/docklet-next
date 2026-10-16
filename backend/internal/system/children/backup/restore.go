// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original transactional Hub restore and offline recovery for issue #32.

package backup

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	sharedbackup "github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
)

func (s *Service) SetExitUnready(cancel context.CancelFunc) { s.exitUnready = cancel }

func (s *Service) SetRestoreSchedulerRefresh(refresh func(context.Context) error) {
	s.refreshJobs = refresh
}

func (s *Service) sourceArchive(ctx context.Context, id string) (*backuptypes.SystemBackupRun, string, archiveInfo, error) {
	name, err := backuptypes.ArchiveNameForID(id)
	if err != nil {
		return nil, "", archiveInfo{}, ErrSystemBackupNotFound
	}
	run, err := s.store.Run(ctx, id)
	if err != nil || run == nil || run.Status != backuptypes.SystemBackupStatusSucceeded || run.ArchiveName != name {
		return nil, "", archiveInfo{}, ErrSystemBackupNotFound
	}
	path := filepath.Join(s.backupDir(), name)
	newest, err := newestMigration()
	if err != nil {
		return nil, "", archiveInfo{}, ErrSystemBackupUnavailable
	}
	info, err := validateArchive(ctx, path, newest)
	if err != nil {
		return nil, "", archiveInfo{}, err
	}
	if run.Size != info.size || run.SourceServerMajor != 18 || run.MigrationVersion != info.migration {
		return nil, "", archiveInfo{}, errInvalidArchive
	}
	if err := validateTargetServer(ctx, s.db); err != nil {
		return nil, "", archiveInfo{}, err
	}
	if err := validateTargetVersion(ctx, s.db, info.migration, newest, false); err != nil {
		return nil, "", archiveInfo{}, err
	}
	return run, path, info, nil
}

func (s *Service) StartRestore(ctx context.Context, actor usertypes.Actor, keyID, id, confirm string) (*backuptypes.SystemBackupOperation, error) {
	if confirm != id {
		return nil, errInvalidArchive
	}
	if s == nil || s.db == nil || s.activityService == nil || s.maintenance == nil || s.config == nil {
		return nil, ErrSystemBackupUnavailable
	}
	if err := s.databaseProviderError(); err != nil {
		return nil, err
	}
	if _, _, _, err := s.sourceArchive(ctx, id); err != nil {
		return nil, err
	}
	answer, err := s.activityService.StartGatedActivity(ctx, activity.GateRequest{
		EnvironmentID: "0", Operation: "restore:" + id, Scope: sharedbackup.SystemAdmissionScope,
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
	operation := &backuptypes.SystemBackupOperation{BackupID: id, ActivityID: answer.Run.ID, Status: string(answer.Run.Status)}
	if answer.WorkContext != nil {
		go s.executeRestore(answer.WorkContext, actor, keyID, id, answer.Run)
	}
	return operation, nil
}

func (s *Service) executeRestore(workCtx context.Context, actor usertypes.Actor, keyID, id string, started activitytypes.Activity) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(workCtx), archiveDeadline)
	defer cancel()
	fail := func(code string, status activitytypes.Status) {
		_, _ = s.activityService.CompleteActivity(workCtx, started.ID, status, code, &code)
	}
	if s.roles != nil {
		permissions, err := s.roles.ResolveExecutionPermissions(ctx, actor.ID, keyID)
		if err != nil || permissions == nil || !permissions.IsGlobalAdmin() || !permissions.Allows(authz.PermSystemBackupsRestore, "") {
			fail("backup.permission_denied", activitytypes.StatusFailed)
			return
		}
	}
	run, path, info, err := s.sourceArchive(ctx, id)
	if err != nil {
		fail("backup.invalid_archive", activitytypes.StatusFailed)
		return
	}
	if err := s.maintenance.Begin(started.ID); err != nil {
		fail("backup.maintenance", activitytypes.StatusFailed)
		return
	}
	markerWritten := false
	blocked := func() {
		s.maintenance.SetPhase("blocked")
		slog.Error("Hub restore requires offline recovery", "backupId", id, "code", "backup.recovery_required")
		if s.exitUnready != nil {
			s.exitUnready()
		}
	}
	resume := func() {
		if s.coordinator != nil {
			s.coordinator.Resume()
		}
		s.maintenance.End()
	}
	if err := s.maintenance.Wait(ctx); err != nil {
		resume()
		fail("backup.interrupted", activitytypes.StatusCancelled)
		return
	}
	if s.coordinator != nil {
		if err := s.coordinator.PauseAndDrain(ctx); err != nil {
			blocked()
			return
		}
	}
	if err := s.activityService.CancelAndDrain(ctx, started.ID); err != nil {
		blocked()
		return
	}
	if err := s.db.SuspendOrdinaryConnections(); err != nil {
		blocked()
		return
	}
	if workCtx.Err() != nil || s.activityService.SealCancellation(started.ID) != nil {
		_ = s.db.ResumeOrdinaryConnections()
		resume()
		fail("backup.interrupted", activitytypes.StatusCancelled)
		return
	}
	if err := CheckRestoreMarker(s.backupDir()); err != nil {
		blocked()
		return
	}
	if err := writeRestoreMarker(s.backupDir(), restoreMarker{BackupID: id, ActivityID: started.ID, Phase: "applying"}); err != nil {
		blocked()
		return
	}
	markerWritten = true
	s.maintenance.SetPhase("applying")
	commandErr := runPGRestore(ctx, s.config.DatabaseURL, path)
	if err := s.db.ResumeOrdinaryConnections(); err != nil {
		blocked()
		return
	}
	s.maintenance.SetPhase("verifying")
	if err := writeRestoreMarker(s.backupDir(), restoreMarker{BackupID: id, ActivityID: started.ID, Phase: "verifying"}); err != nil {
		blocked()
		return
	}
	if commandErr == nil {
		if err := verifyRestoredDatabase(ctx, s.db); err != nil {
			blocked()
			return
		}
		if err := s.store.SaveRun(ctx, restoreArchiveRecord(run, info, started.ID)); err != nil {
			blocked()
			return
		}
		if err := s.activityService.RestoreGatedActivity(ctx, started, id); err != nil {
			blocked()
			return
		}
	} else if err := verifyRollback(ctx, s.db, info.migration); err != nil {
		blocked()
		return
	}
	if err := revokeRestoredSessions(ctx, s.db); err != nil {
		blocked()
		return
	}
	if s.auth != nil {
		s.auth.InvalidateAllTokenCache()
	}
	if s.roles != nil {
		s.roles.InvalidateAllPermissions()
	}
	if s.settings != nil {
		if err := s.settings.RefreshAfterRestore(ctx); err != nil {
			blocked()
			return
		}
	}
	if commandErr == nil && s.refreshJobs != nil {
		if err := s.refreshJobs(ctx); err != nil {
			blocked()
			return
		}
	}
	status, code := activitytypes.StatusSuccess, "backup.restored"
	if commandErr != nil {
		status, code = activitytypes.StatusFailed, "backup.restore_failed"
	}
	if _, err := s.activityService.CompleteActivity(workCtx, started.ID, status, code, nil); err != nil {
		blocked()
		return
	}
	if markerWritten {
		if err := clearRestoreMarker(s.backupDir()); err != nil {
			blocked()
			return
		}
	}
	resume()
}

func restoreArchiveRecord(run *backuptypes.SystemBackupRun, info archiveInfo, activityID string) *backuptypes.SystemBackupRun {
	copy := *run
	copy.ActivityID = activityID
	copy.Status = backuptypes.SystemBackupStatusSucceeded
	copy.Size, copy.MigrationVersion, copy.SourceServerMajor = info.size, info.migration, 18
	return &copy
}

func runPGRestore(ctx context.Context, dsn, path string) error {
	env, err := postgresEnv(dsn)
	if err != nil {
		return errors.New("backup.restore_failed")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Path == "" {
		return errors.New("backup.restore_failed")
	}
	command := exec.CommandContext(ctx, "pg_restore", "--clean", "--if-exists", "--no-owner", "--no-acl", "--single-transaction", "--exit-on-error", "--dbname", parsed.Path[1:], path)
	command.Env, command.Stdout, command.Stderr = env, io.Discard, io.Discard
	if err := command.Run(); err != nil {
		return errors.New("backup.restore_failed")
	}
	return nil
}

func validateTargetServer(ctx context.Context, db *database.DB) error {
	var version int
	if db == nil || db.WithContext(ctx).Raw("SELECT current_setting('server_version_num')::int").Scan(&version).Error != nil || version < 180000 || version >= 190000 {
		return errArchiveMajor
	}
	return nil
}

func verifyRestoredDatabase(ctx context.Context, db *database.DB) error {
	if err := db.Migrate(ctx); err != nil {
		return ErrRestoreRecoveryRequired
	}
	newest, err := newestMigration()
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	return verifyRollback(ctx, db, newest)
}

func verifyRollback(ctx context.Context, db *database.DB, expectedVersion int64) error {
	if err := validateTargetServer(ctx, db); err != nil {
		return ErrRestoreRecoveryRequired
	}
	var version int64
	if err := db.WithContext(ctx).Raw("SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied = true").Scan(&version).Error; err != nil || version != expectedVersion {
		return ErrRestoreRecoveryRequired
	}
	var healthy int
	if err := db.WithContext(ctx).Raw("SELECT 1").Scan(&healthy).Error; err != nil || healthy != 1 {
		return ErrRestoreRecoveryRequired
	}
	return nil
}

func revokeRestoredSessions(ctx context.Context, db *database.DB) error {
	return session.NewSessionService(db).RevokeAllSessions(ctx)
}

// OfflineRestore runs before HTTP and scheduler startup and needs no source
// metadata or admin session in the replacement database.
func OfflineRestore(ctx context.Context, cfg *config.Config, id string) error {
	if cfg == nil || cfg.DatabaseURL == "" || cfg.BackupDir == "" {
		return ErrSystemBackupUnavailable
	}
	if !archiveToolsAvailable() {
		return ErrSystemBackupToolsUnavailable
	}
	name, err := backuptypes.ArchiveNameForID(id)
	if err != nil {
		return errInvalidArchive
	}
	db, err := database.OpenHubForRestore(ctx, cfg.DatabaseURL)
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	defer db.Close()
	newest, err := newestMigration()
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	path := filepath.Join(cfg.BackupDir, name)
	recovery := false
	if _, statErr := os.Lstat(markerPath(cfg.BackupDir)); statErr == nil {
		if _, markerErr := readRestoreMarker(cfg.BackupDir); markerErr != nil {
			return markerErr
		}
		recovery = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return ErrRestoreRecoveryRequired
	}
	if err := validateTargetServer(ctx, db); err != nil {
		return err
	}
	info, err := validateArchive(ctx, path, newest)
	if err != nil {
		return err
	}
	if err := validateTargetVersion(ctx, db, info.migration, newest, recovery); err != nil {
		return err
	}
	if err := writeRestoreMarker(cfg.BackupDir, restoreMarker{BackupID: id, Phase: "applying"}); err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, archiveDeadline)
	defer cancel()
	if err := runPGRestore(deadline, cfg.DatabaseURL, path); err != nil {
		return err
	}
	if err := writeRestoreMarker(cfg.BackupDir, restoreMarker{BackupID: id, Phase: "verifying"}); err != nil {
		return err
	}
	if err := verifyRestoredDatabase(deadline, db); err != nil {
		return err
	}
	if err := activity.NewActivityService(db, nil).FailInterruptedAfterRestore(deadline); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err := revokeRestoredSessions(deadline, db); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err := db.WithContext(deadline).Exec(`INSERT INTO system_backup_runs
        (id, created_at, updated_at, status, trigger, size, archive_name, completed_at,
         source_server_major, migration_version, copy_state, activity_id)
        VALUES (?, NOW(), NOW(), 'succeeded', 'uploaded', ?, ?, NOW(), 18, ?, 'none', '')
        ON CONFLICT (id) DO UPDATE SET status='succeeded', size=EXCLUDED.size,
        archive_name=EXCLUDED.archive_name, completed_at=NOW(), source_server_major=18,
        migration_version=EXCLUDED.migration_version, copy_state='none'`, id, info.size, name, info.migration).Error; err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err := clearRestoreMarker(cfg.BackupDir); err != nil {
		return err
	}
	return nil
}
