// Source: getarcaneapp/arcane/backend/internal/volume/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; split inventory reads; Explicit candidate prune execution; M12 backup foundation; M12 persisted volume lifecycle and legacy restore cut.

package volume

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	containerdomain "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	volumebackup "github.com/florianfinn/docklet-next/backend/internal/volume/children/backup"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/workspace"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	workspacepkg "github.com/florianfinn/docklet-next/backend/pkg/workspace"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
	"golang.org/x/sync/singleflight"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type VolumeService struct {
	db               *database.DB
	dockerService    *dockerInternal.DockerClientService
	eventService     *event.EventService
	settingsService  *settings.SettingsService
	imageService     *image.ImageService
	backupVolumeName string
	workspaceLocks   utils.KeyedMutex
	helperMu         sync.Mutex
	helperByVolume   map[string]*volumeHelper
	// helperGroup deduplicates concurrent read-only helper creation per volume.
	// Without it two simultaneous browse requests each create a helper and the
	// second overwrites the first in helperByVolume, orphaning a `sleep infinity`
	// container that pins the volume until restart.
	helperGroup singleflight.Group
	backup      *volumebackup.Service
	workspace   *workspace.Service
}

var ErrVolumeBackupAlreadyRunning = errors.New("a backup is already running for this volume")

const internalVolumePruneFilterValue = libarcane.InternalResourceLabel + "=true"

func NewVolumeService(
	db *database.DB,
	dockerService *dockerInternal.DockerClientService,
	eventService *event.EventService,
	activityService *activity.ActivityService,
	settingsService *settings.SettingsService,
	containerService *containerdomain.ContainerService,
	imageService *image.ImageService,
	engine *backup.Engine,
	cfg *config.Config,
	recoveryKeys *backup.RecoveryKeyStore,
) *VolumeService {
	backupVolumeName := ""
	encryptionKey := ""
	workspaceMaxDepth := 50
	workspaceMaxEntries := 10000
	workspaceMaxFileSizeMB := workspacepkg.DefaultMaxFileSizeMB
	if cfg != nil {
		backupVolumeName = cfg.BackupVolumeName
		encryptionKey = cfg.EncryptionKey
		workspaceMaxDepth = cfg.VolumeWorkspaceMaxDepth
		workspaceMaxEntries = cfg.VolumeWorkspaceMaxEntries
		workspaceMaxFileSizeMB = cfg.VolumeWorkspaceMaxFileSizeMB
	}
	if strings.TrimSpace(backupVolumeName) == "" {
		backupVolumeName = "arcane-backups"
	}
	service := &VolumeService{
		db:               db,
		dockerService:    dockerService,
		eventService:     eventService,
		settingsService:  settingsService,
		imageService:     imageService,
		backupVolumeName: backupVolumeName,
		helperByVolume:   make(map[string]*volumeHelper),
	}
	backupDeps := volumebackup.Dependencies{
		Store:            service.backupStoreInternal(),
		DB:               db,
		Docker:           dockerService,
		Events:           eventService,
		Activity:         activityService,
		Engine:           engine,
		RecoveryKeys:     recoveryKeys,
		Locks:            &service.workspaceLocks,
		AlreadyRunning:   ErrVolumeBackupAlreadyRunning,
		HelperImage:      service.getVolumeHelperImageInternal,
		AcquireHelper:    service.acquireVolumeHelperInternal,
		Exec:             service.execInContainerInternal,
		BackupVolumeName: backupVolumeName,
		EncryptionKey:    encryptionKey,
		Location: func() *time.Location {
			if cfg != nil {
				return cfg.GetLocation()
			}
			return time.UTC
		}(),
	}
	if containerService != nil {
		backupDeps.StopContainer, backupDeps.StartContainer = containerService.StopContainer, containerService.StartContainer
	}
	service.backup = volumebackup.NewService(backupDeps)
	service.workspace = workspace.NewService(workspace.Dependencies{
		Docker:           dockerService,
		Events:           eventService,
		Locks:            &service.workspaceLocks,
		AcquireHelper:    service.acquireVolumeHelperInternal,
		RequireACFS:      service.requireVolumeHelperACFSInternal,
		Exec:             service.execInContainerInternal,
		HelperImage:      service.getVolumeHelperImageInternal,
		BusyError:        func(name string) error { return &volumeWorkspaceBusyError{name: name} },
		MaxDepth:         workspaceMaxDepth,
		MaxEntries:       workspaceMaxEntries,
		MaxFileSizeBytes: workspacepkg.MaxFileSizeBytes(workspaceMaxFileSizeMB),
	})
	return service
}

// SetScheduler injects the dynamic scheduler and admission gate for per-policy
// backup jobs. Agent mode passes them too: agents run their own volume backups.
func (s *VolumeService) SetScheduler(ctx context.Context, dynamicScheduler scheduler.DynamicScheduler, admissionGate *runs.Admission) error {
	return s.backup.SetScheduler(ctx, dynamicScheduler, admissionGate)
}

func (s *VolumeService) RegisterBackupJobsOnStartup(ctx context.Context) {
	s.backup.RegisterJobsOnStartup(ctx)
}

// BackupStorageMount resolves the repository mount shared with system-backup operations.
func (s *VolumeService) BackupStorageMount(ctx context.Context, dockerClient *client.Client, target string, readOnly bool) (mount.Mount, error) {
	return s.backup.StorageMount(ctx, dockerClient, target, readOnly)
}

// HasEnabledBackupPolicy reports whether a volume-level schedule takes precedence over centralized backups.
func (s *VolumeService) HasEnabledBackupPolicy(ctx context.Context, volumeName string) (bool, error) {
	return s.backup.HasEnabledBackupPolicy(ctx, volumeName)
}

// CreateSystemManagedBackup runs the existing volume backup workflow with a transient centralized policy.
func (
	s *VolumeService,
) CreateSystemManagedBackup(
	ctx context.Context,
	volumeName string,
	user usertypes.Actor,
	trigger VolumeBackupTrigger,
	policyID string,
	policy backuptypes.UpdateBackupPolicy,
) (
	*VolumeBackup,
	error,
) {
	entry, err := s.backup.CreateSystemManagedBackup(ctx, volumeName, user, string(trigger), policyID, policy)
	if entry == nil {
		return nil, err
	}
	return volumeBackupFromRecordInternal(entry), err
}

func (s *VolumeService) ReconcileBackup(ctx context.Context, previous scheduler.Run, volumeName string) (scheduler.Outcome, error) {
	return s.backup.ReconcileBackup(ctx, previous, volumeName)
}

// MigrateRepositoryPasswords re-keys this instance's own volume backup repositories to the recovery key; other instances re-key their own roots.
func (s *VolumeService) MigrateRepositoryPasswords(ctx context.Context) error {
	return s.backup.MigrateRepositoryPasswords(ctx)
}

func (s *VolumeService) GetVolumeByName(ctx context.Context, name string) (*volumetypes.Volume, error) {
	slog.DebugContext(ctx, "volume service: get volume", "volume", name)
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	volResult, err := dockerClient.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("volume not found: %w", err)
	}
	vol := volResult.Volume

	localSettings := s.settingsService.GetSettingsConfig()
	usageCtx, usageCancel := context.WithTimeout(ctx, timeouts.GetDuration(localSettings.DockerAPITimeout.AsInt(), timeouts.DefaultDockerAPI))
	defer usageCancel()
	if usageVolumes, ok := docker.GetVolumeUsageDataStaleWhileRevalidate(usageCtx, dockerClient).Get(); ok {
		for _, uv := range usageVolumes {
			if uv.Name == vol.Name && uv.UsageData != nil {
				vol.UsageData = uv.UsageData
				slog.DebugContext(ctx, "attached volume usage data", "volume", vol.Name, "sizeBytes", uv.UsageData.Size, "refCount", uv.UsageData.RefCount)
				break
			}
		}
	}

	consumers, err := listVolumeConsumersInternal(ctx, dockerClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get containers using volume: %w", err)
	}
	v := volumeSummaryWithConsumersInternal(vol, consumers[name])
	return &v, nil
}

func (s *VolumeService) CreateVolume(ctx context.Context, options client.VolumeCreateOptions, user usertypes.Actor) (*volumetypes.Volume, error) {
	slog.DebugContext(ctx, "volume service: create volume", "volume", options.Name, "driver", options.Driver, "user", user.ID)
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeVolumeError, "volume", "", options.Name, user.ID, user.Username, "0", err, database.JSON{"action": "create", "driver": options.Driver})
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	defer s.eventService.BeginDockerResourceSuppressionWindow("volume", options.Name, options.Name)()
	created, err := dockerClient.VolumeCreate(ctx, options)
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeVolumeError, "volume", "", options.Name, user.ID, user.Username, "0", err, database.JSON{"action": "create", "driver": options.Driver})
		return nil, fmt.Errorf("failed to create volume: %w", err)
	}

	defer s.eventService.BeginDockerResourceSuppressionWindow("volume", created.Volume.Name, created.Volume.Name)()
	vol, err := dockerClient.VolumeInspect(ctx, created.Volume.Name, client.VolumeInspectOptions{})
	if err != nil {
		s.eventService.LogErrorEvent(
			ctx,
			event.EventTypeVolumeError,
			"volume",
			created.Volume.Name,
			created.Volume.Name,
			user.ID,
			user.Username,
			"0",
			err,
			database.JSON{
				"action": "create",
				"driver": options.Driver,
				"step":   "inspect",
			},
		)
		return nil, fmt.Errorf("failed to inspect created volume: %w", err)
	}

	metadata := database.JSON{
		"action": "create",
		"driver": vol.Volume.Driver,
		"name":   vol.Volume.Name,
	}
	if logErr := s.eventService.LogVolumeEvent(ctx, event.EventTypeVolumeCreate, vol.Volume.Name, vol.Volume.Name, user.ID, user.Username, "0", metadata); logErr != nil {
		slog.WarnContext(ctx, "could not log volume creation action", "volume", vol.Volume.Name, "error", logErr.Error())
	}

	docker.InvalidateVolumeUsageCache(dockerClient)

	return new(volumetypes.NewSummary(vol.Volume)), nil
}

func (s *VolumeService) DeleteVolume(ctx context.Context, name string, force bool, user usertypes.Actor) error {
	slog.DebugContext(ctx, "volume service: delete volume", "volume", name, "force", force, "user", user.ID)
	unlock, acquired := s.workspaceLocks.TryLock(name)
	if !acquired {
		return &volumeWorkspaceBusyError{name: name}
	}
	defer unlock()
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeVolumeError, "volume", name, name, user.ID, user.Username, "0", err, database.JSON{"action": "delete", "force": force})
		return fmt.Errorf("failed to connect to Docker: %w", err)
	}

	// Stop any read-only browse helper first; a helper mounting the volume would
	// otherwise block a non-forced VolumeRemove with "volume is in use".
	if stopErr := s.StopHelper(ctx, name); stopErr != nil {
		slog.WarnContext(ctx, "could not stop volume browse helper before delete", "volume", name, "error", stopErr.Error())
	}

	defer s.eventService.BeginDockerResourceSuppressionWindow("volume", name, name)()
	if _, volumeRemoveErr := dockerClient.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: force}); volumeRemoveErr != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeVolumeError, "volume", name, name, user.ID, user.Username, "0", volumeRemoveErr, database.JSON{"action": "delete", "force": force})
		return fmt.Errorf("failed to remove volume: %w", volumeRemoveErr)
	}

	metadata := database.JSON{"action": "delete", "name": name}
	if logErr := s.eventService.LogVolumeEvent(ctx, event.EventTypeVolumeDelete, name, name, user.ID, user.Username, "0", metadata); logErr != nil {
		slog.WarnContext(ctx, "could not log volume deletion action", "volume", name, "error", logErr.Error())
	}

	s.removeHelperEntry(name)
	s.backup.RemovePolicies(ctx, name)
	docker.InvalidateVolumeUsageCache(dockerClient)
	return nil
}

// PruneVolumesWithOptions preserves the internal system/scheduler API while
// executing only a freshly observed, explicitly re-checked candidate list.
func (s *VolumeService) PruneVolumesWithOptions(ctx context.Context, all bool) (*volumetypes.PruneReport, error) {
	plan, err := s.PlanVolumePrune(ctx, all)
	if err != nil {
		return nil, err
	}
	candidates := make([]volumetypes.PruneCandidate, 0, len(plan.Candidates))
	for _, candidate := range plan.Candidates {
		candidates = append(candidates, volumetypes.PruneCandidate{Name: candidate.Name, CreatedAt: candidate.CreatedAt})
	}
	result, err := s.executeVolumePrune(ctx, candidates, all)
	if err != nil {
		return nil, err
	}
	report := &volumetypes.PruneReport{VolumesDeleted: result.Removed, SpaceReclaimed: result.SpaceReclaimed}
	if len(result.Failed) > 0 {
		return report, fmt.Errorf("volume prune failed for %d candidates", len(result.Failed))
	}
	return report, nil
}

func buildVolumePruneMetadataInternal(all bool, volumesDeleted int, spaceReclaimed uint64) database.JSON {
	return database.JSON{
		"action":                    "prune",
		"all":                       all,
		"volumesDeleted":            volumesDeleted,
		"spaceReclaimed":            spaceReclaimed,
		"internalVolumeFilterLabel": internalVolumePruneFilterValue,
	}
}

func (s *VolumeService) GetVolumeUsage(ctx context.Context, name string) (bool, []string, error) {
	slog.DebugContext(ctx, "volume service: get volume usage", "volume", name)
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	vol, err := dockerClient.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		return false, nil, fmt.Errorf("volume not found: %w", err)
	}

	consumers, err := listVolumeConsumersInternal(ctx, dockerClient)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get containers using volume: %w", err)
	}

	containerIDs := consumers[vol.Volume.Name]
	return len(containerIDs) > 0, containerIDs, nil
}

// VolumeSizeData holds size information for a volume.
type VolumeSizeData struct {
	Size     int64
	RefCount int64
}

// GetVolumeSizes returns disk usage data for all volumes.
// This is a slow operation as it calls Docker's DiskUsage API.
func (s *VolumeService) GetVolumeSizes(ctx context.Context) (map[string]VolumeSizeData, error) {
	slog.DebugContext(ctx, "volume service: get volume sizes")
	localSettings := s.settingsService.GetSettingsConfig()
	apiCtx, cancel := context.WithTimeout(ctx, timeouts.GetDuration(localSettings.DockerAPITimeout.AsInt(), timeouts.DefaultDockerAPI))
	defer cancel()

	dockerClient, err := s.dockerService.GetClient(apiCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	usageVolumes, err := docker.GetVolumeUsageData(apiCtx, dockerClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume usage data: %w", err)
	}

	consumers, err := listVolumeConsumersInternal(apiCtx, dockerClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get containers using volumes: %w", err)
	}
	result := make(map[string]VolumeSizeData, len(usageVolumes))
	for _, v := range usageVolumes {
		if v.UsageData != nil {
			result[v.Name] = VolumeSizeData{
				Size:     v.UsageData.Size,
				RefCount: int64(len(consumers[v.Name])),
			}
		}
	}

	return result, nil
}
