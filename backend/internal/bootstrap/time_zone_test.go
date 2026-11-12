// Source: docklet-next/issues/319
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original persisted zone save through the real settings subscription and scheduler.

package bootstrap

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/job"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	scheduleutil "github.com/florianfinn/docklet-next/backend/pkg/scheduler/schedule"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/jobschedule"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"path/filepath"
	"testing"
	"time"
)

func TestHubZoneSaveReinstallsSchedulerAndAcceptsOrdinaryScheduleSave(t *testing.T) {
	db, e := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "zone.db")), database.MigrationOptions{})
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	settingsService, e := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, settingsService.Stop(context.WithoutCancel(t.Context()))) })
	cfg := &config.Config{Timezone: "UTC"}
	settingsService.BindZoneProvider(cfg)
	runtime := francistest.New(t)
	coordinator := runs.New(runtime.Service(), time.UTC)
	require.NoError(t, coordinator.Register(runtime))
	francistest.Start(t, runtime)
	jobs := job.NewJobService(db, settingsService, cfg, coordinator, nil, nil, nil)
	controller, e := scheduler.NewJobScheduler(t.Context(), coordinator, time.UTC)
	require.NoError(t, e)
	controller.(interface {
		SetZoneProvider(scheduleutil.ZoneProvider)
	}).SetZoneProvider(cfg)
	jobs.SetScheduler(t.Context(), controller)
	require.NoError(t, controller.RegisterJob(&st.GenericJob{JobName: "docker-client-refresh", ScheduleFn: func(ctx context.Context) string {
		return settingsService.GetStringSetting(ctx, "dockerClientRefreshInterval", "0 0 4 * * *")
	}}))
	require.NoError(t, controller.StartScheduler(t.Context()))
	t.Cleanup(func() { require.NoError(t, controller.Stop(context.WithoutCancel(t.Context()))) })
	lifecycle := fxtest.NewLifecycle(t)
	require.NoError(t, setupSettingsSubscriptionsInternal(settingsSubscriptionsParams{Lifecycle: lifecycle, LifecycleCtx: t.Context(), Config: cfg, Scheduler: controller, Settings: settingsService}))
	t.Cleanup(func() { require.NoError(t, lifecycle.Stop(context.WithoutCancel(t.Context()))) })
	before, e := coordinator.ScheduleState(t.Context(), "docker-client-refresh")
	require.NoError(t, e)
	require.Equal(t, "UTC", before.Zone)
	_, e = settingsService.UpdateSettings(t.Context(), dto.Update{TimeZone: new("Asia/Tokyo")})
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		state, e := coordinator.ScheduleState(t.Context(), "docker-client-refresh")
		return e == nil && state.Zone == "Asia/Tokyo" && state.Generation > before.Generation
	}, 5*time.Second, 5*time.Millisecond)
	_, e = jobs.UpdateJobSchedules(t.Context(), jobschedule.Update{DockerClientRefreshInterval: new("0 15 4 * * *")})
	require.NoError(t, e)
	after, e := coordinator.ScheduleState(t.Context(), "docker-client-refresh")
	require.NoError(t, e)
	require.Equal(t, "Asia/Tokyo", after.Zone)
	require.Equal(t, "0 15 4 * * *", after.Schedule)
	require.Empty(t, after.Runs)
}
