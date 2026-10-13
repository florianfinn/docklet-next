// Source: getarcaneapp/arcane/backend/pkg/scheduler/job_schedule_integration_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package scheduler

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/types/jobschedule"
	"github.com/libtnb/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/job"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
)

func TestDeprecatedImagePollingSchedulePersistsWithoutRuntimeJob(t *testing.T) {
	ctx := t.Context()
	databasePath := filepath.Join(t.TempDir(), "arcane.db")
	db, settingsService := openJobScheduleTestDatabaseInternal(t, ctx, databasePath)

	appConfig := &config.Config{Timezone: "UTC"}
	lifecycleCtx, cancelLifecycle := context.WithCancel(t.Context())
	jobScheduler := newJobSchedulerForTestInternal(t, lifecycleCtx, appConfig.GetLocation())

	jobService := job.NewJobService(db, settingsService, appConfig, jobScheduler.coordinator, nil, nil, nil)
	jobService.SetScheduler(lifecycleCtx, jobScheduler)
	require.NoError(t, jobScheduler.StartScheduler(t.Context()))

	_, ok := jobScheduler.GetJobRuntimeState("image-polling")
	require.False(t, ok)
	require.Empty(t, jobScheduler.ListRegisteredJobs())

	requestedSchedule := "0 0 8 * * *"
	updatedSchedules, err := jobService.UpdateJobSchedules(ctx, jobschedule.Update{
		PollingInterval: &requestedSchedule,
	})
	require.NoError(t, err)
	require.Equal(t, requestedSchedule, updatedSchedules.PollingInterval)

	var persisted settings.SettingVariable
	require.NoError(t, db.WithContext(ctx).First(&persisted, "key = ?", "pollingInterval").Error)
	require.Equal(t, requestedSchedule, persisted.Value)
	require.Equal(t, requestedSchedule, settingsService.GetStringSetting(ctx, "pollingInterval", ""))

	_, ok = jobScheduler.GetJobRuntimeState("image-polling")
	require.False(t, ok)
	require.Empty(t, jobScheduler.ListRegisteredJobs())

	jobs, err := jobService.ListJobs(ctx)
	require.NoError(t, err)
	imagePollingStatus := findJobStatusInternal(t, jobs, "image-polling")
	require.Equal(t, requestedSchedule, imagePollingStatus.Schedule)
	require.NotNil(t, imagePollingStatus.NextRun)
	require.True(t, imagePollingStatus.IsContinuous)
	require.Equal(t, "pollingInterval", imagePollingStatus.SettingsKey)

	cancelLifecycle()
	require.NoError(t, stopJobSchedulerForTestInternal(t.Context(), jobScheduler))
	closeJobScheduleTestDatabaseInternal(t, db)

	restartDB, restartSettingsService := openJobScheduleTestDatabaseInternal(t, ctx, databasePath)
	restartLifecycleCtx, cancelRestartLifecycle := context.WithCancel(t.Context())
	restartScheduler := newJobSchedulerForTestInternal(t, restartLifecycleCtx, appConfig.GetLocation())
	restartJobService := job.NewJobService(restartDB, restartSettingsService, appConfig, restartScheduler.coordinator, nil, nil, nil)
	restartJobService.SetScheduler(restartLifecycleCtx, restartScheduler)
	require.NoError(t, restartScheduler.StartScheduler(t.Context()))
	t.Cleanup(func() {
		cancelRestartLifecycle()
		require.NoError(t, stopJobSchedulerForTestInternal(context.WithoutCancel(t.Context()), restartScheduler))
		closeJobScheduleTestDatabaseInternal(t, restartDB)
	})

	_, ok = restartScheduler.GetJobRuntimeState("image-polling")
	require.False(t, ok)
	require.Empty(t, restartScheduler.ListRegisteredJobs())
	require.Equal(t, requestedSchedule, restartSettingsService.GetStringSetting(ctx, "pollingInterval", ""))

	restartedJobs, err := restartJobService.ListJobs(ctx)
	require.NoError(t, err)
	require.Equal(t, requestedSchedule, findJobStatusInternal(t, restartedJobs, "image-polling").Schedule)

	require.NoError(t, restartSettingsService.SetBoolSetting(ctx, "pollingEnabled", false))
	disabledSchedule := "0 0 9 * * *"
	_, err = restartJobService.UpdateJobSchedules(ctx, jobschedule.Update{PollingInterval: &disabledSchedule})
	require.NoError(t, err)

	_, ok = restartScheduler.GetJobRuntimeState("image-polling")
	require.False(t, ok)
	require.Empty(t, restartScheduler.ListRegisteredJobs())

	disabledJobs, err := restartJobService.ListJobs(ctx)
	require.NoError(t, err)
	disabledStatus := findJobStatusInternal(t, disabledJobs, "image-polling")
	require.False(t, disabledStatus.Enabled)
	require.Equal(t, disabledSchedule, disabledStatus.Schedule)
}

func openJobScheduleTestDatabaseInternal(t *testing.T, ctx context.Context, databasePath string) (*database.DB, *settings.SettingsService) {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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

func findJobStatusInternal(t *testing.T, jobs *jobschedule.JobListResponse, jobID string) jobschedule.JobStatus {
	t.Helper()

	for _, job := range jobs.Jobs {
		if job.ID == jobID {
			return job
		}
	}
	require.FailNowf(t, "unexpected failure", "job %q not found", jobID)
	return jobschedule.JobStatus{}
}
