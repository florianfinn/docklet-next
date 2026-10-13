// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations.

package backup

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"
	"uuid"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	userdomain "github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/browser"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
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
	EncryptionKey    string
}

// Service runs volume backups and owns their policies, file browser, and restores.
type Service struct {
	deps     Dependencies
	rekeyed  sync.Map
	policies *policies.Service
	browser  *browser.Service
	restore  *restore.Service
}

// lockHeldInternal marks a context whose caller already holds the volume's workspace lock.
type lockHeldInternal struct {
	service    *Service
	volumeName string
}

type lockHeldKeyInternal struct{}

func NewService(deps Dependencies) *Service {
	s := &Service{deps: deps}
	s.policies = policies.NewService(policies.Dependencies{
		DB:             deps.DB,
		Activity:       deps.Activity,
		AlreadyRunning: deps.AlreadyRunning,
		LatestRun:      s.latestRunInternal,
		CreateBackup: func(ctx context.Context, volumeName, policyID string) (*volume.Backup, error) {
			return s.CreateBackup(ctx, volumeName, usertypes.SystemUser, backuptypes.VolumeBackupTriggerScheduled, volume.CreateBackupRequest{PolicyID: policyID})
		},
		Reconcile: s.ReconcileBackup,
	})
	s.browser = browser.NewService(browser.Dependencies{
		Docker:          deps.Docker,
		Engine:          deps.Engine,
		Run:             deps.Store.Run,
		Repository:      s.rusticRepositoryForBackupInternal,
		Password:        s.volumeBackupPasswordInternal,
		SanitizePath:    sanitizeBackupPathInternal,
		ArchiveFilename: backupArchiveFilenameInternal,
		TempContainer:   s.createBackupTempContainerInternal,
		Exec:            deps.Exec,
	})
	s.restore = restore.NewService(restore.Dependencies{
		Docker:          deps.Docker,
		Engine:          deps.Engine,
		Events:          deps.Events,
		Locks:           deps.Locks,
		Run:             deps.Store.Run,
		Repository:      s.rusticRepositoryForBackupInternal,
		Password:        s.volumeBackupPasswordInternal,
		StopContainers:  s.stopRunningContainersForBackupInternal,
		StartContainers: s.startContainersAfterBackupInternal,
		SafetyBackup:    s.safetyBackupInternal,
		ArchivePaths:    s.browser.ArchivePaths,
		ArchiveFilename: backupArchiveFilenameInternal,
		HelperImage:     deps.HelperImage,
		StorageMount:    s.StorageMount,
		AcquireHelper:   deps.AcquireHelper,
		Exec:            deps.Exec,
	})
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

// HasEnabledBackupPolicy reports whether a volume-level schedule takes precedence over centralized backups.
func (s *Service) HasEnabledBackupPolicy(ctx context.Context, volumeName string) (bool, error) {
	return s.policies.HasEnabledBackupPolicy(ctx, volumeName)
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
		PolicyID: entry.PolicyID, Error: entry.Error, Type: entry.Type,
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

// backupPlanInternal selects the local repository for one backup run.
type backupPlanInternal struct {
	policy       *policies.VolumeBackupPolicy
	localEnabled bool
	destination  volume.BackupDestination
}

func (s *Service) resolveBackupPlanInternal(ctx context.Context, volumeName, trigger string, request volume.CreateBackupRequest, suppliedPolicy *policies.VolumeBackupPolicy) (backupPlanInternal, error) {
	if request.Destination != "" && request.Destination != volume.BackupDestinationLocal {
		return backupPlanInternal{}, errors.New("invalid volume backup destination")
	}
	policy := suppliedPolicy
	if trigger != backuptypes.VolumeBackupTriggerSafety && policy == nil {
		var err error
		policy, err = s.policies.Policy(ctx, volumeName, request.PolicyID)
		if err != nil {
			return backupPlanInternal{}, err
		}
		if request.PolicyID != "" && policy == nil {
			return backupPlanInternal{}, errors.New("volume backup policy not found")
		}
	}
	if policy != nil && !policy.LocalEnabled && trigger != backuptypes.VolumeBackupTriggerSafety {
		return backupPlanInternal{}, errors.New("local volume backups are disabled by policy")
	}
	return backupPlanInternal{policy: policy, localEnabled: true, destination: volume.BackupDestinationLocal}, nil
}

func (s *Service) CreateBackup(ctx context.Context, volumeName string, user usertypes.Actor, trigger string, request volume.CreateBackupRequest) (_ *volume.Backup, err error) {
	trigger = cmp.Or(trigger, backuptypes.VolumeBackupTriggerManual)
	plan, err := s.resolveBackupPlanInternal(ctx, volumeName, trigger, request, nil)
	if err != nil {
		return nil, err
	}
	return s.createBackupInternal(ctx, volumeName, user, trigger, request.PolicyID, plan)
}

// safetyBackupInternal takes the local pre-restore backup for a caller that already holds the volume's workspace lock.
func (s *Service) safetyBackupInternal(ctx context.Context, volumeName string, user usertypes.Actor) (string, error) {
	ctx = context.WithValue(ctx, lockHeldKeyInternal{}, lockHeldInternal{service: s, volumeName: volumeName})
	entry, err := s.CreateBackup(ctx, volumeName, user, backuptypes.VolumeBackupTriggerSafety, volume.CreateBackupRequest{Destination: volume.BackupDestinationLocal})
	if err != nil {
		return "", err
	}
	return entry.ID, nil
}

// CreateSystemManagedBackup runs the existing volume backup workflow with a transient centralized policy.
func (
	s *Service,
) CreateSystemManagedBackup(
	ctx context.Context,
	volumeName string,
	user usertypes.Actor,
	trigger string,
	policyID string,
	policy backuptypes.UpdateBackupPolicy,
) (
	*volume.Backup,
	error,
) {
	if !strings.HasPrefix(policyID, backuptypes.SystemVolumePolicyPrefix) {
		return nil, errors.New("invalid system-managed volume backup policy id")
	}
	trigger = cmp.Or(trigger, backuptypes.VolumeBackupTriggerManual)
	transient := &policies.VolumeBackupPolicy{
		VolumeName: volumeName, Enabled: true, Schedule: policy.Schedule, RetentionCount: policy.RetentionCount,
		StopContainers: policy.StopContainers, LocalEnabled: policy.LocalEnabled,
	}
	transient.ID = policyID
	plan, err := s.resolveBackupPlanInternal(ctx, volumeName, trigger, volume.CreateBackupRequest{}, transient)
	if err != nil {
		return nil, err
	}
	return s.createBackupInternal(ctx, volumeName, user, trigger, policyID, plan)
}

func (s *Service) createBackupInternal(ctx context.Context, volumeName string, user usertypes.Actor, trigger, policyID string, plan backupPlanInternal) (*volume.Backup, error) {
	entry, lease, err := s.prepareBackupInternal(ctx, volumeName, trigger, policyID, plan)
	if err != nil {
		return nil, err
	}
	defer lease.Release(ctx)
	err = s.executeBackupInternal(ctx, entry, user, plan)
	err = s.completeBackupInternal(ctx, entry, err)
	return entry, err
}

type volumeBackupRecoveryInternal struct {
	BackupID     string                       `json:"backupId"`
	LocalEnabled bool                         `json:"localEnabled"`
	Policy       *policies.VolumeBackupPolicy `json:"policy,omitempty"`
}

func (s *Service) prepareBackupInternal(ctx context.Context, volumeName, trigger, policyID string, plan backupPlanInternal) (*volume.Backup, *runs.Lease, error) {
	lease, admitted, err := s.deps.Engine.TryAcquireRun(ctx, backup.VolumeAdmissionScope, volumeName)
	if err != nil {
		return nil, nil, err
	}
	if !admitted {
		return nil, nil, s.deps.AlreadyRunning
	}
	entry := &volume.Backup{
		VolumeName: volumeName, CreatedAt: time.Now(), Status: backuptypes.VolumeBackupStatusRunning,
		Trigger: trigger, Destination: plan.destination, Format: volume.BackupFormatRustic,
		PolicyID: policyID,
		Type:     backuptypes.ManagementTypeForPolicy(policyID),
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
		entry.Status, entry.Error = backuptypes.VolumeBackupStatusFailed, err.Error()
	} else {
		entry.Status, entry.Error = backuptypes.VolumeBackupStatusSucceeded, ""
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
	lockHeld, _ := ctx.Value(lockHeldKeyInternal{}).(lockHeldInternal)
	if lockHeld.service != s || lockHeld.volumeName != volumeName {
		defer s.deps.Locks.Lock(volumeName)()
	}
	if cancellationErr := ctx.Err(); cancellationErr != nil {
		return cancellationErr
	}
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return err
	}
	var stopped []container.Summary
	containersStopped := false
	if plan.policy != nil && plan.policy.StopContainers {
		stopped, err = s.stopRunningContainersForBackupInternal(ctx, dockerClient, volumeName, user, false)
		containersStopped = len(stopped) > 0
		defer func() {
			if containersStopped {
				_, restartErr := s.startContainersAfterBackupInternal(context.WithoutCancel(ctx), dockerClient, stopped, user)
				err = errors.Join(err, restartErr)
			}
		}()
		if err != nil {
			return err
		}
	}
	if plan.localEnabled && entry.LocalSnapshotID == "" {
		if createLocalBackupSnapshotErr := s.createLocalBackupSnapshotInternal(ctx, dockerClient, entry); createLocalBackupSnapshotErr != nil {
			return createLocalBackupSnapshotErr
		}
	}
	if containersStopped {
		stopped, err = s.startContainersAfterBackupInternal(context.WithoutCancel(ctx), dockerClient, stopped, user)
		containersStopped = len(stopped) > 0
		if err != nil {
			return err
		}
	}
	// Retention failures must not fail the run: the backup itself succeeded,
	// and the deferred status-save would otherwise mark it failed and hide it
	// from ExpiredRunIDs forever.
	if trigger != backuptypes.VolumeBackupTriggerSafety && plan.policy != nil && plan.policy.RetentionCount > 0 {
		if retentionErr := s.applyVolumeBackupRetentionInternal(ctx, plan.policy.ID, plan.policy.RetentionCount); retentionErr != nil {
			slog.ErrorContext(ctx, "Volume backup retention failed", "volume", volumeName, "policy", plan.policy.ID, "error", retentionErr)
		}
	}
	metadata := database.JSON{"action": "backup_create", "backup_id": entry.ID, "size": entry.Size, "destination": entry.Destination}
	if logErr := s.deps.Events.LogVolumeEvent(ctx, event.EventTypeVolumeBackupCreate, volumeName, volumeName, user.ID, user.Username, "0", metadata); logErr != nil {
		slog.WarnContext(ctx, "could not log volume backup create event", "volume", volumeName, "error", logErr)
	}
	return ctx.Err()
}

func (s *Service) createLocalBackupSnapshotInternal(ctx context.Context, dockerClient *client.Client, entry *volume.Backup) error {
	repository, repoErr := s.localRusticRepositoryInternal(ctx, dockerClient, false)
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
	lease, admitted, err := s.deps.Engine.TryAcquireRun(ctx, backup.VolumeAdmissionScope, entry.VolumeName)
	if err != nil {
		return err
	}
	if !admitted {
		return s.deps.AlreadyRunning
	}
	defer lease.Release(ctx)
	return s.deleteBackupInternal(ctx, backupID, user)
}

func (s *Service) deleteBackupInternal(ctx context.Context, backupID string, user *usertypes.Actor) error {
	entry, err := s.deps.Store.Run(ctx, backupID)
	if err != nil {
		return err
	}
	if entry.Format == volume.BackupFormatArchive {
		return s.deleteArchiveBackupInternal(ctx, entry, user)
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
	if len(entries) == 0 {
		return nil
	}
	snapshotIDs := make([]string, len(entries))
	for index, entry := range entries {
		snapshotIDs[index] = entry.LocalSnapshotID
	}
	repository, repoErr := s.localRusticRepositoryInternal(ctx, dockerClient, false)
	if repoErr == nil {
		repoErr = s.forgetSnapshotsInternal(ctx, dockerClient, repository, snapshotIDs)
	}
	if repoErr != nil {
		return fmt.Errorf("failed to delete local Rustic snapshots: %w", repoErr)
	}
	for _, entry := range entries {
		entry.LocalSnapshotID = ""
	}
	return nil
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

// DownloadBackup streams a backup as a tar.gz archive. Legacy archive rows
// stream their stored file; local Rustic rows are restored into a scratch
// volume and packaged on the fly.
func (s *Service) DownloadBackup(ctx context.Context, backupID string, user *usertypes.Actor) (io.ReadCloser, int64, error) {
	entry, err := s.deps.Store.Run(ctx, backupID)
	if err != nil {
		return nil, 0, err
	}
	var reader io.ReadCloser
	var size int64
	if entry.Format == volume.BackupFormatArchive {
		reader, size, err = s.downloadArchiveBackupInternal(ctx, backupID)
	} else {
		reader, size, err = s.downloadRusticBackupInternal(ctx, entry)
	}
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
	repository, err := s.localRusticRepositoryInternal(ctx, dockerClient, true)
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

func (s *Service) downloadArchiveBackupInternal(ctx context.Context, backupID string) (io.ReadCloser, int64, error) {
	filename, err := backupArchiveFilenameInternal(backupID)
	if err != nil {
		return nil, 0, err
	}
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return nil, 0, err
	}
	containerID, cleanup, err := s.createBackupTempContainerInternal(ctx, dockerClient, "/volume", true)
	if err != nil {
		return nil, 0, err
	}
	return volumehelper.DownloadFileFromContainer(ctx, dockerClient, containerID, path.Join("/volume", filename), cleanup)
}

func (s *Service) deleteArchiveBackupInternal(ctx context.Context, entry *volume.Backup, user *usertypes.Actor) error {
	// Delete from DB first - if this fails, no changes are made. If file
	// deletion fails afterward we just have an orphan file, easier to clean up
	// than an orphan DB record pointing at a missing file.
	volumeName := entry.VolumeName
	backupID := entry.ID
	if err := s.deps.Store.Delete(ctx, backupID); err != nil {
		return err
	}
	containerID, cleanup, err := s.createBackupTempContainerInternal(ctx, nil, "/volume", false)
	if err != nil {
		slog.WarnContext(ctx, "failed to create container for backup file cleanup", "backupId", backupID, "error", err.Error())
	} else {
		defer cleanup()
		filename, filenameErr := backupArchiveFilenameInternal(backupID)
		if filenameErr != nil {
			slog.WarnContext(ctx, "failed to sanitize backup id for file cleanup", "backupId", backupID, "error", filenameErr.Error())
		} else if _, _, err = s.deps.Exec(ctx, containerID, "", []string{"rm", "-f", path.Join("/volume", filename)}); err != nil {
			slog.WarnContext(ctx, "failed to delete backup file (orphan file may remain)", "backupId", backupID, "error", err.Error())
		}
	}
	s.logBackupDeleteEventInternal(ctx, volumeName, backupID, user)
	return nil
}

// ArchiveFileTarget resolves a legacy archive backup of volumeName and the
// sanitized member path for a single-file workspace restore.
func (s *Service) ArchiveFileTarget(ctx context.Context, volumeName, backupID, rel string) (string, string, error) {
	filename, err := backupArchiveFilenameInternal(backupID)
	if err != nil {
		return "", "", common.Classify(common.ErrVolumeWorkspaceNotFound, err)
	}
	entry, err := s.deps.Store.Run(ctx, backupID)
	if err != nil {
		return "", "", common.Classify(common.ErrVolumeWorkspaceNotFound, err)
	}
	if entry.VolumeName != volumeName {
		return "", "", common.Classify(common.ErrVolumeWorkspaceForbidden, errors.New("backup does not belong to volume"))
	}
	cleaned, err := sanitizeBackupPathInternal(rel)
	if err != nil {
		return "", "", common.Classify(common.ErrVolumeWorkspaceBadRequest, err)
	}
	return filename, cleaned, nil
}

// RestoreArchiveMembers extracts members of a legacy archive into a helper that mounts backup storage at /backups.
func (s *Service) RestoreArchiveMembers(ctx context.Context, containerID, filename string, cleanedPaths []string) (string, error) {
	return s.restore.RestoreArchiveMembers(ctx, containerID, filename, cleanedPaths)
}

func (s *Service) applyVolumeBackupRetentionInternal(ctx context.Context, policyID string, retentionCount int) error {
	expired, err := backup.ExpiredRunIDs(ctx, s.deps.DB, "volume_backups", policyID, retentionCount)
	if err != nil || len(expired) == 0 {
		return err
	}
	entries, err := s.deps.Store.Runs(ctx, expired)
	if err != nil {
		return err
	}
	return s.deleteRusticBackupsInternal(ctx, entries, nil)
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

// StartBackup persists a running backup and submits application-owned work.
func (s *Service) StartBackup(ctx context.Context, environmentID, volumeName string, user usertypes.Actor, request volume.CreateBackupRequest) (volume.BackupEntry, error) {
	plan, err := s.resolveBackupPlanInternal(ctx, volumeName, backuptypes.VolumeBackupTriggerManual, request, nil)
	if err != nil {
		return volume.BackupEntry{}, err
	}
	entry, lease, err := s.prepareBackupInternal(ctx, volumeName, backuptypes.VolumeBackupTriggerManual, request.PolicyID, plan)
	if err != nil {
		return volume.BackupEntry{}, err
	}
	activityID, workCtx := activitylib.StartHandlerActivity(ctx, s.deps.Activity, environmentID, activitytypes.TypeResourceAction,
		"volume", volumeName, volumeName, &user, "Creating backup", "Creating volume backup",
		database.JSON{"action": "create_volume_backup", "backupId": entry.ID, "destination": entry.Destination, "policyId": request.PolicyID}, false)
	if activityID == "" {
		defer lease.Release(ctx)
		return volume.BackupEntry{}, s.completeBackupInternal(ctx, entry, errors.New("failed to start backup activity"))
	}
	entry.ActivityID = &activityID
	accepted := entryInternal(entry)
	keyID, _ := ctx.Value(middleware.ContextKeyApiKeyID).(string)
	payload, err := json.Marshal(
		manualVolumeBackupInternal{
			Checkpoint: volumeBackupRecoveryInternal{
				BackupID:     entry.ID,
				LocalEnabled: plan.localEnabled,
				Policy:       plan.policy,
			},
			VolumeName: volumeName,
			UserID:     user.ID,
			ActivityID: activityID,
		},
	)
	if err == nil {
		err = s.deps.Engine.SubmitDurableRun(
			workCtx,
			backuptypes.DurableRunCommand{
				Kind:             "volume",
				RunID:            entry.ID,
				ActivityID:       activityID,
				Payload:          payload,
				UserID:           user.ID,
				EnvironmentID:    environmentID,
				Permission:       authz.PermVolumesBackup,
				RequestedWithKey: keyID,
			},
			lease,
		)
	}
	if err != nil {
		lease.Release(ctx)
		err = s.completeBackupInternal(workCtx, entry, err)
		activitylib.CompleteHandlerActivity(workCtx, s.deps.Activity, activityID, "Volume backup created successfully", err)
		return volume.BackupEntry{}, err
	}

	return accepted, nil
}

func (s *Service) ReconcileBackup(ctx context.Context, previous scheduler.Run, volumeName string) (scheduler.Outcome, error) {
	outcome := jobcontext.ConfirmedTarget(previous, volumeName)
	if outcome.Status == scheduler.Succeeded {
		return outcome, nil
	}
	for _, target := range previous.Outcome.Targets {
		if target.ID != volumeName || len(target.RecoveryData) == 0 {
			continue
		}
		var checkpoint volumeBackupRecoveryInternal
		if err := json.Unmarshal(target.RecoveryData, &checkpoint); err != nil {
			return outcome, err
		}
		entry, loadErr := s.deps.Store.Run(ctx, checkpoint.BackupID)
		if loadErr != nil {
			return outcome, loadErr
		}
		if entry.VolumeName != volumeName {
			return outcome, gorm.ErrRecordNotFound
		}
		if entry.Status != backuptypes.VolumeBackupStatusSucceeded {
			if checkpoint.Policy != nil && checkpoint.Policy.StopContainers {
				return outcome, nil
			}
			lease, admitted, err := s.deps.Engine.AcquireDurableRun(ctx, previous.ID, backup.VolumeAdmissionScope, volumeName)
			if err != nil {
				return outcome, err
			}
			if !admitted {
				return outcome, nil
			}
			defer lease.Release(ctx) //nolint:gocritic // The matching recovery target always returns before the loop advances.
			if resumeBackupErr := s.resumeBackupInternal(ctx, previous, entry, checkpoint); resumeBackupErr != nil {
				return outcome, resumeBackupErr
			}
		}
		target.Status = scheduler.Succeeded
		if err := jobcontext.Progress(ctx, target); err != nil {
			return outcome, err
		}
		return scheduler.Outcome{Status: scheduler.Succeeded, Targets: []scheduler.TargetOutcome{target}}, nil
	}
	return outcome, nil
}

func (
	s *Service,
) recoverSnapshotInternal(
	ctx context.Context,
	dockerClient *client.Client,
	repository backup.Repository,
	entry *volume.Backup,
	snapshotID string,
) (
	backup.Snapshot,
	bool,
	error,
) {
	password, err := s.volumeBackupPasswordInternal(ctx, dockerClient, repository)
	if err != nil {
		return backup.Snapshot{}, false, err
	}
	return s.deps.Engine.FindRunSnapshot(ctx, dockerClient, repository, password, entry.ID, snapshotID)
}

func (s *Service) observeBackupDestinationInternal(ctx context.Context, dockerClient *client.Client, previous scheduler.Run, entry *volume.Backup) (backup.Repository, error) {
	local, err := s.localRusticRepositoryInternal(ctx, dockerClient, false)
	if err != nil {
		return local, err
	}
	if entry.LocalSnapshotID != "" || backupDestinationAttemptedInternal(previous, entry.ID, "local") {
		snapshot, found, err := s.recoverSnapshotInternal(ctx, dockerClient, local, entry, entry.LocalSnapshotID)
		if err != nil {
			return local, err
		}
		if found {
			entry.LocalSnapshotID, entry.Size = snapshot.ID, snapshot.Size
		}
	}
	return local, nil
}

func (s *Service) resumeBackupInternal(ctx context.Context, previous scheduler.Run, entry *volume.Backup, checkpoint volumeBackupRecoveryInternal) error {
	dockerClient, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return err
	}
	_, err = s.observeBackupDestinationInternal(ctx, dockerClient, previous, entry)
	if err != nil {
		return err
	}
	if operationErr := s.deps.Store.Save(ctx, entry); operationErr != nil {
		return operationErr
	}
	plan := backupPlanInternal{
		localEnabled: checkpoint.LocalEnabled,
		policy:       checkpoint.Policy,
		destination:  entry.Destination,
	}
	err = s.executeBackupInternal(ctx, entry, usertypes.SystemUser, plan)
	return s.completeBackupInternal(ctx, entry, err)
}

type manualVolumeBackupInternal struct {
	Checkpoint volumeBackupRecoveryInternal `json:"checkpoint"`
	VolumeName string                       `json:"volumeName"`
	UserID     string                       `json:"userId"`
	ActivityID string                       `json:"activityId"`
}

func (s *Service) executeDurableBackupInternal(ctx context.Context, runID string, payload []byte, interrupted bool) (err error) {
	var command manualVolumeBackupInternal
	if decodeErr := json.Unmarshal(payload, &command); decodeErr != nil {
		return decodeErr
	}
	defer func() {
		if ctx.Err() == nil {
			if err != nil {
				saveErr := s.deps.Store.Fail(ctx, command.Checkpoint.BackupID, err.Error())
				err = errors.Join(err, saveErr)
			}
			activitylib.CompleteHandlerActivity(ctx, s.deps.Activity, command.ActivityID, "Volume backup created successfully", err)
		}
	}()
	entry, loadBackupErr := s.deps.Store.Run(ctx, command.Checkpoint.BackupID)
	if loadBackupErr != nil {
		return loadBackupErr
	}

	if entry.Status == backuptypes.VolumeBackupStatusSucceeded {
		return nil
	}
	if interrupted {
		previous, _ := jobcontext.Run(ctx)
		outcome, recoveryErr := s.ReconcileBackup(ctx, previous, command.VolumeName)
		if recoveryErr != nil {
			return recoveryErr
		}
		if outcome.Status != scheduler.Succeeded {
			return errors.New(outcome.Message)
		}
		return nil
	}
	lease, admitted, err := s.deps.Engine.AcquireDurableRun(ctx, runID, backup.VolumeAdmissionScope, command.VolumeName)
	if err != nil {
		return err
	}
	if !admitted {
		return s.deps.AlreadyRunning
	}
	defer lease.Release(ctx)
	actor := usertypes.SystemUser
	if command.UserID != "" && command.UserID != "agent" {
		var localUser userdomain.User
		if loadUserErr := s.deps.DB.WithContext(ctx).Where("id = ?", command.UserID).First(&localUser).Error; loadUserErr != nil {
			return loadUserErr
		}
		actor = *localUser.Actor()
	}
	checkpoint, err := json.Marshal(command.Checkpoint)
	if err != nil {
		return err
	}
	if progressErr := jobcontext.Progress(
		ctx,
		scheduler.TargetOutcome{
			ResourceType: "volume_backup",
			ID:           command.VolumeName,
			Status:       scheduler.Running,
			RecoveryData: checkpoint,
			ActivityID:   command.ActivityID,
		},
	); progressErr != nil {
		return progressErr
	}
	plan := backupPlanInternal{
		localEnabled: command.Checkpoint.LocalEnabled,
		policy:       command.Checkpoint.Policy,
		destination:  entry.Destination,
	}
	err = s.executeBackupInternal(ctx, entry, actor, plan)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return s.completeBackupInternal(ctx, entry, err)
}

func (s *Service) failDurableBackupInternal(ctx context.Context, _ string, payload []byte, runErr error) error {
	var command manualVolumeBackupInternal
	if err := json.Unmarshal(payload, &command); err != nil {
		return err
	}
	entry, err := s.deps.Store.Run(ctx, command.Checkpoint.BackupID)
	if err != nil {
		return err
	}
	err = s.deps.Store.Fail(ctx, entry.ID, runErr.Error())
	activitylib.CompleteHandlerActivity(ctx, s.deps.Activity, command.ActivityID, "Volume backup created successfully", runErr)
	return err
}
