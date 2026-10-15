// Source: getarcaneapp/arcane/backend/internal/system/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; functional cut: removed SQLite runtime and migrations.

package system

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	containertypes "github.com/florianfinn/docklet-next/types/container"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/system"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	mobycontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"github.com/samber/hot"
	"github.com/samber/mo"
	"go.getarcane.app/docker"
	"go.getarcane.app/docker/compat"
	"go.getarcane.app/sys/cgroup"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/labels"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/network"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	systembackup "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	"github.com/florianfinn/docklet-next/backend/internal/system/children/prune"
	"github.com/florianfinn/docklet-next/backend/internal/system/children/upgrade"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/vuln"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
)

// SystemService owns Docker system-wide operations and coordinates the prune
// and self-upgrade features.
type SystemService struct {
	db                    *database.DB
	dockerService         *dockerInternal.DockerClientService
	containerService      *container.ContainerService
	settingsService       *settings.SettingsService
	activityService       *activity.ActivityService
	dockerHostMemoryCache *hot.HotCache[struct{}, dockerHostMemoryInfo]

	prune   *prune.Service
	upgrade *upgrade.Service
	backup  *systembackup.Service
}

func NewSystemService(
	db *database.DB,
	dockerService *dockerInternal.DockerClientService,
	containerService *container.ContainerService,
	imageUpdateService *imageupdate.ImageUpdateService,
	volumeService *volume.VolumeService,
	networkService *network.NetworkService,
	settingsService *settings.SettingsService,
	activityService *activity.ActivityService,
	versionService *version.VersionService,
	eventService *event.EventService,
	projectService *project.ProjectService,
	backupEngine *backup.Engine,
	recoveryKeys *backup.RecoveryKeyStore,
	cfg *config.Config,
) *SystemService {
	location := time.UTC
	if cfg != nil {
		location = cfg.GetLocation()
	}
	s := &SystemService{
		db:               db,
		dockerService:    dockerService,
		containerService: containerService,
		settingsService:  settingsService,
		activityService:  activityService,
		dockerHostMemoryCache: hot.NewHotCache[struct{}, dockerHostMemoryInfo](hot.LRU, 1).
			WithTTL(dockerHostMemoryCacheTTL).
			Build(),
		prune:   prune.NewService(location, dockerService.GetClient, activityService, imageUpdateService, volumeService, networkService),
		upgrade: upgrade.NewService(db, dockerService, versionService, eventService, settingsService, projectService, resolveUpgraderRuntimeOptionsInternal),
	}
	s.backup = systembackup.NewService(systembackup.Dependencies{
		Store:        s.backupStoreInternal(),
		DB:           db,
		Docker:       dockerService,
		Volumes:      volumeService,
		Engine:       backupEngine,
		Activity:     activityService,
		Settings:     settingsService,
		Config:       cfg,
		RecoveryKeys: recoveryKeys,
	})
	return s
}

// ReconcileInterruptedBackups fails system backup runs left running by the previous process.
func (s *SystemService) ReconcileInterruptedBackups(ctx context.Context, protectedIDs ...string) error {
	query := s.db.WithContext(ctx).Model(&SystemBackupRun{}).Where("status = ?", SystemBackupStatusRunning)
	if len(protectedIDs) > 0 {
		query = query.Where("id NOT IN ?", protectedIDs)
	}
	return query.Updates(map[string]any{"status": SystemBackupStatusFailed, "error_code": backuptypes.SystemBackupErrorInterrupted}).Error
}

// SetBackupScheduler injects the dynamic scheduler and admission gate for
// per-policy system backup jobs. Agent mode leaves them unset.
func (s *SystemService) SetBackupScheduler(ctx context.Context, dynamicScheduler scheduler.DynamicScheduler, admissionGate *runs.Admission) error {
	return s.backup.SetScheduler(ctx, dynamicScheduler, admissionGate)
}

// RegisterBackupJobOnStartup schedules every saved system and system-managed volume backup policy.
func (s *SystemService) RegisterBackupJobOnStartup(ctx context.Context) {
	s.backup.RegisterBackupJobOnStartup(ctx)
}

// PruneScheduled runs the scheduled prune job.
func (s *SystemService) PruneScheduled(ctx context.Context, environmentID string, req system.PruneAllRequest) (*system.PruneAllResult, bool, error) {
	return s.prune.PruneScheduled(ctx, environmentID, req)
}

// ReconcileScheduledPrune recovers an interrupted scheduled prune.
func (s *SystemService) ReconcileScheduledPrune(ctx context.Context, previous scheduler.Run) (scheduler.Outcome, error) {
	return s.prune.ReconcileScheduledPrune(ctx, previous)
}

// TriggerUpgradeViaCLI starts the self-upgrade for the updater engine.
func (s *SystemService) TriggerUpgradeViaCLI(ctx context.Context, user usertypes.Actor, target updater.SelfUpdateTarget) (string, error) {
	return s.upgrade.TriggerUpgradeViaCLI(ctx, user, target)
}

// ResumeUpdateAllOnStartup finalizes an update-all job interrupted by the manager restart.
func (s *SystemService) ResumeUpdateAllOnStartup(ctx context.Context) {
	s.upgrade.ResumeUpdateAllOnStartup(ctx)
}

// PruneUpgradeLogs removes expired upgrade logs.
func (s *SystemService) PruneUpgradeLogs(ctx context.Context, dataDir string, now time.Time) (int, error) {
	return s.upgrade.PruneUpgradeLogs(ctx, dataDir, now)
}

func (
	s *SystemService,
) performBatchContainerAction(
	ctx context.Context,
	containers []mobycontainer.Summary,
	actionName string,
	shouldProcess func(
		mobycontainer.Summary,
	) bool,
	action func(
		context.Context,
		string,
	) error,
) *containertypes.ActionResult {
	result := &containertypes.ActionResult{Success: true}
	var mu sync.Mutex

	g, groupCtx := errgroup.WithContext(ctx)
	// Limit concurrency to avoid overwhelming Docker daemon
	g.SetLimit(5)

	for _, containerSummary := range containers {
		c := containerSummary // capture loop var
		if !shouldProcess(c) {
			continue
		}

		g.Go(func() (workerErr error) {
			defer utils.RecoverToError(&workerErr, "system info worker")

			err := action(groupCtx, c.ID)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.Failed = append(result.Failed, c.ID)
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to %s container %s: %v", actionName, c.ID, err))
				result.Success = false
			} else {
				if actionName == "start" {
					result.Started = append(result.Started, c.ID)
				} else {
					result.Stopped = append(result.Stopped, c.ID)
				}
			}
			return nil
		})
	}

	_ = g.Wait()
	return result
}

func (s *SystemService) StartAllContainers(ctx context.Context, environmentID string) (*containertypes.ActionResult, error) {
	return s.startMatchingContainersInternal(ctx, environmentID, startMatchingContainersOptionsInternal{
		ResourceName:   "All containers",
		StartMessage:   "Starting all containers",
		FailureMessage: "Starting all containers failed",
		SuccessMessage: "Started all containers",
		ShouldStart:    func(c mobycontainer.Summary) bool { return c.State != "running" },
	})
}

func (s *SystemService) StartAllStoppedContainers(ctx context.Context, environmentID string) (*containertypes.ActionResult, error) {
	return s.startMatchingContainersInternal(ctx, environmentID, startMatchingContainersOptionsInternal{
		ResourceName:   "Stopped containers",
		StartMessage:   "Starting stopped containers",
		FailureMessage: "Starting stopped containers failed",
		SuccessMessage: "Started stopped containers",
		ShouldStart:    func(c mobycontainer.Summary) bool { return c.State == "exited" },
	})
}

type startMatchingContainersOptionsInternal struct {
	ResourceName   string
	StartMessage   string
	FailureMessage string
	SuccessMessage string
	ShouldStart    func(mobycontainer.Summary) bool
}

func (s *SystemService) startMatchingContainersInternal(ctx context.Context, environmentID string, opts startMatchingContainersOptionsInternal) (*containertypes.ActionResult, error) {
	activityID := s.startSystemContainerActivityInternal(ctx, environmentID, activitytypes.TypeContainerStart, opts.ResourceName, opts.StartMessage)
	ctx = s.activityService.Track(ctx, activityID)
	containers, _, _, _, err := s.dockerService.GetAllContainers(ctx)
	if err != nil {
		result := &containertypes.ActionResult{
			Success:    false,
			Errors:     []string{fmt.Sprintf("Failed to list containers: %v", err)},
			ActivityID: mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer(),
		}
		s.completeSystemContainerActivityInternal(ctx, activityID, opts.FailureMessage, result)
		return result, err
	}

	result := s.performBatchContainerAction(ctx, containers, "start", opts.ShouldStart, func(ctx context.Context, id string) error {
		return s.containerService.StartContainer(ctx, id, usertypes.SystemUser)
	})
	result.ActivityID = mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer()
	s.completeSystemContainerActivityInternal(ctx, activityID, opts.SuccessMessage, result)
	return result, nil
}

func (s *SystemService) StopAllContainers(ctx context.Context, environmentID string) (*containertypes.ActionResult, error) {
	activityID := s.startSystemContainerActivityInternal(ctx, environmentID, activitytypes.TypeContainerStop, "All containers", "Stopping all containers")
	ctx = s.activityService.Track(ctx, activityID)
	containers, _, _, _, err := s.dockerService.GetAllContainers(ctx)
	if err != nil {
		result := &containertypes.ActionResult{
			Success:    false,
			Errors:     []string{fmt.Sprintf("Failed to list containers: %v", err)},
			ActivityID: mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer(),
		}
		s.completeSystemContainerActivityInternal(ctx, activityID, "Stopping all containers failed", result)
		return result, err
	}

	result := s.performBatchContainerAction(ctx, containers, "stop",
		func(c mobycontainer.Summary) bool {
			// Skip Arcane container
			return !labels.IsArcaneContainer(c.Labels)
		},
		func(ctx context.Context, id string) error {
			return s.containerService.StopContainer(ctx, id, usertypes.SystemUser)
		})
	result.ActivityID = mo.EmptyableToOption(strings.TrimSpace(activityID)).ToPointer()
	s.completeSystemContainerActivityInternal(ctx, activityID, "Stopped all containers", result)
	return result, nil
}

func (s *SystemService) startSystemContainerActivityInternal(ctx context.Context, environmentID string, activityType activitytypes.Type, resourceName, message string) string {
	if s.activityService == nil {
		return ""
	}
	localActivity, err := s.activityService.StartActivity(ctx, activity.StartActivityRequest{
		EnvironmentID: environmentID,
		Type:          activityType,
		ResourceType:  new("system"),
		ResourceName:  &resourceName,
		Step:          message,
		LatestMessage: message,
		Metadata:      database.JSON{"scope": resourceName},
	})
	if err != nil {
		slog.DebugContext(ctx, "failed to start system container activity", "type", activityType, "error", err)
		return ""
	}
	return localActivity.ID
}

func (s *SystemService) completeSystemContainerActivityInternal(ctx context.Context, activityID, successMessage string, result *containertypes.ActionResult) {
	if s.activityService == nil || activityID == "" || result == nil {
		return
	}

	status := activitytypes.StatusSuccess
	message := successMessage
	var errMessage *string
	if !result.Success || len(result.Errors) > 0 {
		status = activitytypes.StatusFailed
		message = strings.Join(result.Errors, "; ")
		errMessage = &message
	}

	if _, err := s.activityService.CompleteActivity(utils.ActivityRuntimeContext(ctx, nil), activityID, status, message, errMessage); err != nil {
		slog.DebugContext(ctx, "failed to complete system container activity", "activityId", activityID, "error", err)
	}
}

func (s *SystemService) GetDiskUsagePath(ctx context.Context) string {
	cfg := s.settingsService.GetSettingsConfig()
	if cfg == nil {
		return "/"
	}

	path := cmp.Or(cfg.DiskUsagePath.Value, "/")
	return path
}

const dockerHostMemoryCacheTTL = 30 * time.Second

type dockerHostMemoryInfo struct {
	root  string
	total uint64
}

// GetDockerHostMemory reports enclosing guest memory when Docker shares its
// host's cgroup namespace. A false result leaves the caller's baseline intact.
func (s *SystemService) GetDockerHostMemory(ctx context.Context) (uint64, uint64, bool) {
	if runtime.GOOS != "linux" || s.dockerService == nil || s.dockerHostMemoryCache == nil || !cgroup.IsDockerContainer() {
		return 0, 0, false
	}

	info, found, err := s.dockerHostMemoryCache.GetWithLoaders(struct{}{}, func(_ []struct{}) (map[struct{}]dockerHostMemoryInfo, error) {
		metadata, err := s.loadDockerHostMemoryInternal(ctx)
		if err != nil {
			slog.DebugContext(ctx, "Docker host memory accounting unavailable", "error", err)
			// Cache failures too, without retaining expired metadata.
			metadata = dockerHostMemoryInfo{}
		}
		return map[struct{}]dockerHostMemoryInfo{{}: metadata}, nil
	})
	if err != nil || !found || info.total == 0 || ctx.Err() != nil {
		return 0, 0, false
	}

	used, err := cgroup.MemoryUsage(info.root)
	if err != nil {
		return 0, 0, false
	}
	return min(used, info.total), info.total, true
}

func (s *SystemService) loadDockerHostMemoryInternal(ctx context.Context) (dockerHostMemoryInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	containerID, err := cgroup.CurrentContainerID()
	if err != nil {
		return dockerHostMemoryInfo{}, err
	}
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return dockerHostMemoryInfo{}, err
	}
	inspect, err := compat.ContainerInspectWithCompatibility(ctx, dockerClient, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return dockerHostMemoryInfo{}, err
	}
	if !strings.HasPrefix(inspect.Container.ID, containerID) || inspect.Container.HostConfig == nil ||
		inspect.Container.HostConfig.CgroupnsMode != "host" {
		return dockerHostMemoryInfo{}, errors.New("current container does not share the Docker host cgroup namespace")
	}
	// Inspection expands hostname and mountinfo short IDs before cgroup validation.
	root, err := cgroup.DockerHostRoot(inspect.Container.ID)
	if err != nil {
		return dockerHostMemoryInfo{}, err
	}

	info, err := dockerClient.Info(ctx, client.InfoOptions{})
	if err != nil {
		return dockerHostMemoryInfo{}, err
	}
	if info.Info.MemTotal <= 0 {
		return dockerHostMemoryInfo{}, errors.New("Docker host memory total is unavailable") //nolint:staticcheck // Preserve the existing error message.
	}
	return dockerHostMemoryInfo{root: root, total: uint64(info.Info.MemTotal)}, nil
}

// resolveUpgraderRuntimeOptionsInternal determines how a helper container reaches the Docker daemon.
func resolveUpgraderRuntimeOptionsInternal(
	ctx context.Context,
	dockerHost string,
	currentContainer *mobycontainer.InspectResponse,
	discoverHostPath func(context.Context, string) (string, error),
	isRunningInDocker func() bool,
	selectReachableNetwork func(context.Context, *mobycontainer.InspectResponse, string) string,
) ([]string, []mount.Mount, mobycontainer.NetworkMode, error) {
	containerEnv := vuln.BuildDockerHostEnv(dockerHost)

	scheme, socketPath, err := vuln.ParseDockerHost(dockerHost)
	if err != nil {
		return nil, nil, "", fmt.Errorf("resolve docker host %q: %w", dockerHost, err)
	}

	if scheme != "unix" {
		// The upgrader must reach the tcp DOCKER_HOST, so prefer a network the
		// daemon proxy is actually on over the plain auto heuristic (#3533).
		networkMode := docker.SelectAutoNetworkMode(currentContainer)
		if selectReachableNetwork != nil {
			networkMode = selectReachableNetwork(ctx, currentContainer, dockerHost)
		}
		return containerEnv, nil, mobycontainer.NetworkMode(networkMode), nil
	}

	socketSource, err := vuln.ResolveUnixSocketSource(
		ctx,
		socketPath,
		discoverHostPath,
		isRunningInDocker,
	)
	if err != nil {
		return nil, nil, "", fmt.Errorf("resolve unix socket source: %w", err)
	}

	mounts := []mount.Mount{{
		Type:   mount.TypeBind,
		Source: socketSource,
		Target: socketPath,
	}}

	return containerEnv, mounts, "", nil
}

const systemBackupHistoryUnion = "SELECT id, size, created_at, status, trigger, 'local' AS destination, 'pg_custom' AS format, '' AS local_snapshot_id, " +
	"policy_id, error_code AS error, 'system' AS type, 'system' AS resource_type, 'Hub database' AS resource_name " +
	"FROM system_backup_runs UNION ALL SELECT id, size, created_at, status, trigger, destination, format, " +
	"local_snapshot_id, policy_id, error, CASE WHEN policy_id LIKE 'system-volume:%' THEN 'system' ELSE " +
	"'volume' END AS type, 'volume' AS resource_type, volume_name AS resource_name FROM volume_backups"

// backupStoreInternal persists system backup runs and policies for the backup child.
func (s *SystemService) backupStoreInternal() systembackup.Store {
	return systembackup.Store{
		CreateRun: func(ctx context.Context, dto *backuptypes.SystemBackupRun) error {
			run := systemBackupRunFromDTOInternal(*dto)
			if err := s.db.WithContext(ctx).Create(&run).Error; err != nil {
				return err
			}
			dto.ID, dto.CreatedAt = run.ID, run.CreatedAt
			return nil
		},
		SaveRun: func(ctx context.Context, dto *backuptypes.SystemBackupRun) error {
			run := systemBackupRunFromDTOInternal(*dto)
			return s.db.WithContext(ctx).Save(&run).Error
		},
		FailRun: func(ctx context.Context, id, _ string) error {
			return s.db.WithContext(ctx).Model(&SystemBackupRun{}).Where("id = ?", id).Updates(map[string]any{"status": SystemBackupStatusFailed, "error_code": backuptypes.SystemBackupErrorFailed}).Error
		},
		DeleteRun: func(ctx context.Context, id string) error {
			return s.db.WithContext(ctx).Where("id = ?", id).Delete(&SystemBackupRun{}).Error
		},
		Run: func(ctx context.Context, id string) (*backuptypes.SystemBackupRun, error) {
			var run SystemBackupRun
			if err := s.db.WithContext(ctx).Where("id = ?", id).First(&run).Error; err != nil {
				return nil, err
			}
			return new(run.ToDTO()), nil
		},
		ListRuns: func(ctx context.Context, params pagination.QueryParams) ([]backuptypes.SystemBackupRun, pagination.Response, error) {
			var records []SystemBackupRun
			query := s.db.WithContext(ctx).Model(&SystemBackupRun{})
			if term := strings.TrimSpace(params.Search); term != "" {
				pattern := "%" + term + "%"
				query = query.Where("status LIKE ? OR trigger LIKE ? OR archive_name LIKE ? OR error_code LIKE ?", pattern, pattern, pattern, pattern)
			}
			response, err := pagination.PaginateAndSortDB(params, query, &records)
			if err != nil {
				return nil, pagination.Response{}, err
			}
			result := make([]backuptypes.SystemBackupRun, len(records))
			for i := range records {
				result[i] = records[i].ToDTO()
			}
			return result, response, nil
		},
		ExpiredRunIDs: func(ctx context.Context, policyID string, keep int) ([]string, error) {
			var ids []string
			err := s.db.WithContext(ctx).Model(&SystemBackupRun{}).
				Where("policy_id = ? AND status = ? AND archive_name IS NOT NULL", policyID, SystemBackupStatusSucceeded).
				Order("created_at DESC").Offset(keep).Pluck("id", &ids).Error
			return ids, err
		},
		RunsByID: func(ctx context.Context, ids []string) ([]*backuptypes.SystemBackupRun, error) {
			var records []SystemBackupRun
			if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&records).Error; err != nil {
				return nil, err
			}
			result := make([]*backuptypes.SystemBackupRun, len(records))
			for i := range records {
				result[i] = new(records[i].ToDTO())
			}
			return result, nil
		},
		LatestPolicyRun: func(ctx context.Context, policyID string) (*backuptypes.SystemBackupRun, error) {
			var run SystemBackupRun
			err := s.db.WithContext(ctx).Where("policy_id = ?", policyID).Order("created_at DESC").First(&run).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			return new(run.ToDTO()), nil
		},
		CountRuns: func(ctx context.Context) (int64, error) {
			var count int64
			err := s.db.WithContext(ctx).Model(&SystemBackupRun{}).Count(&count).Error
			return count, err
		},
		CountVolumeRepositoryRuns: func(ctx context.Context) (int64, error) {
			var count int64
			err := s.db.WithContext(ctx).Model(&volume.VolumeBackup{}).Where("format = ?", volume.VolumeBackupFormatRustic).Count(&count).Error
			return count, err
		},
		Policies: func(ctx context.Context) ([]backuptypes.SystemBackupPolicy, error) {
			policies, err := s.backupPoliciesInternal(ctx)
			if err != nil {
				return nil, err
			}
			result := make([]backuptypes.SystemBackupPolicy, len(policies))
			for i := range policies {
				result[i] = policies[i].ToDTO()
			}
			return result, nil
		},
		Policy: func(ctx context.Context, id string) (*backuptypes.SystemBackupPolicy, error) {
			if strings.TrimSpace(id) == "" {
				return nil, nil
			}
			var policy SystemBackupPolicy
			err := s.db.WithContext(ctx).Where("id = ?", id).First(&policy).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil
			}
			if err != nil {
				return nil, fmt.Errorf("failed to load system backup policy: %w", err)
			}
			return new(policy.ToDTO()), nil
		},
		ReconcilePolicies: func(
			ctx context.Context,
			updates []backuptypes.UpdateSystemBackupPolicy,
			build func(context.Context, *backuptypes.SystemBackupPolicy, backuptypes.UpdateSystemBackupPolicy) error,
			unregister func(context.Context, string),
			reschedule func(context.Context, *backuptypes.SystemBackupPolicy),
		) error {
			existing, err := s.backupPoliciesInternal(ctx)
			if err != nil {
				return err
			}
			return backup.PolicyReconciliation[SystemBackupPolicy, backuptypes.UpdateSystemBackupPolicy]{
				Domain:   "system",
				DB:       s.db,
				Existing: existing,
				ID:       func(policy *SystemBackupPolicy) string { return policy.ID },
				UpdateID: func(update backuptypes.UpdateSystemBackupPolicy) string { return update.ID },
				New:      func() SystemBackupPolicy { return SystemBackupPolicy{} },
				Build: func(ctx context.Context, policy *SystemBackupPolicy, update backuptypes.UpdateSystemBackupPolicy) error {
					dto := policy.ToDTO()
					if buildErr := build(ctx, &dto, update); buildErr != nil {
						return buildErr
					}
					policy.applyDTOInternal(dto)
					return nil
				},
				Unregister: unregister,
				Reschedule: func(ctx context.Context, policy *SystemBackupPolicy) {
					reschedule(ctx, new(policy.ToDTO()))
				},
			}.Run(ctx, updates)
		},
		ListHistory: func(ctx context.Context, params pagination.QueryParams) ([]backuptypes.HistoryEntry, pagination.Response, error) {
			query := s.db.WithContext(ctx).Table("(?) AS backup_history", s.db.Raw(systemBackupHistoryUnion))
			if term := strings.TrimSpace(params.Search); term != "" {
				pattern := "%" + term + "%"
				query = query.Where("id LIKE ? OR status LIKE ? OR trigger LIKE ? OR destination LIKE ? OR COALESCE(error, '') LIKE ? OR "+
					"resource_name LIKE ? OR resource_type LIKE ? OR type LIKE ?",
					pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern,
				)
			}
			query = pagination.ApplyFilter(query, "type", params.Filters["type"])
			if params.Sort == "" {
				params.Sort = "createdAt"
				params.Order = pagination.SortDesc
			}
			var history []backuptypes.HistoryEntry
			page, err := pagination.PaginateAndSortDB(params, query, &history)
			return history, page, err
		},
	}
}

func (s *SystemService) backupPoliciesInternal(ctx context.Context) ([]SystemBackupPolicy, error) {
	var policies []SystemBackupPolicy
	if err := s.db.WithContext(ctx).Order("created_at ASC").Find(&policies).Error; err != nil {
		return nil, fmt.Errorf("failed to load system backup policies: %w", err)
	}
	return policies, nil
}
