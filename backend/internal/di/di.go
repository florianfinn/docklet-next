// Source: getarcaneapp/arcane/backend/internal/di/di.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

// Package di owns the backend's dependency-injection graph.
package di

import (
	"context"
	"log/slog"

	"go.uber.org/fx"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/apns"
	"github.com/florianfinn/docklet-next/backend/internal/appimages"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/build"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/dashboard"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/diagnostics"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/federated"
	"github.com/florianfinn/docklet-next/backend/internal/gitops"
	"github.com/florianfinn/docklet-next/backend/internal/gitrepo"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/job"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/network"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/oidc"
	"github.com/florianfinn/docklet-next/backend/internal/passkey"
	"github.com/florianfinn/docklet-next/backend/internal/port"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/search"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/swarm"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/internal/template"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	"github.com/florianfinn/docklet-next/backend/internal/upload"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/internal/variable"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/internal/vulnerability"
	"github.com/florianfinn/docklet-next/backend/internal/webhook"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"github.com/florianfinn/docklet-next/backend/resources"
)

// ActorOptions provides the durable Francis runtime and admission.
var ActorOptions = fx.Options(
	fx.Provide(provideActorRuntimeInternal, provideRunCoordinatorInternal),
	fx.Provide(provideAdmissionGateInternal),
	fx.Provide(provideTunnelRegistryInternal),
)

// ServiceOptions constructs services once and injects them into route modules.
var ServiceOptions = fx.Options(
	fx.Supply(
		resources.FS,
	),
	fx.Provide(
		fx.Annotate(activity.NewActivityService, fx.As(fx.Self()), fx.As(new(activitylib.Service))),
		event.NewEventService,
		job.NewJobService,
		role.NewRoleService,
		apikey.NewApiKeyService,
		func(db *database.DB, roles *role.RoleService) *user.UserService {
			return user.NewUserService(db, roles, session.RevokeAllUserSessionsExceptInDB)
		},
		federated.NewFederatedCredentialService,
		kv.NewKVService,
		appimages.NewApplicationImagesService,
		session.NewSessionService,
		passkey.NewPasskeyService,
		environment.NewEnvironmentService,
		apns.NewApnsService,
		notification.NewNotificationService,
		vulnerability.NewVulnerabilityService,
		imageupdate.NewImageUpdateService,
		image.NewImageService,
		build.NewBuildService,
		project.NewLifecycleService,
		container.NewContainerService,
		dashboard.NewDashboardService,
		network.NewNetworkService,
		port.NewPortService,
		swarm.NewSwarmService,
		template.NewTemplateService,
		oidc.NewOidcService,
		system.NewSystemService,
		diagnostics.NewDiagnosticsService,
		gitops.NewGitOpsSyncService,
		variable.NewVariableService,
		backup.NewRecoveryKeyStore,
		upload.NewUploadService,
		auth.NewAuthService,
		settings.NewSettingsSearchService,
		fx.Annotate(settings.NewSettingsService, fx.OnStop(func(ctx context.Context, service *settings.SettingsService) error { return service.Stop(ctx) })),
		fx.Annotate(volume.NewVolumeService, fx.OnStop(func(ctx context.Context, service *volume.VolumeService) { service.CleanupHelperContainers(ctx) })),
		fx.Annotate(webhook.NewWebhookService,
			fx.OnStart(func(ctx context.Context, service *webhook.WebhookService) error { return service.LoadTokenHashes(ctx) }),
			fx.OnStop(func(ctx context.Context, service *webhook.WebhookService) {
				if err := service.DrainActions(ctx); err != nil {
					slog.WarnContext(ctx, "shutdown proceeding with webhook actions still in flight", "error", err)
				}
			}),
		),
	),
	// These providers bind config values, callbacks, or managed resource lifetimes.
	fx.Provide(
		provideJWKSetManagerInternal,
		provideDockerClientServiceInternal,
		provideProjectServiceInternal,
		provideBackupEngineInternal,
		provideVersionServiceInternal,
		provideGitRepositoryServiceInternal,
		provideS3ServiceInternal,
		provideContainerRegistryServiceInternal,
		provideUpdaterServiceInternal,
		provideAuthMiddlewareInternal,
	),
	fx.Provide(
		event.New,
		role.New,
		apikey.New,
		template.New,
		gitrepo.New,
		job.New,
		environment.New,
		image.New,
		container.New,
		dashboard.New,
		swarm.New,
		system.New,
		volume.New,
		updater.New,
		webhook.New,
		apns.New,
		notification.New,
		vulnerability.New,
		project.New,
		gitops.New,
		variable.New,
		upload.New,
		search.New,
		provideActivityModuleInternal,
		provideImageUpdateModuleInternal,
		provideSettingsModuleInternal,
		provideS3ModuleInternal,
		provideContainerRegistryModuleInternal,
		provideAuthModuleInternal,
		provideUserModuleInternal,
	),
)

// JobOptions provides every scheduler job. Registration and settings callbacks
// remain bootstrap concerns because their ordering is application-specific.
var JobOptions = fx.Options(
	fx.Provide(
		scheduler.NewAutoUpdateJob,
		scheduler.NewImageUpdateWatcher,
		scheduler.NewDockerClientRefreshJob,
		scheduler.NewAnalyticsJob,
		scheduler.NewEventCleanupJob,
		scheduler.NewPruningVolumeHelperJob,
		scheduler.NewExpiredSessionsCleanupJob,
		scheduler.NewScheduledPruneJob,
		provideFilesystemWatcherJobInternal,
		scheduler.NewVulnerabilityScanJob,
		scheduler.NewVulnerabilityRiskJob,
		scheduler.NewAutoPatchJob,
		scheduler.NewAutoHealJob,
		scheduler.NewActivitySweepJob,
		scheduler.NewUploadSessionsCleanupJob,
		scheduler.NewGitCloneCleanupJob,
		fx.Annotate(scheduler.NewUpgradeLogCleanupJob, fx.ResultTags(`name:"upgrade-log-cleanup"`)),
		scheduler.NewApnsOutboxJob,
	),
)
