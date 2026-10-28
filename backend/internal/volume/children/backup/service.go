// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; M12 backup foundation; M12 persisted volume lifecycle and legacy restore cut.; Remove legacy archive and centralized volume execution; preserve historical metadata deletion and safety pins.

package backup

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
	"uuid"

	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/browser"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
)

// Store persists volume backup records for this feature.
type Store struct {
	Create func(ctx context.Context, entry *volume.Backup) error
	Save   func(ctx context.Context, entry *volume.Backup) error
	Fail   func(ctx context.Context, backupID, message string) error
	Delete func(ctx context.Context, backupID string) error
	Run    func(ctx context.Context, backupID string) (*volume.Backup, error)
	Runs   func(ctx context.Context, backupIDs []string) ([]*volume.Backup, error)
	Latest func(ctx context.Context, policyID string) (*volume.Backup, error)
	List   func(ctx context.Context, volumeName string, params pagination.QueryParams) ([]volume.Backup, int64, error)
}

// Dependencies are the persistence, runtime, and helper-container operations volume backups use.
type Dependencies struct {
	Store            Store
	DB               *database.DB
	Docker           *docker.DockerClientService
	Events           *event.EventService
	Activity         *activity.ActivityService
	Engine           *backup.Engine
	RecoveryKeys     *backup.RecoveryKeyStore
	Locks            *utils.KeyedMutex
	AlreadyRunning   error
	StopContainer    func(ctx context.Context, containerID string, user usertypes.Actor) error
	StartContainer   func(ctx context.Context, containerID string, user usertypes.Actor) error
	HelperImage      func(ctx context.Context, dockerClient *client.Client) (string, error)
	AcquireHelper    func(ctx context.Context, volumeName string) (string, func(), error)
	Exec             func(ctx context.Context, containerID, execUser string, cmd []string) (string, string, error)
	BackupVolumeName string
	Location         *time.Location
	EncryptionKey    string
}

// Service runs volume backups and owns their policies, file browser, and restores.
type Service struct {
	deps                   Dependencies
	targets                *backuptarget.Service
	targetStore            *backuptarget.Store
	rekeyed                sync.Map
	recoveryMu             sync.Mutex
	sweepMu                sync.Mutex
	sweepStarted           bool
	writerDiscoveryPending bool
	settledInitial         bool
	confirmedWriters       map[string]bool
	fenceMu                sync.Mutex
	writerFences           map[string]bool
	writerIDs              map[string]map[string]bool
	writerFenceRelease     map[string]func()
	lifecycle              *restore.Lifecycle
	policies               *policies.Service
	browser                *browser.Service
	restore                *restore.Service
}

// lockHeldInternal marks a context whose caller already holds the volume's workspace lock.
type lockHeldInternal struct {
	service    *Service
	volumeName string
}

type lockHeldKeyInternal struct{}

func NewService(deps Dependencies) *Service {
	if deps.Locks == nil {
		deps.Locks = &utils.KeyedMutex{}
	}
	s := &Service{deps: deps, writerFences: map[string]bool{}}
	s.targetStore = &backuptarget.Store{DB: deps.DB}
	s.targets = backuptarget.NewService(backuptarget.Dependencies{Store: s.targetStore})
	s.policies = policies.NewService(policies.Dependencies{
		DB:             deps.DB,
		ValidateTarget: s.validatePolicyTarget,
		Location:       deps.Location,
		Activity:       deps.Activity,
		AlreadyRunning: deps.AlreadyRunning,
		LatestRun:      s.latestRunInternal,
		CreateBackup: func(ctx context.Context, volumeName, policyID string) (*volume.Backup, error) {
			return s.CreateBackup(ctx, volumeName, usertypes.SystemUser, backuptypes.VolumeBackupTriggerScheduled, volume.CreateBackupRequest{PolicyID: policyID})
		},
		Reconcile: s.ReconcileBackup,
	})
	s.browser = browser.NewService(browser.Dependencies{
		Docker:       deps.Docker,
		Engine:       deps.Engine,
		Run:          deps.Store.Run,
		Repository:   s.rusticRepositoryForBackupInternal,
		Password:     s.volumeBackupPasswordInternal,
		SanitizePath: sanitizeBackupPathInternal,
	})
	s.restore = restore.NewService(restore.Dependencies{
		Docker: deps.Docker, Engine: deps.Engine, Run: deps.Store.Run, Repository: s.rusticRepositoryForBackupInternal, Password: s.volumeBackupPasswordInternal, HelperImage: deps.HelperImage, Exec: deps.Exec,
	})
	s.configureRestoreLifecycle()
	if deps.Engine != nil {
		deps.Engine.RegisterRunKind("volume", s.executeDurableBackupInternal, s.failDurableBackupInternal)
	}
	return s
}

// SetScheduler injects the dynamic scheduler and admission gate for per-policy
// backup jobs. Agent mode passes them too: agents run their own volume backups.
func (s *Service) SetScheduler(ctx context.Context, dynamicScheduler scheduler.DynamicScheduler, admissionGate *runs.Admission) error {
	return s.policies.SetScheduler(ctx, dynamicScheduler, admissionGate)
}

func (s *Service) RegisterJobsOnStartup(ctx context.Context) {
	s.policies.RegisterJobsOnStartup(ctx)
}

// RemovePolicies drops the backup schedules of a removed volume.
func (s *Service) RemovePolicies(ctx context.Context, volumeName string) {
	s.policies.Remove(ctx, volumeName)
}

// RenamePolicies moves a renamed volume's backup policies inside the caller's transaction.
func (s *Service) RenamePolicies(tx *gorm.DB, oldName, newName string) error {
	return s.policies.Rename(tx, oldName, newName)
}

func entryInternal(entry *volume.Backup) volume.BackupEntry {
	return volume.BackupEntry{
		ActivityID: entry.ActivityID, ID: entry.ID, VolumeName: entry.VolumeName, Size: entry.Size,
		CreatedAt: entry.CreatedAt.Format(time.RFC3339), Status: entry.Status, Trigger: entry.Trigger,
		Destination: entry.Destination, Format: entry.Format, LocalSnapshotID: entry.LocalSnapshotID,
		PolicyID: entry.PolicyID, Error: entry.Error, ErrorCode: entry.ErrorCode, Type: entry.Type,
	}
}

func (s *Service) latestRunInternal(ctx context.Context, policyID string) (*volume.BackupEntry, error) {
	entry, err := s.deps.Store.Latest(ctx, policyID)
	if err != nil || entry == nil {
		return nil, err
	}
	return new(entryInternal(entry)), nil
}

func (s *Service) List(ctx context.Context, volumeName string, params pagination.QueryParams) ([]volume.Backup, pagination.Response, error) {
	slog.DebugContext(
		ctx,
		"volume service: list backups paginated",
		"volume",
		volumeName,
		"search",
		params.Search,
		"sort",
		params.Sort,
		"order",
		params.Order,
		"start",
		params.Start,
		"limit",
		params.Limit,
	)
	backups, totalItems, err := s.deps.Store.List(ctx, volumeName, params)
	if err != nil {
		return nil, pagination.Response{}, err
	}

	return backups, pagination.BuildResponse(totalItems, totalItems, params), nil
}

func (s *Service) CreateBackup(ctx context.Context, volumeName string, user usertypes.Actor, trigger string, request volume.CreateBackupRequest) (_ *volume.Backup, err error) {
	trigger = cmp.Or(trigger, backuptypes.VolumeBackupTriggerManual)
	entry, owner, plan, err := s.prepareCapture(ctx, volumeName, trigger, request)
	if err != nil {
		return nil, err
	}
	defer owner.Release(ctx)
	err = s.executeBackupInternal(owner.ctx, entry, user, plan)
	return entry, s.completeBackupInternal(owner.ctx, entry, err)
}

// safetyBackupInternal takes the local pre-restore backup for a caller that already holds the volume's workspace lock.
func (s *Service) safetyBackupInternal(ctx context.Context, volumeName string, user usertypes.Actor) (string, error) {
	entry := &volume.Backup{ID: uuid.New().String(), VolumeName: volumeName, CreatedAt: time.Now().UTC(), Status: backuptypes.VolumeBackupStatusRunning, Trigger: backuptypes.VolumeBackupTriggerSafety, Destination: volume.BackupDestinationLocal, Format: volume.BackupFormatRustic}
	if err := s.deps.Store.Create(ctx, entry); err != nil {
		return "", err
	}
	err := s.executeBackupInternal(ctx, entry, user, backupPlanInternal{localEnabled: true, destination: volume.BackupDestinationLocal})
	if err = s.completeBackupInternal(ctx, entry, err); err != nil {
		return "", err
	}
	return entry.ID, nil
}

type volumeBackupRecoveryInternal struct {
	BackupID     string                       `json:"backupId"`
	LocalEnabled bool                         `json:"localEnabled"`
	Policy       *policies.VolumeBackupPolicy `json:"policy,omitempty"`
}

func (s *Service) prepareBackupInternal(ctx context.Context, volumeName, trigger, policyID string, plan backupPlanInternal) (*volume.Backup, *operationOwnership, error) {
	_, lease, err := s.acquireOperation(ctx, volumeName, "")
	if err != nil {
		return nil, nil, err
	}
	entry := &volume.Backup{
		VolumeName: volumeName, CreatedAt: time.Now(), Status: backuptypes.VolumeBackupStatusRunning,
		Trigger: trigger, Destination: plan.destination, Format: volume.BackupFormatRustic,
		PolicyID:       policyID,
		StopContainers: plan.stopContainers,
		Type:           backuptypes.ManagementTypeForPolicy(policyID),
	}
	entry.ID = fmt.Sprintf("%s-%d-%s", volumeName, time.Now().UnixNano(), uuid.New().String()[:8])
	if createBackupErr := s.deps.Store.Create(ctx, entry); createBackupErr != nil {
		lease.Release(ctx)
		return nil, nil, createBackupErr
	}
	checkpoint, err := json.Marshal(
		volumeBackupRecoveryInternal{
			BackupID:     entry.ID,
			LocalEnabled: plan.localEnabled,
			Policy:       plan.policy,
		},
	)
	if err == nil {
		err = jobcontext.Progress(ctx, scheduler.TargetOutcome{ResourceType: "volume_backup", ID: volumeName, Status: scheduler.Running, RecoveryData: checkpoint})
	}
	if err != nil {
		lease.Release(ctx)
		return nil, nil, err
	}
	return entry, lease, nil
}

func (s *Service) completeBackupInternal(ctx context.Context, entry *volume.Backup, err error) error {
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		if entry.ErrorCode == "" {
			entry.ErrorCode = "volume.backup.interrupted"
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
	if saveErr := s.deps.Store.Save(context.WithoutCancel(ctx), entry); saveErr != nil {
		return errors.Join(err, fmt.Errorf("failed to save volume backup result: %w", saveErr))
	}
	if err == nil {
		err = jobcontext.Progress(ctx, scheduler.TargetOutcome{ResourceType: "volume_backup", ID: entry.VolumeName, Status: scheduler.Succeeded})
	}
	return err
}

func (s *Service) executeBackupInternal(ctx context.Context, entry *volume.Backup, user usertypes.Actor, plan backupPlanInternal) (err error) {
	defer utils.RecoverToError(&err, "volume backup")
	volumeName, trigger := entry.VolumeName, entry.Trigger
	if cancellationErr := ctx.Err(); cancellationErr != nil {
		return cancellationErr
	}
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return err
	}
	var stopped []volume.StoppedContainer
	restarted := false
	if entry.StopContainers {
		stopped, err = s.listConsumers(ctx, volumeName, false)
		if err != nil {
			return err
		}
		entry.StoppedContainers = stopped
		if err = s.deps.Store.Save(ctx, entry); err != nil {
			return err
		}
		defer func() {
			if restarted {
				return
			}
			if restartErr := s.restartConsumers(context.WithoutCancel(ctx), stopped, user); restartErr != nil {
				entry.ErrorCode = "volume.backup.restart-failed"
				err = errors.Join(err, restartErr)
			}
		}()
		for _, consumer := range stopped {
			if err = s.deps.StopContainer(ctx, consumer.ID, user); err != nil {
				return err
			}
		}
	}
	if plan.localEnabled && entry.LocalSnapshotID == "" {
		if createLocalBackupSnapshotErr := s.createLocalBackupSnapshotInternal(ctx, dockerClient, entry); createLocalBackupSnapshotErr != nil {
			return createLocalBackupSnapshotErr
		}
	}
	restarted = true
	if restartErr := s.restartConsumers(context.WithoutCancel(ctx), stopped, user); restartErr != nil {
		entry.ErrorCode = "volume.backup.restart-failed"
		err = errors.Join(err, restartErr)
	}

	if trigger == backuptypes.VolumeBackupTriggerScheduled && plan.policy != nil && plan.policy.RetentionCount > 0 {
		if retentionErr := s.applyVolumeBackupRetentionInternal(ctx, entry.VolumeName, entry.ID, plan.policy.RetentionCount); retentionErr != nil {
			entry.ErrorCode = "volume.backup.retention-failed"
			err = errors.Join(err, retentionErr)
		}
	}
	metadata := database.JSON{"action": "backup_create", "backup_id": entry.ID, "size": entry.Size, "destination": entry.Destination}
	if logErr := s.deps.Events.LogVolumeEvent(ctx, event.EventTypeVolumeBackupCreate, volumeName, volumeName, user.ID, user.Username, "0", metadata); logErr != nil {
		slog.WarnContext(ctx, "could not log volume backup create event", "volume", volumeName, "error", logErr)
	}
	return errors.Join(err, ctx.Err())
}

func (s *Service) createLocalBackupSnapshotInternal(ctx context.Context, dockerClient *client.Client, entry *volume.Backup) error {
	repository, repoErr := s.repositoryFor(ctx, dockerClient, string(entry.Destination), false)
	if repoErr != nil {
		return repoErr
	}
	password, passwordErr := s.volumeBackupPasswordInternal(ctx, dockerClient, repository)
	if passwordErr != nil {
		return passwordErr
	}
	if err := jobcontext.Progress(ctx, scheduler.TargetOutcome{ResourceType: "backup_destination", ID: entry.ID + ":local", Status: scheduler.Running}); err != nil {
		return err
	}
	localSnapshot, err := s.deps.Engine.CreateSnapshot(
		ctx,
		dockerClient,
		repository,
		password,
		entry.VolumeName,
		backup.RootSnapshotInput(
			volumeSourceMountInternal(
				entry.VolumeName,
			),
			backup.RunSnapshotTag(
				entry.ID,
			),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create local Rustic snapshot: %w", err)
	}
	entry.LocalSnapshotID = localSnapshot.ID
	entry.Size = localSnapshot.Size
	if saveLocalBackupErr := s.deps.Store.Save(ctx, entry); saveLocalBackupErr != nil {
		return saveLocalBackupErr
	}
	if progressErr := jobcontext.Progress(
		ctx,
		scheduler.TargetOutcome{
			ResourceType: "backup_destination",
			ID:           entry.ID + ":local",
			Status:       scheduler.Succeeded,
			Message:      localSnapshot.ID,
		},
	); progressErr != nil {
		return progressErr
	}
	return nil
}

func (s *Service) DeleteBackup(ctx context.Context, backupID string, user *usertypes.Actor) error {
	entry, err := s.deps.Store.Run(ctx, backupID)
	if err != nil {
		return err
	}
	// Deletes contend with create/restore/upload on the volume's run lease so
	// a slow operation cannot resurrect or double-forget snapshots. Retention
	// deletes run under CreateBackup's lease and call the internal path.
	ctx, owner, err := s.acquireOperation(ctx, entry.VolumeName, "")
	if err != nil {
		return err
	}
	defer owner.Release(ctx)
	return s.deleteBackupInternal(ctx, backupID, user)
}

func (s *Service) deleteBackupInternal(ctx context.Context, backupID string, user *usertypes.Actor) error {
	entry, err := s.deps.Store.Run(ctx, backupID)
	if err != nil {
		return err
	}
	var pins int64
	if err := s.deps.DB.WithContext(ctx).Table("volume_restore_runs").Where("safety_backup_id = ? AND pending_restart = ?", entry.ID, true).Count(&pins).Error; err != nil {
		return err
	}
	if pins > 0 {
		return backuptypes.NewProblem(409, "volume.backup.safety-pinned", nil)
	}
	if entry.Format != volume.BackupFormatRustic {
		if err := s.deps.Store.Delete(ctx, entry.ID); err != nil {
			return err
		}
		s.logBackupDeleteEventInternal(ctx, entry.VolumeName, entry.ID, user)
		return nil
	}
	return s.deleteRusticBackupsInternal(ctx, []*volume.Backup{entry}, user)
}

// deleteRusticBackupsInternal forgets local snapshots before removing records.
func (s *Service) deleteRusticBackupsInternal(ctx context.Context, entries []*volume.Backup, user *usertypes.Actor) error {
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return err
	}
	var localEntries []*volume.Backup
	for _, entry := range entries {
		if entry.LocalSnapshotID != "" {
			localEntries = append(localEntries, entry)
		}
	}
	deleteErr := s.forgetLocalSnapshotsInternal(ctx, dockerClient, localEntries)
	for _, entry := range entries {
		if entry.LocalSnapshotID == "" {
			if err := s.deps.Store.Delete(ctx, entry.ID); err != nil {
				deleteErr = errors.Join(deleteErr, fmt.Errorf("failed to delete volume backup record: %w", err))
			} else {
				s.logBackupDeleteEventInternal(ctx, entry.VolumeName, entry.ID, user)
			}
		} else if err := s.deps.Store.Save(ctx, entry); err != nil {
			deleteErr = errors.Join(deleteErr, err)
		}
	}
	return deleteErr
}

func (s *Service) forgetLocalSnapshotsInternal(ctx context.Context, dockerClient *client.Client, entries []*volume.Backup) error {
	groups := map[string][]*volume.Backup{}
	for _, entry := range entries {
		groups[string(entry.Destination)] = append(groups[string(entry.Destination)], entry)
	}
	var failures []error
	for destination, group := range groups {
		repository, err := s.repositoryFor(ctx, dockerClient, destination, false)
		if err == nil {
			ids := make([]string, len(group))
			for i, entry := range group {
				ids[i] = entry.LocalSnapshotID
			}
			err = s.forgetSnapshotsInternal(ctx, dockerClient, repository, ids)
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, entry := range group {
			entry.LocalSnapshotID = ""
		}
	}
	return errors.Join(failures...)
}

func (s *Service) forgetSnapshotsInternal(ctx context.Context, dockerClient *client.Client, repository backup.Repository, snapshotIDs []string) error {
	password, err := s.volumeBackupPasswordInternal(ctx, dockerClient, repository)
	if err != nil {
		return err
	}
	return s.deps.Engine.ForgetSnapshots(ctx, dockerClient, repository, password, snapshotIDs)
}

func (s *Service) logBackupDeleteEventInternal(ctx context.Context, volumeName, backupID string, user *usertypes.Actor) {
	actingUser := user
	if actingUser == nil {
		actingUser = &usertypes.SystemUser
	}
	if logErr := s.deps.Events.LogVolumeEvent(
		ctx,
		event.EventTypeVolumeBackupDelete,
		volumeName,
		volumeName,
		actingUser.ID,
		actingUser.Username,
		"0",
		database.JSON{
			"action":    "backup_delete",
			"backup_id": backupID,
		},
	); logErr != nil {
		slog.WarnContext(ctx, "could not log volume backup delete event", "volume", volumeName, "error", logErr)
	}
}

// DownloadBackup prepares a Rustic snapshot in scratch and streams its archive.
func (s *Service) DownloadBackup(ctx context.Context, backupID string, user *usertypes.Actor) (io.ReadCloser, int64, error) {
	entry, err := s.deps.Store.Run(ctx, backupID)
	if err != nil {
		return nil, 0, err
	}
	var reader io.ReadCloser
	var size int64
	if entry.Format != volume.BackupFormatRustic {
		return nil, 0, backuptypes.NewProblem(404, "volume.backup.snapshot-not-found", nil)
	}
	reader, size, err = s.downloadRusticBackupInternal(ctx, entry)
	if err != nil {
		return nil, 0, err
	}
	actingUser := user
	if actingUser == nil {
		actingUser = &usertypes.SystemUser
	}
	metadata := database.JSON{"action": "backup_download", "backup_id": backupID, "size": size}
	if logErr := s.deps.Events.LogVolumeEvent(ctx, event.EventTypeVolumeBackupDownload, entry.VolumeName, entry.VolumeName, actingUser.ID, actingUser.Username, "0", metadata); logErr != nil {
		slog.WarnContext(ctx, "could not log volume backup download event", "volume", entry.VolumeName, "error", logErr)
	}
	return reader, size, nil
}

func (s *Service) downloadRusticBackupInternal(ctx context.Context, entry *volume.Backup) (io.ReadCloser, int64, error) {
	if entry.LocalSnapshotID == "" {
		return nil, 0, errors.New("only local volume backups can be downloaded")
	}
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return nil, 0, err
	}
	repository, err := s.repositoryFor(ctx, dockerClient, string(entry.Destination), true)
	if err != nil {
		return nil, 0, err
	}
	password, err := s.volumeBackupPasswordInternal(ctx, dockerClient, repository)
	if err != nil {
		return nil, 0, err
	}
	scratchVolume := "arcane-rustic-download-" + uuid.New().String()
	if _, volumeCreateErr := dockerClient.VolumeCreate(ctx, client.VolumeCreateOptions{Name: scratchVolume, Labels: volumehelper.Labels()}); volumeCreateErr != nil {
		return nil, 0, fmt.Errorf("failed to create download scratch volume: %w", volumeCreateErr)
	}
	removeScratch := func() {
		_, _ = dockerClient.VolumeRemove(context.WithoutCancel(ctx), scratchVolume, client.VolumeRemoveOptions{Force: true})
	}
	if restoreSnapshotErr := s.deps.Engine.RestoreSnapshot(
		ctx,
		dockerClient,
		repository,
		password,
		entry.LocalSnapshotID,
		mount.Mount{
			Type:   mount.TypeVolume,
			Source: scratchVolume,
			Target: "/volume",
		},
		backup.RestoreOptions{
			DeleteExtra: true,
		},
	); restoreSnapshotErr != nil {
		removeScratch()
		return nil, 0, fmt.Errorf("failed to restore Rustic snapshot for download: %w", restoreSnapshotErr)
	}
	helperImage, err := s.deps.HelperImage(ctx, dockerClient)
	if err != nil {
		removeScratch()
		return nil, 0, err
	}
	archiveMount := mount.Mount{Type: mount.TypeVolume, Source: scratchVolume, Target: "/volume", ReadOnly: false}
	containerID, removeContainer, err := s.createBackupTempContainerWithMountInternal(ctx, dockerClient, helperImage, archiveMount)
	if err != nil {
		removeScratch()
		return nil, 0, err
	}
	cleanup := func() {
		removeContainer()
		removeScratch()
	}
	archivePath := "/tmp/" + entry.ID + ".tar.gz"
	if _, _, execInContainerErr := s.deps.Exec(ctx, containerID, "", []string{"tar", "-czf", archivePath, "-C", "/volume", "."}); execInContainerErr != nil {
		cleanup()
		return nil, 0, fmt.Errorf("failed to package Rustic snapshot for download: %w", execInContainerErr)
	}
	return volumehelper.DownloadFileFromContainer(ctx, dockerClient, containerID, archivePath, cleanup)
}

func (s *Service) MigrateRepositoryPasswords(ctx context.Context) error {
	if s.deps.RecoveryKeys == nil || s.deps.Engine == nil {
		return nil
	}
	key, err := s.deps.RecoveryKeys.Get(ctx)
	if errors.Is(err, backup.ErrRecoveryKeyNotConfigured) {
		return nil
	}
	if err != nil {
		return err
	}
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return err
	}
	local, err := s.localRusticRepositoryInternal(ctx, dockerClient, true)
	if err != nil {
		return err
	}
	s.rekeyRepositoryInternal(ctx, dockerClient, local, key)
	return nil
}
