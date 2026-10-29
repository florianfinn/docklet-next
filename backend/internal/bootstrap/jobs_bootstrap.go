// Source: getarcaneapp/arcane/backend/internal/bootstrap/jobs_bootstrap.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed upstream analytics heartbeat and APNS mobile push; functional cut: removed template catalogue; functional cut: removed GitOps and saved Git repositories; M12 persisted volume lifecycle and legacy restore cut.

package bootstrap

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/florianfinn/docklet-next/types/features"
	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"
	"go.uber.org/fx"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/job"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	"github.com/florianfinn/docklet-next/backend/internal/upload"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/internal/vulnerability"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
)

func newJobScheduler(
	appCtx context.Context,
	lc fx.Lifecycle,
	cfg *config.Config,
	runtime *runs.Coordinator,
	_ *runs.Admission,
	imageUpdateWatcher *scheduler.ImageUpdateWatcher,
	systemService *system.SystemService,
	jobService *job.JobService,
	backupEngine *backup.Engine,
	updaterService *updater.UpdaterService,
	volumes *volume.VolumeService,
	uploads *upload.UploadService,
) (
	schedulertypes.JobScheduler,
	error,
) {
	if volumes != nil && uploads != nil {
		volumes.SetBackupInputCleaner(uploads.CleanupRestoreInputs)
	}
	schedulerCtx, cancelScheduler := context.WithCancel(appCtx)
	jobScheduler, err := scheduler.NewJobScheduler(schedulerCtx, jobService.Coordinator(), cfg.GetLocation())
	if err != nil {
		cancelScheduler()
		return nil, err
	}
	jobService.SetScheduler(schedulerCtx, jobScheduler)
	var repairDone chan struct{}
	var resumeDone chan struct{}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			started := false
			defer func() {
				if !started {
					cancelScheduler()
				}
			}()
			slog.InfoContext(appCtx, "Starting scheduler")
			if startErr := jobService.Coordinator().Start(ctx, schedulerCtx); startErr != nil {
				return fmt.Errorf("initialize scheduler: %w", startErr)
			}
			if reconcileCoordinatorStartupErr := reconcileCoordinatorStartupInternal(ctx, jobService, backupEngine, updaterService, volumes, systemService); reconcileCoordinatorStartupErr != nil {
				return fmt.Errorf("reconcile scheduler startup: %w", reconcileCoordinatorStartupErr)
			}
			if startSchedulerErr := jobScheduler.StartScheduler(ctx); startSchedulerErr != nil {
				return fmt.Errorf("start scheduler: %w", startSchedulerErr)
			}
			if volumes != nil {
				go volumes.RetryFencedBackupWriters(schedulerCtx)
			}
			jobService.Coordinator().Activate()
			if backupEngine != nil {
				repairDone = make(chan struct{})
				go func() {
					defer close(repairDone)
					repairBackupDispatchesInternal(schedulerCtx, backupEngine)
				}()
			}
			if !cfg.AgentMode && systemService != nil {
				resumeDone = make(chan struct{})
				go func() { defer close(resumeDone); systemService.ResumeUpdateAllOnStartup(schedulerCtx) }()
			}
			if startupCancellationErr := ctx.Err(); startupCancellationErr != nil {
				return fmt.Errorf("complete scheduler startup: %w", startupCancellationErr)
			}
			started = true
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return stopJobSchedulerInternal(ctx, cancelScheduler, []chan struct{}{repairDone, resumeDone}, jobService.Coordinator(), jobScheduler)
		},
	})
	return jobScheduler, nil
}

func stopJobSchedulerInternal(ctx context.Context, cancelScheduler context.CancelFunc, workers []chan struct{}, coordinator *runs.Coordinator, jobScheduler schedulertypes.JobScheduler) error {
	cancelScheduler()
	for _, done := range workers {
		if done != nil {
			select {
			case <-done:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	err := errors.Join(coordinator.Stop(ctx), jobScheduler.Stop(ctx))
	if err != nil {
		slog.ErrorContext(ctx, "Job scheduler exited with error", "error", err)
		return err
	}
	slog.InfoContext(ctx, "Scheduler stopped")
	return nil
}

func reconcileCoordinatorStartupInternal(
	ctx context.Context,
	jobs *job.JobService,
	engine *backup.Engine,
	updates *updater.UpdaterService,
	volumes *volume.VolumeService,
	systemService *system.SystemService,
) error {
	if err := retireSystemVolumeJobs(ctx, jobs.Coordinator()); err != nil {
		return err
	}
	if volumes != nil {
		if err := volumes.SweepBackupWriters(ctx); err != nil {
			return err
		}
	}
	if engine != nil {
		var fence func(string) bool
		if volumes != nil {
			fence = volumes.BackupWriterFenced
		}
		if err := engine.SettleVolumeRuns(ctx, fence); err != nil {
			return err
		}
	}
	if volumes != nil {
		if reconcileInterruptedBackupsErr := volumes.ReconcileInterruptedBackups(ctx); reconcileInterruptedBackupsErr != nil {
			return reconcileInterruptedBackupsErr
		}
	}
	activities, backups, err := startupProtectionInternal(ctx, jobs, engine, updates)
	if err != nil {
		return err
	}
	if systemService != nil {
		if reconcileSystemBackupsErr := systemService.ReconcileInterruptedBackups(ctx, backups...); reconcileSystemBackupsErr != nil {
			return reconcileSystemBackupsErr
		}
	}
	return jobs.ReconcileStartupActivities(ctx, activities...)
}

func startupProtectionInternal(ctx context.Context, jobs *job.JobService, engine *backup.Engine, updates *updater.UpdaterService) ([]string, []string, error) {
	records, err := jobs.Coordinator().Records(ctx)
	if err != nil {
		return nil, nil, err
	}
	scheduled, err := scheduledBackupProtectionInternal(records)
	if err != nil {
		return nil, nil, err
	}

	activities := []string{}
	backups := make([]string, 0, len(scheduled))
	if engine != nil {
		var loadActivityIDsErr error
		activities, loadActivityIDsErr = engine.ActiveActivityIDs(ctx)
		if loadActivityIDsErr != nil {
			return nil, nil, loadActivityIDsErr
		}
		backups, loadActivityIDsErr = engine.ActiveRunIDs(ctx)
		if loadActivityIDsErr != nil {
			return nil, nil, loadActivityIDsErr
		}
		if reconcileDispatchesErr := engine.ReconcileDispatches(ctx); reconcileDispatchesErr != nil {
			return nil, nil, reconcileDispatchesErr
		}
	}
	if updates != nil {
		ids, activeUpdateActivityIDsErr := updates.ActiveUpdateActivityIDs(ctx)
		if activeUpdateActivityIDsErr != nil {
			return nil, nil, activeUpdateActivityIDsErr
		}
		activities = append(activities, ids...)
	}
	return activities, append(backups, scheduled...), nil
}

func scheduledBackupProtectionInternal(records []schedulertypes.QueueRecord) ([]string, error) {
	ids := []string{}
	for _, record := range records {
		if strings.HasPrefix(record.JobID, "volume-backup:") {
			continue
		}
		for _, run := range record.Runs {
			if run.Status.Terminal() {
				continue
			}
			for _, target := range run.Outcome.Targets {
				if len(target.RecoveryData) == 0 {
					continue
				}
				var checkpoint struct {
					BackupID string `json:"backupId"`
				}
				if err := json.Unmarshal(target.RecoveryData, &checkpoint); err != nil {
					return nil, fmt.Errorf("decode startup recovery evidence: %w", err)
				}
				if checkpoint.BackupID != "" {
					ids = append(ids, checkpoint.BackupID)
				}
			}
		}
	}
	return ids, nil
}

func repairBackupDispatchesInternal(ctx context.Context, engine *backup.Engine) {
	timer := time.NewTicker(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := engine.ReconcileDispatches(ctx); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "Backup dispatch reconciliation failed", "error", err)
			}
		}
	}
}

type registerJobsParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	AppCtx    context.Context
	Config    *config.Config
	Scheduler schedulertypes.JobScheduler

	DB            *database.DB
	Notification  *notification.NotificationService
	Vulnerability *vulnerability.VulnerabilityService
	Activity      *activity.ActivityService
	Environment   *environment.EnvironmentService
	JobSchedule   *job.JobService
	Settings      *settings.SettingsService
	Volume        *volume.VolumeService
	System        *system.SystemService
	Admission     *runs.Admission
	Updater       *updater.UpdaterService
	Version       *version.VersionService

	AutoUpdate             *scheduler.AutoUpdateJob
	ImageUpdateWatcher     *scheduler.ImageUpdateWatcher
	DockerClientRefresh    *scheduler.DockerClientRefreshJob
	UpgradeLogCleanup      schedulertypes.Job `name:"upgrade-log-cleanup"`
	EventCleanup           *scheduler.EventCleanupJob
	PruningVolumeHelper    *scheduler.PruningVolumeHelperJob
	ExpiredSessionsCleanup *scheduler.ExpiredSessionsCleanupJob
	ScheduledPrune         *scheduler.ScheduledPruneJob
	FilesystemWatcher      *scheduler.FilesystemWatcherJob
	VulnerabilityScan      *scheduler.VulnerabilityScanJob
	VulnerabilityRisk      *scheduler.VulnerabilityRiskJob
	AutoPatch              *scheduler.AutoPatchJob
	AutoHeal               *scheduler.AutoHealJob
	ActivitySweep          *scheduler.ActivitySweepJob
	UploadSessionsCleanup  *scheduler.UploadSessionsCleanupJob
}

func registerJobs(params registerJobsParams) error {
	params.JobSchedule.SetScheduler(params.AppCtx, params.Scheduler)
	if err := registerImagePolicyJobs(params); err != nil {
		return err
	}

	for _, job := range []schedulertypes.Job{
		params.DockerClientRefresh,
		params.EventCleanup,
		params.PruningVolumeHelper,
		params.ExpiredSessionsCleanup,
		params.ScheduledPrune,
		params.AutoPatch,
		params.AutoHeal,
		params.ActivitySweep,
		params.UploadSessionsCleanup,
		params.UpgradeLogCleanup,
	} {
		if err := params.Scheduler.RegisterJob(job); err != nil {
			return err
		}
	}
	hub := vulnerability.NewHubScans(params.DB, params.Settings, params.Environment, params.Vulnerability, params.Notification)
	params.VulnerabilityScan.SetDispatcher(hub)
	if err := registerVulnerabilityJobs(params.Scheduler, params.Config, params.VulnerabilityScan, params.VulnerabilityRisk, &scheduler.VulnerabilityNotifyJob{Hub: hub}); err != nil {
		return err
	}

	// FilesystemWatcher is intentionally not scheduler-registered; it watches inline
	// and is only rebound on settings changes below.
	// Internal self-healing sweep (managers and agents alike); intentionally
	// absent from job_metadata so it stays out of the Jobs UI.

	// Environment health registers one dynamic job for each environment.
	dynamic := dynamicJobsParams{
		AppCtx:      params.AppCtx,
		Config:      params.Config,
		Scheduler:   params.Scheduler,
		Environment: params.Environment,
		JobSchedule: params.JobSchedule,
		Volume:      params.Volume,
		System:      params.System,
		Admission:   params.Admission,
	}
	if err := registerDynamicJobs(dynamic); err != nil {
		return err
	}
	if !params.Config.AgentMode && params.System != nil {
		params.System.SetRestoreSchedulerRefresh(func(ctx context.Context) error {
			return refreshDynamicJobsAfterRestore(ctx, dynamic)
		})
	}

	if err := setupSettingsSubscriptionsInternal(settingsSubscriptionsParams{
		Lifecycle:          params.Lifecycle,
		LifecycleCtx:       params.AppCtx,
		Config:             params.Config,
		Scheduler:          params.Scheduler,
		Settings:           params.Settings,
		Environment:        params.Environment,
		AutoUpdate:         params.AutoUpdate,
		ImageUpdateWatcher: params.ImageUpdateWatcher,
		FilesystemWatcher:  params.FilesystemWatcher,
		ScheduledPrune:     params.ScheduledPrune,
		VulnerabilityScan:  params.VulnerabilityScan,
		VulnerabilityRisk:  params.VulnerabilityRisk,
		AutoPatch:          params.AutoPatch,
		AutoHeal:           params.AutoHeal,
		PolicyRefresh:      params.Updater.RefreshImagePolicySchedules,
	}); err != nil {
		return err
	}
	return nil
}

type dynamicJobsParams struct {
	AppCtx      context.Context
	Config      *config.Config
	Scheduler   schedulertypes.JobScheduler
	Environment *environment.EnvironmentService
	JobSchedule *job.JobService
	Volume      *volume.VolumeService
	System      *system.SystemService
	Admission   *runs.Admission
}

// registerDynamicJobs injects the scheduler into the services that own per-entity
// jobs and registers the jobs for already-existing entities at startup. AddJob is
// an idempotent upsert, so these run safely before the scheduler is started.
func registerDynamicJobs(params dynamicJobsParams) error {
	// Volume backups run on the environment that owns the Docker volume. This is
	// registered on managers and agents; environment proxying persists each policy
	// in the correct Arcane database.
	if params.Volume != nil {
		if err := params.Volume.SetScheduler(params.AppCtx, params.Scheduler, params.Admission); err != nil {
			return err
		}
		params.Volume.RegisterBackupJobsOnStartup(params.AppCtx)
	}
	if !params.Config.AgentMode && params.System != nil {
		if err := params.System.SetBackupScheduler(params.AppCtx, params.Scheduler, params.Admission); err != nil {
			return err
		}
		params.System.RegisterBackupJobOnStartup(params.AppCtx)
	}
	// Environment health: one job per enabled environment (manager only). The Jobs
	// UI still addresses "environment-health" by ID, so bridge its reschedule and
	// run-now back to EnvironmentService.
	if !params.Config.AgentMode && params.Environment != nil {
		if err := params.Environment.SetScheduler(params.AppCtx, params.Scheduler, params.Admission); err != nil {
			return err
		}
		params.JobSchedule.OnEnvironmentHealthReschedule = func(ctx context.Context) {
			params.Environment.RescheduleHealthJobs(ctx)
		}
		params.JobSchedule.RunEnvironmentHealthNow = func(ctx context.Context) error {
			return params.Environment.RunHealthChecksNow(ctx)
		}
		params.Environment.RegisterHealthJobsOnStartup(params.AppCtx)
	}
	return nil
}

type settingsSubscriptionsParams struct {
	PolicyRefresh func(context.Context) error
	Lifecycle     fx.Lifecycle
	LifecycleCtx  context.Context
	Config        *config.Config
	Scheduler     settingsEffectsSchedulerInternal
	Settings      settingsChangeSubscriberInternal
	Environment   timeoutSettingsEnvironmentInternal

	AutoUpdate         *scheduler.AutoUpdateJob
	ImageUpdateWatcher *scheduler.ImageUpdateWatcher
	FilesystemWatcher  *scheduler.FilesystemWatcherJob
	ScheduledPrune     *scheduler.ScheduledPruneJob
	VulnerabilityScan  *scheduler.VulnerabilityScanJob
	VulnerabilityRisk  *scheduler.VulnerabilityRiskJob
	AutoPatch          *scheduler.AutoPatchJob
	AutoHeal           *scheduler.AutoHealJob
}

type settingsChangeSubscriberInternal interface {
	SubscribeSettingsChanges(keys []string, callback func([]libarcane.SettingUpdate)) func()
}

type settingsEffectsSchedulerInternal interface {
	RescheduleJob(ctx context.Context, job schedulertypes.Job) error
}

type timeoutSettingsEnvironmentInternal interface {
	ListRemoteEnvironments(ctx context.Context) ([]environment.Environment, error)
	ProxyRequest(ctx context.Context, envID, method, path string, body []byte) ([]byte, int, error)
}

func setupSettingsSubscriptionsInternal(params settingsSubscriptionsParams) error {
	unsubscribes := make([]func(), 0, 8)
	subscribe := func(keys []string, callback func([]libarcane.SettingUpdate)) {
		unsubscribes = append(unsubscribes, params.Settings.SubscribeSettingsChanges(keys, callback))
	}
	rescheduleOn := func(keys []string, job schedulertypes.Job) {
		subscribe(keys, func(_ []libarcane.SettingUpdate) {
			if err := params.Scheduler.RescheduleJob(params.LifecycleCtx, job); err != nil {
				slog.WarnContext(params.LifecycleCtx, "Failed to reschedule job", "job", job.Name(), "error", err)
			}
		})
	}
	timeoutSyncExecutor, cancelTimeoutSync := setupTimeoutSettingsSubscriptionInternal(params, subscribe)

	subscribe([]string{"pollingEnabled", "pollingInterval"}, func(_ []libarcane.SettingUpdate) {
		if params.PolicyRefresh != nil && !params.Config.AgentMode {
			if err := params.PolicyRefresh(params.LifecycleCtx); err != nil {
				slog.WarnContext(params.LifecycleCtx, "Failed to refresh hub update schedules", "error", err)
			}
		}
	})

	subscribe([]string{"autoUpdate", "autoUpdateInterval"}, func(_ []libarcane.SettingUpdate) {
		if params.PolicyRefresh != nil && !params.Config.AgentMode {
			_ = params.PolicyRefresh(params.LifecycleCtx)
		}
	})

	subscribe([]string{"projectsDirectory", "followProjectSymlinks"}, func(_ []libarcane.SettingUpdate) {
		if params.FilesystemWatcher != nil {
			if err := params.FilesystemWatcher.RestartProjectsWatcher(params.LifecycleCtx); err != nil {
				slog.WarnContext(params.LifecycleCtx, "Failed to restart projects filesystem watcher", "error", err)
			}
		}
	})

	rescheduleOn([]string{"scheduledPruneEnabled", "scheduledPruneInterval"}, params.ScheduledPrune)
	if !params.Config.AgentMode && !params.Config.EdgeAgent {
		rescheduleOn([]string{features.VulnerabilityManagementSettingKey, "vulnerabilityScanEnabled", "vulnerabilityScanSchedule"}, params.VulnerabilityScan)
	}
	rescheduleOn([]string{features.VulnerabilityManagementSettingKey}, params.VulnerabilityRisk)
	rescheduleOn([]string{features.VulnerabilityManagementSettingKey, "imageAutoPatchEnabled", "imageAutoPatchInterval"}, params.AutoPatch)
	rescheduleOn([]string{"autoHealEnabled", "autoHealInterval", "autoHealExcludedContainers", "autoHealExcludedStacks", "autoHealMaxRestarts", "autoHealRestartWindow"}, params.AutoHeal)

	params.Lifecycle.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			for _, unsubscribe := range slices.Backward(unsubscribes) {
				unsubscribe()
			}
			if cancelTimeoutSync != nil {
				cancelTimeoutSync()
			}
			return timeoutSyncExecutor.Stop(ctx)
		},
	})
	return nil
}

type timeoutSyncWorkerInternal struct {
	mu      sync.Mutex
	updates [][]libarcane.SettingUpdate
	wake    chan struct{}
	done    chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	closed  bool
}

func (w *timeoutSyncWorkerInternal) Stop(ctx context.Context) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	w.closed = true
	w.cancel()
	w.mu.Unlock()
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func setupTimeoutSettingsSubscriptionInternal(params settingsSubscriptionsParams, subscribe func([]string, func([]libarcane.SettingUpdate))) (*timeoutSyncWorkerInternal, context.CancelFunc) {
	if params.Config.AgentMode {
		return nil, nil
	}
	ctx, cancel := context.WithCancel(params.LifecycleCtx)
	worker := &timeoutSyncWorkerInternal{wake: make(chan struct{}, 1), done: make(chan struct{}), ctx: ctx, cancel: cancel}
	go func() {
		defer close(worker.done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-worker.wake:
			}
			for {
				worker.mu.Lock()
				if len(worker.updates) == 0 {
					worker.mu.Unlock()
					break
				}
				updates := worker.updates[0]
				worker.updates = worker.updates[1:]
				worker.mu.Unlock()
				syncTimeoutSettingsToAgentsInternal(ctx, params.Environment, updates)
			}
		}
	}()
	subscribe(libarcane.TimeoutSettingKeys(), func(updates []libarcane.SettingUpdate) {
		worker.mu.Lock()
		defer worker.mu.Unlock()
		if worker.closed {
			return
		}
		worker.updates = append(worker.updates, slices.Clone(updates))
		select {
		case worker.wake <- struct{}{}:
		default:
		}
	})
	return worker, cancel
}

// syncTimeoutSettingsToAgentsInternal syncs timeout settings to all connected remote environments
func syncTimeoutSettingsToAgentsInternal(ctx context.Context, localEnvironment timeoutSettingsEnvironmentInternal, timeoutSettings []libarcane.SettingUpdate) {
	envs, err := localEnvironment.ListRemoteEnvironments(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Failed to list remote environments for timeout sync", "error", err)
		return
	}

	if len(envs) == 0 {
		return
	}

	// Build the settings update payload
	settingsMap := make(map[string]string, len(timeoutSettings))
	keys := make([]string, 0, len(timeoutSettings))
	for _, update := range timeoutSettings {
		settingsMap[update.Key] = update.Value
		keys = append(keys, update.Key)
	}
	body, err := json.Marshal(settingsMap)
	if err != nil {
		slog.WarnContext(ctx, "Failed to marshal timeout settings for sync", "error", err)
		return
	}

	slog.InfoContext(ctx, "Syncing environment settings to remote environments", "count", len(envs), "keys", keys)

	for _, env := range envs {
		responseBody, statusCode, proxyRequestErr := localEnvironment.ProxyRequest(ctx, env.ID, http.MethodPut, "/api/environments/0/settings", body)
		if proxyRequestErr != nil {
			slog.WarnContext(ctx, "Failed to sync timeout settings to environment", "environmentId", env.ID, "environmentName", env.Name, "error", proxyRequestErr)
			continue
		}
		if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
			slog.WarnContext(ctx, "Environment returned non-OK status for timeout sync", "environmentId", env.ID, "environmentName", env.Name, "statusCode", statusCode, "response", string(responseBody))
			continue
		}
		slog.DebugContext(ctx, "Successfully synced timeout settings to environment", "environmentId", env.ID, "environmentName", env.Name)
	}
}

// Intel refresh remains host-local; image scans are admitted by the hub dispatcher.
func registerVulnerabilityJobs(controller interface {
	RegisterJob(schedulertypes.Job) error
}, cfg *config.Config, scan, intel schedulertypes.Job, notify ...schedulertypes.Job) error {
	if err := controller.RegisterJob(intel); err != nil {
		return err
	}
	if !cfg.AgentMode && !cfg.EdgeAgent {
		if err := controller.RegisterJob(scan); err != nil {
			return err
		}
		for _, job := range notify {
			if err := controller.RegisterJob(job); err != nil {
				return err
			}
		}
	}
	return nil
}
