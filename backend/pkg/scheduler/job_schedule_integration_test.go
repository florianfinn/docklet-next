// Source: getarcaneapp/arcane/backend/pkg/scheduler/job_schedule_integration_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; SQLite temp-file persistence fixture; retired image schedules reject writes across restart.

package scheduler

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/jobschedule"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/job"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
)

func TestDeprecatedImagePollingScheduleRefusesWithoutRuntimeJob(t *testing.T) {
	ctx := t.Context()
	databasePath := filepath.Join(t.TempDir(), "arcane.db")
	db, settingsService := openJobScheduleTestDatabaseInternal(t, ctx, databasePath)
	baseline := settingsService.GetStringSetting(ctx, "pollingInterval", "")
	appConfig := &config.Config{Timezone: "UTC"}
	lifecycleCtx, cancelLifecycle := context.WithCancel(t.Context())
	jobScheduler := newJobSchedulerForTestInternal(t, lifecycleCtx, appConfig.GetLocation())
	jobService := job.NewJobService(db, settingsService, appConfig, jobScheduler.coordinator, nil, nil, nil)
	jobService.SetScheduler(lifecycleCtx, jobScheduler)
	require.NoError(t, jobScheduler.StartScheduler(t.Context()))
	requestedSchedule := "0 0 8 * * *"
	_, err := jobService.UpdateJobSchedules(ctx, jobschedule.Update{PollingInterval: &requestedSchedule})
	require.Equal(t, "update.schedule-hub-owned", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Equal(t, baseline, settingsService.GetStringSetting(ctx, "pollingInterval", ""))
	require.Empty(t, jobScheduler.ListRegisteredJobs())
	cancelLifecycle()
	require.NoError(t, stopJobSchedulerForTestInternal(t.Context(), jobScheduler))
	closeJobScheduleTestDatabaseInternal(t, db)

	restartDB, restartSettings := openJobScheduleTestDatabaseInternal(t, ctx, databasePath)
	restartCtx, cancelRestart := context.WithCancel(t.Context())
	restartScheduler := newJobSchedulerForTestInternal(t, restartCtx, appConfig.GetLocation())
	restartJobs := job.NewJobService(restartDB, restartSettings, appConfig, restartScheduler.coordinator, nil, nil, nil)
	restartJobs.SetScheduler(restartCtx, restartScheduler)
	require.NoError(t, restartScheduler.StartScheduler(ctx))
	t.Cleanup(func() {
		cancelRestart()
		require.NoError(t, stopJobSchedulerForTestInternal(context.WithoutCancel(t.Context()), restartScheduler))
		closeJobScheduleTestDatabaseInternal(t, restartDB)
	})
	require.Equal(t, baseline, restartSettings.GetStringSetting(ctx, "pollingInterval", ""))
	require.Empty(t, restartScheduler.ListRegisteredJobs())
	rows, err := restartJobs.ListJobs(ctx)
	require.NoError(t, err)
	for _, row := range rows.Jobs {
		require.NotEqual(t, "image-polling", row.ID)
		require.NotEqual(t, "auto-update", row.ID)
	}
	require.NoError(t, restartSettings.SetBoolSetting(ctx, "pollingEnabled", false))
	_, err = restartJobs.UpdateJobSchedules(ctx, jobschedule.Update{PollingInterval: &requestedSchedule})
	require.Equal(t, "update.schedule-hub-owned", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Equal(t, baseline, restartSettings.GetStringSetting(ctx, "pollingInterval", ""))
	require.Empty(t, restartScheduler.ListRegisteredJobs())
}

func openJobScheduleTestDatabaseInternal(t *testing.T, ctx context.Context, databasePath string) (*database.DB, *settings.SettingsService) {
	t.Helper()

	gormDB, err := gorm.Open(testsqlite.Dialector(t, databasePath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(&settings.SettingVariable{}, &kv.KVEntry{}))

	db := &database.DB{DB: gormDB}
	settingsService, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.EnsureDefaultSettings(ctx))
	require.NoError(t, settingsService.LoadDatabaseSettings(ctx))

	return db, settingsService
}

func closeJobScheduleTestDatabaseInternal(t *testing.T, db *database.DB) {
	t.Helper()

	sqlDB, err := db.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}
