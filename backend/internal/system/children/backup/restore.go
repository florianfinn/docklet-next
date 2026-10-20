// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original paired SQLite Hub restore and offline recovery for issue #32.

package backup

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"log/slog"
	"os"
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
	"github.com/florianfinn/docklet-next/backend/pkg/utils/francis"
	"time"
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
	if run.Size != info.size || run.ArchiveVersion != archiveVersion || run.MigrationVersion != info.migration {
		return nil, "", archiveInfo{}, errInvalidArchive
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
	run, _, info, err := s.sourceArchive(ctx, id)
	if err != nil {
		fail("backup.invalid_archive", activitytypes.StatusFailed)
		return
	}
	if err := s.maintenance.Begin(started.ID); err != nil {
		fail("backup.maintenance", activitytypes.StatusFailed)
		return
	}
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
	if err := s.activityService.PrepareRuntimeRestore(ctx); err != nil {
		blocked()
		return
	}
	if err := s.activityService.CancelAndDrain(ctx, started.ID); err != nil {
		blocked()
		return
	}
	if workCtx.Err() != nil || s.activityService.SealCancellation(started.ID) != nil {
		resume()
		fail("backup.interrupted", activitytypes.StatusCancelled)
		return
	}
	if err := CheckRestoreMarker(restoreStateDir(s.db.Path())); err != nil {
		blocked()
		return
	}
	if s.runtime != nil {
		if err := s.runtime.Stop(ctx); err != nil {
			blocked()
			return
		}
	}
	if err := s.db.SuspendOrdinaryConnections(); err != nil {
		blocked()
		return
	}
	if err := checkpointStoppedPair(ctx, s.db.Path()); err != nil {
		blocked()
		return
	}
	marker, err := preparePair(ctx, s.config, s.db.Path(), id, started.ID)
	if err != nil {
		blocked()
		return
	}
	s.maintenance.SetPhase("applying")
	if err = installPair(ctx, restoreStateDir(s.db.Path()), marker); err != nil {
		blocked()
		return
	}
	if err = s.db.ResumeOrdinaryConnections(); err != nil {
		blocked()
		return
	}
	s.maintenance.SetPhase("verifying")
	if err = finalizeRestoredDatabase(ctx, s.db, s.config); err != nil {
		blocked()
		return
	}
	if err = s.store.SaveRun(ctx, restoreArchiveRecord(run, info, started.ID)); err != nil {
		blocked()
		return
	}
	if err = s.activityService.RestoreGatedActivity(ctx, started, id); err != nil {
		blocked()
		return
	}
	if err := s.activityService.FinishRuntimeRestore(ctx); err != nil {
		blocked()
		return
	}
	if s.coordinator != nil {
		s.coordinator.ResetAfterRestore()
	}
	if s.runtime != nil {
		if err = s.runtime.Restart(ctx); err != nil {
			blocked()
			return
		}
	}
	if s.apiKeys != nil {
		s.apiKeys.InvalidateAfterRestore()
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
	if s.refreshJobs != nil {
		if err := s.refreshJobs(ctx); err != nil {
			blocked()
			return
		}
	}
	if _, err = s.activityService.CompleteActivity(workCtx, started.ID, activitytypes.StatusSuccess, "backup.restored", nil); err != nil {
		blocked()
		return
	}
	if err = clearRestoreMarker(restoreStateDir(s.db.Path())); err != nil {
		blocked()
		return
	}
	s.activityService.ResumeRuntimeIntake()
	resume()
}

func restoreArchiveRecord(run *backuptypes.SystemBackupRun, info archiveInfo, activityID string) *backuptypes.SystemBackupRun {
	copy := *run
	copy.ActivityID = activityID
	copy.Status = backuptypes.SystemBackupStatusSucceeded
	copy.Size, copy.MigrationVersion, copy.ArchiveVersion = info.size, info.migration, archiveVersion
	return &copy
}

func finalizeRestoredDatabase(ctx context.Context, db *database.DB, cfg *config.Config) error {
	if err := db.WithContext(ctx).Exec("DROP TABLE IF EXISTS docklet_backup_image").Error; err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err := db.Migrate(ctx); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err := francis.ClearRestoredOwnership(ctx, cfg.DatabaseURL); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err := activity.NewActivityService(db, nil).FailInterruptedAfterRestore(ctx); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err := revokeRestoredSessions(ctx, db); err != nil {
		return ErrRestoreRecoveryRequired
	}
	pool, err := db.SQLDB()
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err = checkImage(ctx, pool); err != nil {
		return ErrRestoreRecoveryRequired
	}
	return nil
}
func revokeRestoredSessions(ctx context.Context, db *database.DB) error {
	return session.NewSessionService(db).RevokeAllSessions(ctx)
}

// OfflineRestore needs no source metadata or session in an empty replacement hub.
func OfflineRestore(ctx context.Context, cfg *config.Config, id string) error {
	if cfg == nil || cfg.DatabaseURL == "" || cfg.BackupDir == "" {
		return ErrSystemBackupUnavailable
	}
	if _, err := backuptypes.ArchiveNameForID(id); err != nil {
		return errInvalidArchive
	}
	if CheckPendingRestore(cfg) != nil {
		return RecoverPendingRestore(ctx, cfg)
	}
	ctx, cancel := context.WithTimeout(ctx, archiveDeadline)
	defer cancel()
	lock, path, err := database.AcquireRestoreLock(ctx, cfg.DatabaseURL)
	if err != nil {
		return restoreLockError(err)
	}
	defer lock.Stop()
	if err = checkpointStoppedPair(ctx, path); err != nil {
		return err
	}
	// The target is stopped. Validate any existing Hub identity/version before replacing it.
	if _, err := os.Stat(path); err == nil {
		pool, err := openImage(path)
		if err != nil {
			return errMigration
		}
		var count int
		err = pool.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&count)
		if err == nil && count > 0 {
			err = database.ValidateSnapshot(ctx, pool, mustNewestMigration())
		}
		_ = pool.Close()
		if err != nil {
			return errMigration
		}
	}
	marker, err := preparePair(ctx, cfg, path, id, "")
	if err != nil {
		return err
	}
	if err = installPair(ctx, restoreStateDir(path), marker); err != nil {
		return err
	}
	db, err := database.OpenLockedForRestore(ctx, path, lock)
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	defer db.SuspendOrdinaryConnections()
	if err = finalizeRestoredDatabase(ctx, db, cfg); err != nil {
		return err
	}
	if err = recordRecoveredArchive(ctx, db, marker); err != nil {
		return err
	}
	return clearRestoreMarker(restoreStateDir(path))
}
func recordRecoveredArchive(ctx context.Context, db *database.DB, marker restoreMarker) error {
	name, _ := backuptypes.ArchiveNameForID(marker.BackupID)
	now := time.Now().UTC()
	if err := db.WithContext(ctx).Exec(`INSERT INTO system_backup_runs
  (id,created_at,updated_at,status,trigger,size,archive_name,completed_at,archive_version,migration_version,copy_state,activity_id)
  VALUES (?,?,?,'succeeded','uploaded',?,?,?,?,?,'none','')
  ON CONFLICT(id) DO UPDATE SET status='succeeded',size=excluded.size,archive_name=excluded.archive_name,
  completed_at=excluded.completed_at,archive_version=excluded.archive_version,migration_version=excluded.migration_version,copy_state='none'`,
		marker.BackupID, now, now, marker.ArchiveSize, name, now, archiveVersion, marker.MigrationVersion).Error; err != nil {
		return ErrRestoreRecoveryRequired
	}
	return nil
}

// Recover committed WAL before inspecting or copying a stopped target. A marker
// uses immutable staged images instead and must not open an interrupted pair.
func checkpointStoppedPair(ctx context.Context, path string) error {
	for i, file := range []string{path, sqlitestore.FrancisPath(path)} {
		if _, err := os.Lstat(file); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return ErrRestoreRecoveryRequired
		}
		pool, err := sqlitestore.Open(ctx, file, i == 0, false)
		if err != nil {
			return ErrRestoreRecoveryRequired
		}
		var busy, log, done int
		err = pool.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &log, &done)
		closeErr := pool.Close()
		if err != nil || closeErr != nil || busy != 0 {
			return ErrRestoreRecoveryRequired
		}
	}
	return nil
}
