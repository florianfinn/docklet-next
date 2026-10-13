// Source: getarcaneapp/arcane/backend/internal/di/providers.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed template catalogue; functional cut: removed GitOps and saved Git repositories; functional cut: removed S3 backup destinations; functional cut: removed OIDC and passkeys.

package di

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
	"uuid"

	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/moby/moby/api/types/events"
	"go.getarcane.app/streams/bus"
	"go.uber.org/fx"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/build"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/concurrency"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/francis"
)

func provideActivityModuleInternal(service *activity.ActivityService, localEnvironment *environment.EnvironmentService) *activity.Module {
	return activity.New(service, activity.EnvironmentDependencies{
		ProxyJSONRequest:               localEnvironment.ProxyJSONRequest,
		ListActiveRemoteEnvironments:   localEnvironment.ListActiveRemoteEnvironments,
		GetActiveRemoteEnvironment:     localEnvironment.GetActiveRemoteEnvironmentSnapshot,
		ProxyJSONRequestForEnvironment: localEnvironment.ProxyJSONRequestForEnvironment,
		ResolveEnvironmentName:         localEnvironment.ResolveEnvironmentName,
	})
}

func provideImageUpdateModuleInternal(service *imageupdate.ImageUpdateService, imageService *image.ImageService) *imageupdate.Module {
	return imageupdate.New(service, imageService.GetUpdateInfoByImageRefs)
}

func provideSettingsModuleInternal(service *settings.SettingsService, search *settings.SettingsSearchService, localEnvironment *environment.EnvironmentService, cfg *config.Config) *settings.Module {
	return settings.New(service, search, localEnvironment.ProxyJSONRequest, cfg)
}

func provideBackupEngineInternal(
	ctx context.Context,
	lc fx.Lifecycle,
	admission *runs.Admission,
	imageService *image.ImageService,
	runtime *francis.Runtime,
	coordinator *runs.Coordinator,
	roles *role.RoleService,
	cfg *config.Config,
) (
	*backup.Engine,
	error,
) {
	engine := backup.NewEngine(ctx, admission, imageService)
	if err := engine.Register(runtime); err != nil {
		return nil, err
	}
	engine.SetExecutionReady(coordinator.Active)
	engine.SetAuthorize(func(ctx context.Context, command backuptypes.DurableRunCommand) error {
		if cfg.AgentMode && command.UserID == "agent" && command.RequestedWithKey == "" {
			return nil
		}
		if command.UserID == "" {
			return errors.New("requesting user unavailable")
		}
		permissions, err := roles.ResolveExecutionPermissions(ctx, command.UserID, command.RequestedWithKey)
		if err != nil {
			return err
		}
		if !permissions.Allows(command.Permission, command.EnvironmentID) {
			return errors.New("requesting user no longer has permission for this backup")
		}
		if command.Permission == authz.PermSystemBackupsManage && !permissions.IsGlobalAdmin() {
			return errors.New("system backups require a global administrator")
		}
		return nil
	})
	lc.Append(fx.Hook{OnStop: engine.Stop})
	return engine, nil
}

func provideActorRuntimeInternal(
	appCtx context.Context,
	cfg *config.Config,
	db *database.DB,
	settingsService *settings.SettingsService,
	lc fx.Lifecycle,
	cancelApp context.CancelFunc,
) (*francis.Runtime, error) {
	runtime, err := francis.New(cfg.DatabaseURL, "", "", cfg.ActorPort)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error {
		sqlDB, sqlDBErr := db.SQLDB()
		if sqlDBErr != nil {
			return sqlDBErr
		}
		if migrateErr := francis.MigrateLegacyStore(ctx, sqlDB, cfg.DatabaseURL); migrateErr != nil {
			return migrateErr
		}
		if configureIdentityErr := runtime.ConfigureIdentity(cfg.EncryptionKey, settingsService.GetSettingsConfig().InstanceID.Value); configureIdentityErr != nil {
			return configureIdentityErr
		}
		return runtime.Start(ctx, appCtx, func(err error) { slog.ErrorContext(appCtx, "Francis host failed", "error", err); cancelApp() })
	}, OnStop: runtime.Stop})
	return runtime, nil
}

func provideRunCoordinatorInternal(runtime *francis.Runtime, store *kv.KVService, cfg *config.Config) (*runs.Coordinator, error) {
	coordinator := runs.New(store, runtime.Service(), cfg.GetLocation())
	if err := coordinator.Register(runtime); err != nil {
		return nil, err
	}
	return coordinator, nil
}

func provideAdmissionGateInternal(runtime *francis.Runtime) (*runs.Admission, error) {
	admission := runs.NewAdmission(runtime.Service(), uuid.New().String())
	if err := admission.Register(runtime); err != nil {
		return nil, err
	}
	return admission, nil
}

func provideTunnelRegistryInternal(lc fx.Lifecycle) *edge.TunnelRegistry {
	localRegistry := edge.NewTunnelRegistry()
	edge.SetDefaultRegistry(localRegistry)
	lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
		defer edge.ClearDefaultRegistry(localRegistry)
		return localRegistry.Stop(ctx)
	}})
	return localRegistry
}

func provideDockerClientServiceInternal(
	ctx context.Context,
	lc fx.Lifecycle,
	db *database.DB,
	cfg *config.Config,
	localSettings *settings.SettingsService,
	eventService *event.EventService,
) *docker.DockerClientService {
	service := docker.NewDockerClientService(ctx, db, cfg, localSettings, bus.WithDroppedEventCallback(func(message events.Message) {
		// Overflow is recovered synchronously, applying backpressure instead of losing log entries.
		eventService.RecordDockerEvent(ctx, message)
	}))
	var stopStream, stopLog func(context.Context) error
	var unsubscribe func()
	lc.Append(fx.Hook{
		OnStart: func(startCtx context.Context) error {
			run, cleanup := eventService.SubscribeDockerEvents(service.EventBus())
			unsubscribe = cleanup
			var err error
			stopLog, err = concurrency.StartSupervised(ctx, "Docker event log", run)
			if err != nil {
				cleanup()
				return err
			}
			stopStream, err = concurrency.StartSupervised(ctx, "Docker event watcher", func(runCtx context.Context) error {
				service.WatchEvents(runCtx)
				return nil
			})
			if err != nil {
				err = errors.Join(err, stopLog(startCtx))
				cleanup()
			}
			return err
		},
		OnStop: func(stopCtx context.Context) error {
			var err error
			if stopStream != nil {
				err = stopStream(stopCtx)
			}
			if stopLog != nil {
				err = errors.Join(err, stopLog(stopCtx))
			}
			if unsubscribe != nil {
				unsubscribe()
			}
			service.Close()
			return err
		},
	})
	return service
}

func provideVersionServiceInternal(
	httpClient *http.Client,
	cfg *config.Config,
	localRegistry *registry.ContainerRegistryService,
	localDocker *docker.DockerClientService,
	imageUpdate *imageupdate.ImageUpdateService,
	settingsService *settings.SettingsService,
) *version.VersionService {
	return version.NewVersionService(httpClient, cfg.UpdateCheckDisabled, config.Version, config.Revision, localRegistry, localDocker, imageUpdate, settingsService)
}

func provideAuthModuleInternal(service *auth.AuthService, userService *user.UserService, settingsService *settings.SettingsService) *auth.Module {
	return auth.New(service, userService, settingsService)
}

func provideContainerRegistryServiceInternal(
	db *database.DB,
	localDocker *docker.DockerClientService,
	kvService *kv.KVService,
	settingsService *settings.SettingsService,
) *registry.ContainerRegistryService {
	return registry.NewContainerRegistryService(db, func(ctx context.Context) (registry.RegistryDaemonClient, error) { return localDocker.GetClient(ctx) }, kvService, settingsService)
}

func provideContainerRegistryModuleInternal(service *registry.ContainerRegistryService, localEnvironment *environment.EnvironmentService) *registry.Module {
	return registry.New(service, localEnvironment.SyncRegistriesToRemoteEnvironments)
}

func provideProjectServiceInternal(
	db *database.DB,
	localSettings *settings.SettingsService,
	localEvent *event.EventService,
	localImage *image.ImageService,
	localDocker *docker.DockerClientService,
	localBuild *build.BuildService,
	localKv *kv.KVService,
	localRegistry *registry.ContainerRegistryService,
	localEnvironment *environment.EnvironmentService,
	cfg *config.Config,
) *project.ProjectService {
	return project.NewProjectService(db, localSettings, localEvent, localImage, localDocker, localBuild, localRegistry, cfg, localKv, localEnvironment.GetEnabledRegistryCredentials)
}

// updaterServiceParams includes actor registration and worker lifecycle dependencies.
type updaterServiceParams struct {
	fx.In

	Context      context.Context
	Config       *config.Config
	Lifecycle    fx.Lifecycle
	ActorRuntime *francis.Runtime
	Coordinator  *runs.Coordinator
	Admission    *runs.Admission
	Roles        *role.RoleService
	DB           *database.DB
	Settings     *settings.SettingsService
	Docker       *docker.DockerClientService
	Project      *project.ProjectService
	ImageUpdate  *imageupdate.ImageUpdateService
	Registry     *registry.ContainerRegistryService
	Event        *event.EventService
	Image        *image.ImageService
	Notification *notification.NotificationService
	System       *system.SystemService
	Activity     *activity.ActivityService
}

func provideUpdaterServiceInternal(p updaterServiceParams) (*updater.UpdaterService, error) {
	service, err := updater.NewUpdaterService(
		p.DB,
		p.Settings,
		p.Docker,
		p.Project,
		p.ImageUpdate,
		p.Registry,
		p.Event,
		p.Image,
		p.Notification,
		p.System,
		p.Activity,
		p.Config,
		p.Coordinator,
		p.Admission,
		p.Roles,
	)
	if err != nil {
		return nil, err
	}
	if registerActorsErr := service.RegisterActors(p.ActorRuntime); registerActorsErr != nil {
		return nil, registerActorsErr
	}
	p.Lifecycle.Append(
		fx.Hook{
			OnStart: func(
				context.Context,
			) error {
				return service.Start(
					p.Context,
				)
			},
			OnStop: service.Stop,
		},
	) //nolint:contextcheck // Workers inherit the application lifetime after startup returns.
	return service, nil
}

func provideUserModuleInternal(service *user.UserService, localAuth *auth.AuthService, settingsService *settings.SettingsService) *user.Module {
	return user.New(service, localAuth.InvalidateUserTokenCache, settingsService)
}

func provideAuthMiddlewareInternal(
	authService *auth.AuthService,
	apiKey *apikey.ApiKeyService,
	env *environment.EnvironmentService,
	localRole *role.RoleService,
	cfg *config.Config,
) *auth.AuthMiddleware {
	return auth.NewAuthMiddleware(authService, cfg).
		WithApiKeyValidator(apiKey).
		WithEnvironmentAccessTokenResolver(env).
		WithPermissionResolver(localRole)
}

func provideFilesystemWatcherJobInternal(
	ctx context.Context,
	lc fx.Lifecycle,
	localProject *project.ProjectService,
	localSettings *settings.SettingsService,
	cfg *config.Config,
) (
	*scheduler.FilesystemWatcherJob,
	error,
) {
	job, err := scheduler.NewFilesystemWatcherJob(ctx, localProject, localSettings, cfg.ProjectScanMaxDepth)
	if err != nil {
		return nil, err
	}
	var stop func(context.Context) error
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			stop, err = concurrency.StartSupervised(ctx, "filesystem watcher", func(runCtx context.Context) error {
				delay := 5 * time.Second
				for runCtx.Err() == nil {
					startErr := job.Start(runCtx)
					if startErr == nil {
						break
					}
					slog.WarnContext(runCtx, "Filesystem watcher startup failed; retrying", "error", startErr)

					timer := time.NewTimer(delay)
					select {
					case <-runCtx.Done():
						timer.Stop()
						return nil
					case <-timer.C:
					}
					delay = min(delay*2, 5*time.Minute)
				}
				<-runCtx.Done()
				return nil
			})
			if err != nil {
				return err
			}
			slog.InfoContext(ctx, "Filesystem watcher job registered")
			return nil
		},
		OnStop: func(stopCtx context.Context) error {
			var stopWorkerErr error
			if stop != nil {
				stopWorkerErr = stop(stopCtx)
			}
			return errors.Join(stopWorkerErr, job.Stop(stopCtx))
		},
	})
	return job, nil
}
