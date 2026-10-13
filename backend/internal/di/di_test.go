// Source: getarcaneapp/arcane/backend/internal/di/di_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed upstream analytics heartbeat; functional cut: removed template catalogue; functional cut: removed federated credentials; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories.

package di

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/libtnb/sqlite"
	"github.com/moby/moby/api/types/events"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/appimages"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/build"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/dashboard"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/diagnostics"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/job"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/network"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/oidc"
	"github.com/florianfinn/docklet-next/backend/internal/port"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	s3domain "github.com/florianfinn/docklet-next/backend/internal/s3"
	"github.com/florianfinn/docklet-next/backend/internal/search"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/swarm"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/internal/vulnerability"
	"github.com/florianfinn/docklet-next/backend/internal/webhook"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/oidcjwk"
)

type graphParams struct {
	fx.In

	AppImages         *appimages.ApplicationImagesService
	User              *user.UserService
	Project           *project.ProjectService
	Environment       *environment.EnvironmentService
	Settings          *settings.SettingsService
	KV                *kv.KVService
	JobSchedule       *job.JobService
	Search            *search.Module
	Container         *container.Module
	Image             *image.ImageService
	Build             *build.BuildService
	Volume            *volume.Module
	BackupEngine      *backup.Engine
	S3Destination     *s3domain.Module
	Network           *network.NetworkService
	Port              *port.PortService
	Swarm             *swarm.SwarmService
	ImageUpdate       *imageupdate.ImageUpdateService
	Session           *session.SessionService
	Auth              *auth.AuthService
	Oidc              *oidc.OidcService
	Docker            *docker.DockerClientService
	ContainerRegistry *registry.ContainerRegistryService
	System            *system.Module
	Diagnostics       *diagnostics.DiagnosticsService
	Updater           *updater.Module
	Event             *event.EventService
	Activity          *activity.ActivityService
	Version           *version.VersionService
	Notification      *notification.NotificationService
	ApiKey            *apikey.ApiKeyService
	Webhook           *webhook.Module
	Vulnerability     *vulnerability.VulnerabilityService
	Dashboard         *dashboard.Module
	Role              *role.RoleService
	AuthMiddleware    *auth.AuthMiddleware
	JWKSetManager     *oidcjwk.KeySetManager

	AutoUpdate             *scheduler.AutoUpdateJob
	ImageUpdateWatcher     *scheduler.ImageUpdateWatcher
	DockerClientRefresh    *scheduler.DockerClientRefreshJob
	EventCleanup           *scheduler.EventCleanupJob
	PruningVolumeHelper    *scheduler.PruningVolumeHelperJob
	ExpiredSessionsCleanup *scheduler.ExpiredSessionsCleanupJob
	ScheduledPrune         *scheduler.ScheduledPruneJob
	FilesystemWatcher      *scheduler.FilesystemWatcherJob
	VulnerabilityScan      *scheduler.VulnerabilityScanJob
	AutoHeal               *scheduler.AutoHealJob
}

func TestOptionsValidate(t *testing.T) {
	_, cancel := context.WithCancel(t.Context())
	defer cancel()
	err := fx.ValidateApp(
		fx.Supply(
			cancel,
			&config.Config{},
			(*database.DB)(nil),
			&http.Client{},
		),
		fx.Provide(func() context.Context { return t.Context() }),
		ActorOptions,
		ServiceOptions,
		JobOptions,
		fx.Invoke(func(graphParams) {}),
	)
	require.NoError(t, err)
}

func TestDockerEventLifecycleInternal(t *testing.T) {
	for _, mode := range []string{"manager", "agent", "edge"} {
		t.Run(mode, func(t *testing.T) {
			streamStopped := make(chan struct{})
			var stopped sync.Once
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/_ping") {
					w.Header().Set("Api-Version", "1.56")
					return
				}
				if !strings.HasSuffix(r.URL.Path, "/events") {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				err := json.MarshalWrite(
					w,
					events.Message{
						Type:   events.ContainerEventType,
						Action: events.ActionStart,
						Actor: events.Actor{
							ID:         "daemon-container",
							Attributes: map[string]string{"name": "external"},
						},
					},
				)
				if err != nil {
					return
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				stopped.Do(func() { close(streamStopped) })
			}))
			t.Cleanup(server.Close)
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&event.Event{}))
			databaseDB := &database.DB{DB: db}
			cfg := &config.Config{DockerHost: server.URL, AgentMode: mode != "manager", EdgeAgent: mode == "edge"}
			lifecycle := fxtest.NewLifecycle(t)
			eventService := event.NewEventService(databaseDB, cfg, nil)
			provideDockerClientServiceInternal(t.Context(), lifecycle, databaseDB, cfg, nil, eventService)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 3*time.Second)
				defer cancel()
				require.NoError(t, lifecycle.Stop(ctx))
			})
			require.NoError(t, lifecycle.Start(t.Context()))
			require.Eventually(t, func() bool {
				var count int64
				return db.Model(&event.Event{}).Count(&count).Error == nil && count == 1
			}, 2*time.Second, 10*time.Millisecond)
			var record event.Event
			require.NoError(t, db.First(&record).Error)
			require.Equal(t, event.EventTypeContainerStart, record.Type)
			require.Equal(t, "System", *record.Username)
			require.Equal(t, "0", *record.EnvironmentID)
			require.Equal(t, "docker", record.Metadata["source"])
			stopCtx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			require.NoError(t, lifecycle.Stop(stopCtx))
			select {
			case <-streamStopped:
			case <-stopCtx.Done():
				t.Fatal("Docker event stream was not joined")
			}
		})
	}
}
