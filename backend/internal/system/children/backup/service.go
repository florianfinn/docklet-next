// Source: getarcaneapp/arcane/backend/internal/system/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Replaced Rustic system recovery with coordinated SQLite archives; retained the separate volume-backup coordinator.

package backup

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
	"uuid"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	sharedbackup "github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/system/children/backup/children/volumes"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/entityjobs"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/francis"
)

const systemAdmissionID = "system"

var (
	ErrSystemBackupAlreadyRunning    = errors.New("run.busy")
	ErrSystemBackupUnavailable       = errors.New("backup.storage_unavailable")
	ErrSystemBackupToolsUnavailable  = errors.New("backup.tools_unavailable")
	ErrSystemBackupNotFound          = errors.New("backup.not_found")
	ErrSystemBackupRetention         = errors.New("backup.retention_failed")
	ErrSystemBackupTargetUnavailable = errors.New("backup.target_unavailable")
)

// Store is owned by the system domain, which maps rows to the archive contract.
type Store struct {
	CreateRun                 func(context.Context, *backuptypes.SystemBackupRun) error
	SaveRun                   func(context.Context, *backuptypes.SystemBackupRun) error
	FailRun                   func(context.Context, string, string) error
	DeleteRun                 func(context.Context, string) error
	Run                       func(context.Context, string) (*backuptypes.SystemBackupRun, error)
	ListRuns                  func(context.Context, pagination.QueryParams) ([]backuptypes.SystemBackupRun, pagination.Response, error)
	ExpiredRunIDs             func(context.Context, string, int) ([]string, error)
	RunsByID                  func(context.Context, []string) ([]*backuptypes.SystemBackupRun, error)
	LatestPolicyRun           func(context.Context, string) (*backuptypes.SystemBackupRun, error)
	CountRuns                 func(context.Context) (int64, error)
	CountVolumeRepositoryRuns func(context.Context) (int64, error)
	Policies                  func(context.Context) ([]backuptypes.SystemBackupPolicy, error)
	Policy                    func(context.Context, string) (*backuptypes.SystemBackupPolicy, error)
	ReconcilePolicies         func(context.Context, []backuptypes.UpdateSystemBackupPolicy,
		func(context.Context, *backuptypes.SystemBackupPolicy, backuptypes.UpdateSystemBackupPolicy) error,
		func(context.Context, string), func(context.Context, *backuptypes.SystemBackupPolicy)) error
	ListHistory func(context.Context, pagination.QueryParams) ([]backuptypes.HistoryEntry, pagination.Response, error)
}

type Dependencies struct {
	APIKeys      *apikey.ApiKeyService
	Store        Store
	DB           *database.DB
	Docker       *dockerInternal.DockerClientService
	Volumes      *volume.VolumeService
	Engine       *sharedbackup.Engine
	Activity     *activity.ActivityService
	Settings     *settings.SettingsService
	Config       *config.Config
	RecoveryKeys *sharedbackup.RecoveryKeyStore
	Auth         *auth.AuthService
	Roles        *role.RoleService
	Coordinator  *runs.Coordinator
	Runtime      *francis.Runtime
}

type Service struct {
	apiKeys         *apikey.ApiKeyService
	store           Store
	db              *database.DB
	engine          *sharedbackup.Engine
	activityService *activity.ActivityService
	config          *config.Config
	recoveryKeys    *sharedbackup.RecoveryKeyStore
	volumeService   *volume.VolumeService
	jobs            *entityjobs.Registry
	volumes         *volumes.Service
	maintenance     *MaintenanceGate
	auth            *auth.AuthService
	roles           *role.RoleService
	coordinator     *runs.Coordinator
	settings        *settings.SettingsService
	exitUnready     context.CancelFunc
	refreshJobs     func(context.Context) error
	runtime         *francis.Runtime
}

func NewService(deps Dependencies) *Service {
	s := &Service{apiKeys: deps.APIKeys, store: deps.Store, db: deps.DB, engine: deps.Engine, activityService: deps.Activity,
		config: deps.Config, recoveryKeys: deps.RecoveryKeys, volumeService: deps.Volumes,
		jobs: entityjobs.New("system-backup:", sharedbackup.SystemAdmissionScope), maintenance: NewMaintenanceGate(),
		auth: deps.Auth, roles: deps.Roles, coordinator: deps.Coordinator, settings: deps.Settings, runtime: deps.Runtime}
	s.volumes = volumes.NewService(volumes.Dependencies{
		DB: deps.DB, Engine: deps.Engine, Volumes: deps.Volumes, Activity: deps.Activity,
		Settings: deps.Settings, Jobs: s.jobs, AcquireRun: s.acquireRunInternal,
		AcquireDurableRun: s.acquireDurableRunInternal, AlreadyRunning: ErrSystemBackupAlreadyRunning,
	})
	if deps.Engine != nil {
		deps.Engine.RegisterRunKind("system-volumes", s.volumes.ExecuteDurable, s.volumes.FailDurable)
	}
	return s
}

// Volume backups still use the existing Rustic run lease.
func (s *Service) acquireRunInternal(ctx context.Context) (*runs.Lease, error) {
	lease, admitted, err := s.engine.TryAcquireRun(ctx, sharedbackup.SystemAdmissionScope, systemAdmissionID)
	if err != nil {
		return nil, err
	}
	if !admitted {
		return nil, ErrSystemBackupAlreadyRunning
	}
	return lease, nil
}
func (s *Service) acquireDurableRunInternal(ctx context.Context, runID string) (*runs.Lease, error) {
	lease, admitted, err := s.engine.AcquireDurableRun(ctx, runID, sharedbackup.SystemAdmissionScope, systemAdmissionID)
	if err != nil {
		return nil, err
	}
	if !admitted {
		return nil, ErrSystemBackupAlreadyRunning
	}
	return lease, nil
}

func (s *Service) ListBackups(ctx context.Context, params pagination.QueryParams) ([]backuptypes.SystemBackupRun, pagination.Response, error) {
	return s.store.ListRuns(ctx, params)
}
func (s *Service) ListBackupHistory(ctx context.Context, params pagination.QueryParams) ([]backuptypes.HistoryEntry, pagination.Response, error) {
	history, page, err := s.store.ListHistory(ctx, params)
	page.GrandTotalItems = page.TotalItems
	return history, page, err
}

func (s *Service) validateRequest(ctx context.Context, request backuptypes.CreateSystemBackupRequest) error {
	if request.TargetID != "" {
		return ErrSystemBackupTargetUnavailable
	}
	if request.PolicyID != "" {
		policy, err := s.store.Policy(ctx, request.PolicyID)
		if err != nil {
			return err
		}
		if policy == nil {
			return ErrSystemBackupNotFound
		}
		if policy.TargetID != "" {
			return ErrSystemBackupTargetUnavailable
		}
	}
	if err := s.databaseProviderError(); err != nil {
		return err
	}
	return nil
}

func (s *Service) startRun(ctx context.Context, actor usertypes.Actor, trigger string, request backuptypes.CreateSystemBackupRequest) (*backuptypes.SystemBackupRun, context.Context, error) {
	if err := s.validateRequest(ctx, request); err != nil {
		return nil, nil, err
	}
	id := "system-" + uuid.New().String()
	answer, err := s.activityService.StartGatedActivity(ctx, activity.GateRequest{
		EnvironmentID: "0", Operation: "create:" + id, Scope: sharedbackup.SystemAdmissionScope,
		Type: activitytypes.TypeResourceAction, ResourceType: "system_backup", ResourceID: id,
		StartedBy: &actor,
	})
	if err != nil {
		var conflict *activity.GateConflict
		if errors.As(err, &conflict) {
			return nil, nil, ErrSystemBackupAlreadyRunning
		}
		return nil, nil, err
	}
	run := &backuptypes.SystemBackupRun{ID: id, ActivityID: answer.Run.ID,
		CreatedAt: time.Now().UTC(), Status: backuptypes.SystemBackupStatusRunning,
		Trigger: trigger, PolicyID: request.PolicyID, CopyState: backuptypes.SystemBackupCopyNone}
	if err := s.store.CreateRun(ctx, run); err != nil {
		code := "backup.failed"
		_, _ = s.activityService.CompleteActivity(answer.WorkContext, answer.Run.ID, activitytypes.StatusFailed, code, &code)
		return nil, nil, err
	}
	return run, answer.WorkContext, nil
}

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

func (s *Service) StartBackup(ctx context.Context, actor usertypes.Actor, request backuptypes.CreateSystemBackupRequest) (*backuptypes.SystemBackupRun, error) {
	run, workCtx, err := s.startRun(ctx, actor, backuptypes.SystemBackupTriggerManual, request)
	if err != nil {
		return nil, err
	}
	go func() {
		if err := s.finishRun(workCtx, run); err != nil {
			slog.Warn("Hub backup failed", "backupId", run.ID, "code", "backup.failed")
		}
	}()
	return run, nil
}

// CreateBackup executes a scheduled archive before the job reports its outcome.
func (s *Service) CreateBackup(ctx context.Context, actor usertypes.Actor, trigger string, request backuptypes.CreateSystemBackupRequest) (*backuptypes.SystemBackupRun, error) {
	run, workCtx, err := s.startRun(ctx, actor, trigger, request)
	if err != nil {
		return nil, err
	}
	return run, s.finishRun(workCtx, run)
}

func (s *Service) OpenArchive(ctx context.Context, id string) (io.ReadCloser, int64, string, error) {
	name, err := backuptypes.ArchiveNameForID(id)
	if err != nil {
		return nil, 0, "", ErrSystemBackupNotFound
	}
	run, err := s.store.Run(ctx, id)
	if err != nil || run == nil || run.Status != backuptypes.SystemBackupStatusSucceeded || run.ArchiveName != name {
		return nil, 0, "", ErrSystemBackupNotFound
	}
	file, err := os.Open(filepath.Join(s.backupDir(), name))
	if err != nil {
		return nil, 0, "", ErrSystemBackupNotFound
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != run.Size {
		_ = file.Close()
		return nil, 0, "", ErrSystemBackupNotFound
	}
	return file, info.Size(), name, nil
}

func (s *Service) DeleteBackup(ctx context.Context, id string) (*backuptypes.SystemBackupOperation, error) {
	name, err := backuptypes.ArchiveNameForID(id)
	if err != nil {
		return nil, ErrSystemBackupNotFound
	}
	answer, err := s.activityService.StartGatedActivity(ctx, activity.GateRequest{
		EnvironmentID: "0", Operation: "delete:" + id, Scope: sharedbackup.SystemAdmissionScope,
		Type: activitytypes.TypeResourceAction, ResourceType: "system_backup", ResourceID: id,
	})
	if err != nil {
		var conflict *activity.GateConflict
		if errors.As(err, &conflict) {
			return nil, ErrSystemBackupAlreadyRunning
		}
		return nil, errors.New("backup.failed")
	}
	workCtx := answer.WorkContext
	run, err := s.store.Run(workCtx, id)
	if err != nil || run == nil {
		err = ErrSystemBackupNotFound
	}
	if err == nil && run != nil && run.ArchiveName != "" && run.ArchiveName != name {
		err = ErrSystemBackupNotFound
	}
	if err == nil && run != nil {
		if run.ArchiveName != "" {
			err = os.Remove(filepath.Join(s.backupDir(), name))
			if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
		}
		if err == nil {
			err = s.store.DeleteRun(workCtx, id)
		}
	}
	code, status := "backup.deleted", activitytypes.StatusSuccess
	if err != nil {
		code, status = "backup.failed", activitytypes.StatusFailed
	}
	_, completeErr := s.activityService.CompleteActivity(workCtx, answer.Run.ID, status, code, nil)
	if err != nil {
		return nil, errors.New(code)
	}
	if completeErr != nil {
		return nil, errors.New("backup.failed")
	}
	return &backuptypes.SystemBackupOperation{BackupID: id, ActivityID: answer.Run.ID, Status: string(status)}, nil
}

func (s *Service) GetPolicies(ctx context.Context) (*backuptypes.SystemBackupPolicyCollection, error) {
	policies, err := s.store.Policies(ctx)
	if err != nil {
		return nil, err
	}
	result := &backuptypes.SystemBackupPolicyCollection{Policies: make([]backuptypes.SystemBackupPolicy, 0, len(policies))}
	for _, policy := range policies {
		last, err := s.store.LatestPolicyRun(ctx, policy.ID)
		if err != nil {
			return nil, err
		}
		policy.LastRun = last
		result.Policies = append(result.Policies, policy)
	}
	return result, nil
}
func (s *Service) applyRetention(ctx context.Context, policyID string, keep int) error {
	if keep < 1 {
		return errors.New("backup.invalid_retention")
	}
	ids, err := s.store.ExpiredRunIDs(ctx, policyID, keep)
	if err != nil {
		return err
	}
	for _, id := range ids {
		name, err := backuptypes.ArchiveNameForID(id)
		if err != nil {
			return err
		}
		run, err := s.store.Run(ctx, id)
		if err != nil {
			return err
		}
		if run.Trigger != backuptypes.SystemBackupTriggerScheduled || run.PolicyID != policyID || run.ArchiveName != name {
			continue
		}
		if err := os.Remove(filepath.Join(s.backupDir(), name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("backup.retention_failed")
		}
		if err := s.store.DeleteRun(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// Recovery keys remain available to the separate volume backup policy.
func (s *Service) GenerateRecoveryKey() (*backuptypes.SystemBackupRecoveryKey, error) {
	key, err := sharedbackup.GenerateRecoveryKey()
	if err != nil {
		return nil, err
	}
	return &backuptypes.SystemBackupRecoveryKey{RecoveryKey: key}, nil
}
func (s *Service) SetRecoveryKey(ctx context.Context, key string) (*backuptypes.SystemBackupRecoveryKeyStatus, error) {
	if err := sharedbackup.ValidateRecoveryKey(key); err != nil {
		return nil, err
	}
	if s.recoveryKeys == nil {
		return nil, ErrSystemBackupUnavailable
	}
	configured, err := s.recoveryKeys.Configured(ctx)
	if err != nil {
		return nil, err
	}
	if configured {
		current, err := s.recoveryKeys.Get(ctx)
		if err == nil && current != key {
			count, err := s.store.CountVolumeRepositoryRuns(ctx)
			if err != nil {
				return nil, err
			}
			if count > 0 {
				return nil, errors.New("volume backups must be removed before replacing the recovery key")
			}
		}
	}
	if err := s.recoveryKeys.Set(ctx, key); err != nil {
		return nil, err
	}
	if s.volumeService != nil {
		_ = s.volumeService.MigrateRepositoryPasswords(ctx)
	}
	return &backuptypes.SystemBackupRecoveryKeyStatus{Configured: true}, nil
}
