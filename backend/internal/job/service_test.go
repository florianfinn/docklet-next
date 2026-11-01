// Source: getarcaneapp/arcane/backend/internal/job/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed upstream analytics heartbeat; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture; M15 simple-user rights and identity contracts.

package job

import (
	"context"
	"encoding/json/v2"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/jobschedule"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/job/children/remote"
	"github.com/florianfinn/docklet-next/backend/internal/job/children/runtime"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	userdomain "github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
)

func setupSettingsTestDBInternal(t *testing.T) *database.DB {
	t.Helper()
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&settings.SettingVariable{}, &kv.KVEntry{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return &database.DB{DB: db}
}

func newSettingsServiceForTestInternal(t testing.TB, ctx context.Context, db *database.DB) (*settings.SettingsService, error) {
	t.Helper()
	service, err := settings.NewSettingsService(ctx, db)
	if err == nil {
		t.Cleanup(func() {
			stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
			defer cancel()
			require.NoError(t, service.Stop(stopCtx))
		})
	}
	return service, err
}

func newJobCoordinatorForTestInternal(t *testing.T, db *database.DB) *runs.Coordinator {
	t.Helper()
	francisRuntime := francistest.New(t)
	coordinator := runs.New(francisRuntime.Service(), time.UTC)
	require.NoError(t, coordinator.Register(francisRuntime))
	francistest.Start(t, francisRuntime)
	return coordinator
}

func newJobServiceForTestInternal(t *testing.T, db *database.DB, settingsService *settings.SettingsService, cfg *config.Config) *JobService {
	t.Helper()
	return NewJobService(db, settingsService, cfg, newJobCoordinatorForTestInternal(t, db), nil, nil, nil)
}

func TestJobService_GetJobSchedules_DefaultDockerClientRefreshInterval(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDBInternal(t)

	settingsSvc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)

	jobSvc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{})
	cfg := jobSvc.GetJobSchedules(ctx)

	require.Equal(t, "0 */5 * * * *", cfg.DockerClientRefreshInterval)
	require.Equal(t, "0 0 * * * *", cfg.PollingInterval)
}

func TestJobService_ListJobs_IncludesDisabledAutoHealJob(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDBInternal(t)

	settingsSvc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsSvc.SetBoolSetting(ctx, "autoHealEnabled", false))

	jobSvc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{})
	jobs, err := jobSvc.ListJobs(ctx)
	require.NoError(t, err)

	autoHealJob := findJobStatusByIDInternal(t, jobs.Jobs, "auto-heal")
	require.False(t, autoHealJob.Enabled)
	require.Equal(t, "autoHealInterval", autoHealJob.SettingsKey)
}

func TestJobService_ListJobs_IncludesDockerClientRefreshJob(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDBInternal(t)

	settingsSvc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)

	jobSvc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{})
	jobs, err := jobSvc.ListJobs(ctx)
	require.NoError(t, err)

	refreshJob := findJobStatusByIDInternal(t, jobs.Jobs, "docker-client-refresh")
	require.True(t, refreshJob.Enabled)
	require.True(t, refreshJob.CanRunManually)
	require.Equal(t, "monitoring", refreshJob.Category)
	require.Equal(t, "dockerClientRefreshInterval", refreshJob.SettingsKey)
	require.Equal(t, "0 */5 * * * *", refreshJob.Schedule)
}

func TestJobService_ListJobs_UsesRuntimeScheduleAndNextRun(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDBInternal(t)

	settingsSvc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)

	nextRun := time.Date(2026, time.July, 10, 8, 0, 0, 0, time.UTC)
	fakeScheduler := newFakeJobSchedulerInternal("docker-client-refresh")
	fakeScheduler.runtimeStates["docker-client-refresh"] = scheduler.JobRuntimeState{
		Schedule:  "0 0 8 * * *",
		NextRun:   &nextRun,
		Scheduled: true,
	}

	jobSvc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{})
	jobSvc.SetScheduler(ctx, fakeScheduler)
	jobs, err := jobSvc.ListJobs(ctx)
	require.NoError(t, err)

	refreshJob := findJobStatusByIDInternal(t, jobs.Jobs, "docker-client-refresh")
	require.Equal(t, "0 0 8 * * *", refreshJob.Schedule)
	require.Equal(t, nextRun, *refreshJob.NextRun)

	now := time.Now().UTC()
	failed := scheduler.Run{
		ID: "failed", EnvironmentID: "0", Status: scheduler.Failed, CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
		Outcome: scheduler.Outcome{Status: scheduler.Failed, Message: "clone failed"},
	}
	succeeded := scheduler.Run{
		ID:            "succeeded",
		EnvironmentID: "0",
		Status:        scheduler.Succeeded,
		CreatedAt:     now.Add(-time.Minute),
		UpdatedAt:     now.Add(-time.Minute),
		FinishedAt:    new(now.Add(-time.Minute)),
	}
	for _, history := range [][]scheduler.Run{{failed, succeeded}, {succeeded, failed}} {
		status := jobschedule.JobStatus{LastRun: new(failed), LastError: failed.Outcome.Message}
		runtime.ApplyRunStatus(&status, history)
		require.Equal(t, succeeded.ID, status.LastRun.ID)
		require.Empty(t, status.LastError)
		require.Equal(t, "clone failed", history[0].Outcome.Message+history[1].Outcome.Message)
	}
	// An acknowledgement updates an old failure's revision, not its execution time.
	failed.UpdatedAt = now.Add(time.Hour)
	status := jobschedule.JobStatus{LastRun: new(succeeded)}
	runtime.ApplyRunStatus(&status, []scheduler.Run{failed})
	require.Equal(t, succeeded.ID, status.LastRun.ID)
	require.Empty(t, status.LastError)
	// A real retry of the older run becomes the latest execution.
	failed.StartedAt = new(now.Add(time.Second))
	failed.Attempts = []scheduler.Attempt{{Number: 2, StartedAt: *failed.StartedAt}}
	runtime.ApplyRunStatus(&status, []scheduler.Run{failed})
	require.Equal(t, failed.ID, status.LastRun.ID)
	require.Equal(t, "clone failed", status.LastError)
	// Merge the same run's terminal revision before choosing an active run.
	status.CurrentRun = new(succeeded)
	status.CurrentRun.Status = scheduler.Running
	succeeded.UpdatedAt = now.Add(2 * time.Second)
	runtime.ApplyRunStatus(&status, []scheduler.Run{succeeded})
	require.Nil(t, status.CurrentRun)
	failed.Status = scheduler.Running
	failed.UpdatedAt = now.Add(2 * time.Hour)
	runtime.ApplyRunStatus(&status, []scheduler.Run{failed})
	require.Equal(t, failed.ID, status.CurrentRun.ID)
	require.Empty(t, status.LastError)

	// A later manual failure stays visible while older work waits to retry.
	retrying := failed
	retrying.Status = scheduler.Retrying
	retrying.NextAttempt = new(now.Add(time.Hour))
	manualFailure := failed
	manualFailure.ID = "manual-failure"
	manualFailure.Status = scheduler.Failed
	manualFailure.CreatedAt = now.Add(time.Minute)
	manualFailure.StartedAt = nil
	manualFailure.Attempts = nil
	manualFailure.Outcome.Message = "latest manual failure"
	succeeded.CreatedAt = now.Add(2 * time.Minute)
	for _, history := range [][]scheduler.Run{{retrying, manualFailure}, {manualFailure, retrying}} {
		status = jobschedule.JobStatus{}
		runtime.ApplyRunStatus(&status, history)
		require.Equal(t, retrying.ID, status.CurrentRun.ID)
		require.Equal(t, manualFailure.ID, status.LastRun.ID)
		require.Equal(t, manualFailure.Outcome.Message, status.LastError)
		runtime.ApplyRunStatus(&status, []scheduler.Run{succeeded})
		require.Empty(t, status.LastError)
	}

	cloneErr := errors.New("failed to copy volume: network is unreachable")
	outcome, runErr := classifyOutcomeInternal("volume-backup:project", scheduler.Outcome{}, cloneErr)
	require.ErrorIs(t, runErr, cloneErr)
	require.Equal(t, scheduler.Failed, outcome.Status)
	outcome, runErr = classifyOutcomeInternal("environment-health:0", scheduler.Outcome{}, context.DeadlineExceeded)
	require.ErrorIs(t, runErr, context.DeadlineExceeded)
	require.Equal(t, scheduler.Retrying, outcome.Status)
	outcome, runErr = classifyOutcomeInternal("docker-client-refresh", scheduler.Outcome{Status: scheduler.Partial}, cloneErr)
	require.ErrorIs(t, runErr, cloneErr)
	require.Equal(t, scheduler.Partial, outcome.Status)
}

func TestJobService_ListJobs_OmitsUnconfiguredLegacyImageWatcher(t *testing.T) {
	db := setupSettingsTestDBInternal(t)
	settingsSvc, err := newSettingsServiceForTestInternal(t, t.Context(), db)
	require.NoError(t, err)
	svc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{})
	jobs, err := svc.ListJobs(t.Context())
	require.NoError(t, err)
	for _, job := range jobs.Jobs {
		require.NotEqual(t, "image-polling", job.ID)
		require.NotEqual(t, "auto-update", job.ID)
	}
}

func TestJobService_UpdateJobSchedules_ReschedulesChangedJob(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDBInternal(t)

	settingsSvc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)

	jobSvc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{})
	fakeScheduler := newFakeJobSchedulerInternal("docker-client-refresh")
	jobSvc.SetScheduler(ctx, fakeScheduler)

	_, err = jobSvc.UpdateJobSchedules(ctx, jobschedule.Update{
		DockerClientRefreshInterval: new("0 */10 * * * *"),
	})
	require.NoError(t, err)

	require.Equal(t, []string{"docker-client-refresh"}, fakeScheduler.rescheduled)
}

func TestJobService_UpdateJobSchedules_UsesLifecycleContextForReschedule(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDBInternal(t)

	settingsSvc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)

	type lifecycleContextKey struct{}
	lifecycleCtx := context.WithValue(t.Context(), lifecycleContextKey{}, true)
	requestCtx, cancelRequest := context.WithCancel(t.Context())

	jobSvc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{})
	fakeScheduler := newFakeJobSchedulerInternal("docker-client-refresh")
	jobSvc.SetScheduler(lifecycleCtx, fakeScheduler)

	_, err = jobSvc.UpdateJobSchedules(requestCtx, jobschedule.Update{
		DockerClientRefreshInterval: new("0 */10 * * * *"),
	})
	require.NoError(t, err)

	cancelRequest()

	require.Len(t, fakeScheduler.rescheduleContexts, 1)
	require.NoError(t, fakeScheduler.rescheduleContexts[0].Err())
	require.Equal(t, true, fakeScheduler.rescheduleContexts[0].Value(lifecycleContextKey{}))
}

func TestJobService_UpdateJobSchedules_RejectsInvalidCronWithoutChangingSetting(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDBInternal(t)

	settingsSvc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsSvc.EnsureDefaultSettings(ctx))
	require.NoError(t, settingsSvc.LoadDatabaseSettings(ctx))

	jobSvc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{})
	fakeScheduler := newFakeJobSchedulerInternal()
	jobSvc.SetScheduler(ctx, fakeScheduler)

	_, err = jobSvc.UpdateJobSchedules(ctx, jobschedule.Update{
		DockerClientRefreshInterval: new("not a cron expression"),
	})
	require.ErrorContains(t, err, "invalid cron expression for dockerClientRefreshInterval")
	require.Equal(t, "0 */5 * * * *", settingsSvc.GetStringSetting(ctx, "dockerClientRefreshInterval", ""))
	require.Empty(t, fakeScheduler.rescheduled)
}

func TestJobService_UpdateJobSchedules_UnchangedScheduleDoesNotReschedule(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDBInternal(t)

	settingsSvc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)

	jobSvc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{})
	updated, err := jobSvc.UpdateJobSchedules(ctx, jobschedule.Update{
		DockerClientRefreshInterval: new("0 */5 * * * *"),
	})
	require.NoError(t, err)
	require.Equal(t, "0 */5 * * * *", updated.DockerClientRefreshInterval)
}

func TestJobService_UpdateJobSchedules_RestoresPreviousScheduleWhenRescheduleFails(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDBInternal(t)

	settingsSvc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsSvc.EnsureDefaultSettings(ctx))
	require.NoError(t, settingsSvc.LoadDatabaseSettings(ctx))

	jobSvc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{})
	fakeScheduler := newFakeJobSchedulerInternal("docker-client-refresh")
	fakeScheduler.rescheduleErr = errors.New("scheduler unavailable")
	jobSvc.SetScheduler(ctx, fakeScheduler)

	_, err = jobSvc.UpdateJobSchedules(ctx, jobschedule.Update{
		DockerClientRefreshInterval: new("0 0 8 * * *"),
	})
	require.ErrorContains(t, err, "scheduler unavailable")
	require.Equal(t, "0 */5 * * * *", settingsSvc.GetStringSetting(ctx, "dockerClientRefreshInterval", ""))

	var persisted settings.SettingVariable
	require.NoError(t, db.WithContext(ctx).First(&persisted, "key = ?", "dockerClientRefreshInterval").Error)
	require.Equal(t, "0 */5 * * * *", persisted.Value)
}

func TestJobService_UpdateJobSchedules_SkipsManagerOnlyJobsInAgentMode(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDBInternal(t)

	settingsSvc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)

	jobSvc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{AgentMode: true})
	fakeScheduler := newFakeJobSchedulerInternal("environment-health")
	jobSvc.SetScheduler(ctx, fakeScheduler)

	_, err = jobSvc.UpdateJobSchedules(ctx, jobschedule.Update{
		EnvironmentHealthInterval: new("0 */5 * * * *"),
	})
	require.NoError(t, err)

	require.Empty(t, fakeScheduler.rescheduled)
}

func TestJobService_UpdateJobSchedules_DelegatesEnvironmentHealthReschedule(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDBInternal(t)

	settingsSvc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)

	jobSvc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{})
	fakeScheduler := newFakeJobSchedulerInternal()
	jobSvc.SetScheduler(ctx, fakeScheduler)

	rescheduled := 0
	jobSvc.OnEnvironmentHealthReschedule = func(context.Context) {
		rescheduled++
	}

	_, err = jobSvc.UpdateJobSchedules(ctx, jobschedule.Update{
		EnvironmentHealthInterval: new("0 */5 * * * *"),
	})
	require.NoError(t, err)
	require.Equal(t, 1, rescheduled)
	require.Empty(t, fakeScheduler.rescheduled)
}

func TestJobService_Submit_PersistsOtherJobRun(t *testing.T) {
	ctx := t.Context()
	db := setupSettingsTestDBInternal(t)
	settingsSvc, err := newSettingsServiceForTestInternal(t, ctx, db)
	require.NoError(t, err)
	fakeScheduler := newFakeJobSchedulerInternal("docker-client-refresh")
	jobSvc := newJobServiceForTestInternal(t, db, settingsSvc, &config.Config{})
	jobSvc.SetScheduler(ctx, fakeScheduler)
	require.NoError(t, db.AutoMigrate(&userdomain.User{}, &access.Assignment{},
		&role.Role{}, &role.UserRoleAssignment{}))
	require.NoError(t, db.Create(&userdomain.User{Username: "operator", BaseModel: database.BaseModel{ID: "operator"}}).Error)
	require.NoError(t, db.Create(&role.Role{ID: authz.BuiltInRoleAdmin, Name: "Admin", Permissions: database.StringSlice(authz.AllPermissions())}).Error)
	require.NoError(t, db.Create(&role.UserRoleAssignment{UserID: "operator", RoleID: authz.BuiltInRoleAdmin}).Error)
	jobSvc.roles = role.NewRoleService(db)
	run, err := jobSvc.Submit(ctx, scheduler.Request{JobID: "docker-client-refresh", EnvironmentID: "0", Trigger: "manual", RequestedBy: "operator"})
	require.NoError(t, err)
	require.Equal(t, scheduler.Queued, run.Status)
	persisted, err := jobSvc.runs.Get(ctx, "0", "docker-client-refresh", run.ID)
	require.NoError(t, err)
	require.Equal(t, run.ID, persisted.ID)
	require.Empty(t, fakeScheduler.busWatcherRuns)
}

func findJobStatusByIDInternal(t *testing.T, jobs []jobschedule.JobStatus, id string) jobschedule.JobStatus {
	t.Helper()

	for _, job := range jobs {
		if job.ID == id {
			return job
		}
	}
	require.FailNowf(t, "unexpected failure", "job %q not found", id)
	return jobschedule.JobStatus{}
}

type fakeJobSchedulerInternal struct {
	jobs               map[string]scheduler.Job
	runtimeStates      map[string]scheduler.JobRuntimeState
	rescheduled        []string
	rescheduleContexts []context.Context
	rescheduleErr      error
	busWatcherRuns     []string
	busWatcherContexts []context.Context
}

func newFakeJobSchedulerInternal(jobIDs ...string) *fakeJobSchedulerInternal {
	jobs := make(map[string]scheduler.Job, len(jobIDs))
	for _, jobID := range jobIDs {
		jobs[jobID] = fakeJobInternal{name: jobID}
	}

	return &fakeJobSchedulerInternal{
		jobs:          jobs,
		runtimeStates: make(map[string]scheduler.JobRuntimeState),
	}
}

func (s *fakeJobSchedulerInternal) GetJob(jobID string) (scheduler.Job, bool) {
	job, ok := s.jobs[jobID]
	return job, ok
}

func (s *fakeJobSchedulerInternal) GetJobRuntimeState(jobID string) (scheduler.JobRuntimeState, bool) {
	state, ok := s.runtimeStates[jobID]
	return state, ok
}

func (s *fakeJobSchedulerInternal) RescheduleJob(ctx context.Context, job scheduler.Job) error {
	s.rescheduled = append(s.rescheduled, job.Name())
	s.rescheduleContexts = append(s.rescheduleContexts, ctx)
	if s.rescheduleErr != nil {
		return s.rescheduleErr
	}

	s.runtimeStates[job.Name()] = scheduler.JobRuntimeState{
		Schedule:  job.Schedule(ctx),
		Scheduled: true,
	}
	return nil
}

func (s *fakeJobSchedulerInternal) RunBusWatcherNow(ctx context.Context, watcherID string) error {
	s.busWatcherRuns = append(s.busWatcherRuns, watcherID)
	s.busWatcherContexts = append(s.busWatcherContexts, ctx)
	return nil
}

type fakeJobInternal struct {
	name string
}

func (j fakeJobInternal) Name() string {
	return j.name
}

func (j fakeJobInternal) Schedule(context.Context) string {
	return "0 0 0 * * *"
}

func (j fakeJobInternal) Run(context.Context) (scheduler.Outcome, error) {
	return scheduler.Outcome{Status: scheduler.Succeeded}, nil
}

func newJobActivityTestServiceInternal(t *testing.T) (*JobService, *activity.ActivityService, *database.DB) {
	t.Helper()
	db := setupSettingsTestDBInternal(t)
	sqlDB, err := db.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(activity.Models()...))
	activities := activity.NewActivityService(db, nil)
	svc := NewJobService(db, nil, &config.Config{}, newJobCoordinatorForTestInternal(t, db), nil, nil, activities)
	return svc, activities, db
}

func TestJobActivityLifecycle(t *testing.T) {
	svc, activities, db := newJobActivityTestServiceInternal(t)
	ctx := t.Context()
	run, err := svc.runs.Submit(ctx, scheduler.Request{JobID: "auto-update", RunID: "b6d679c2-bdf5-4af1-985f-49c468983ba0", Trigger: "scheduled"})
	require.NoError(t, err)
	activityID, err := uuid.Parse(run.ActivityID)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil(), activityID)
	require.Equal(t, "0", run.ActivityEnvironmentID)
	check := func(status activitytypes.Status, precise scheduler.RunStatus) {
		t.Helper()
		detail, detailErr := activities.GetActivityDetail(ctx, "0", run.ActivityID, 10)
		require.NoError(t, detailErr)
		require.Equal(t, status, detail.Activity.Status)
		require.Equal(t, string(precise), detail.Activity.Metadata["jobStatus"])
		require.Equal(t, run.ID, *detail.Activity.BatchID)
	}
	check(activitytypes.StatusQueued, scheduler.Queued)
	duplicate, err := svc.runs.Submit(ctx, scheduler.Request{JobID: run.JobID, RunID: run.ID, Trigger: "scheduled"})
	require.NoError(t, err)
	require.Equal(t, run.ActivityID, duplicate.ActivityID)
	for _, status := range []scheduler.RunStatus{scheduler.Running, scheduler.Waiting, scheduler.Failed} {
		require.NoError(t, svc.runs.UpdateRun(ctx, run, func(current *scheduler.Run) error {
			current.Status = status
			current.UpdatedAt = time.Now().UTC()
			current.Outcome = scheduler.Outcome{Status: status, Message: "operation detail"}
			return nil
		}))
	}
	check(activitytypes.StatusFailed, scheduler.Failed)
	failedDetail, err := activities.GetActivityDetail(ctx, "0", run.ActivityID, 10)
	require.NoError(t, err)
	require.NotNil(t, failedDetail.Activity.Error)
	require.Equal(t, "operation detail", *failedDetail.Activity.Error)
	later, err := svc.runs.Submit(ctx, scheduler.Request{JobID: run.JobID, Trigger: "scheduled"})
	require.NoError(t, err)
	require.NoError(t, svc.runs.UpdateRun(ctx, later, func(current *scheduler.Run) error {
		current.Status = scheduler.Succeeded
		current.UpdatedAt = time.Now().UTC()
		return nil
	}))
	failedDetail, err = activities.GetActivityDetail(ctx, "0", run.ActivityID, 10)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusFailed, failedDetail.Activity.Status)
	require.Equal(t, "operation detail", *failedDetail.Activity.Error)
	_, err = svc.runs.Retry(ctx, "0", run.JobID, run.ID)
	require.NoError(t, err)
	check(activitytypes.StatusQueued, scheduler.Queued)
	_, err = activities.CancelActivity(ctx, "0", run.ActivityID, "operator")
	require.ErrorIs(t, err, activity.ErrActivityNotCancelable)
	check(activitytypes.StatusQueued, scheduler.Queued)
	require.NoError(t, svc.runs.UpdateRun(ctx, run, func(current *scheduler.Run) error {
		current.Status = scheduler.NeedsAttention
		current.UpdatedAt = time.Now().UTC()
		return nil
	}))
	_, err = svc.runs.Resolve(ctx, "0", run.JobID, run.ID, "operator")
	require.NoError(t, err)
	check(activitytypes.StatusCancelled, scheduler.Canceled)
	var count int64
	require.NoError(t, db.Model(&activity.Activity{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestJobActivityVisibilityAndGrouping(t *testing.T) {
	svc, activities, _ := newJobActivityTestServiceInternal(t)
	for _, test := range []struct {
		job, trigger, environment string
		visible                   bool
	}{
		{"auto-update", "scheduled", "0", true},
		{"volume-backup:project", "scheduled", "0", true},
		{"volume-backup:volume", "recovery", "0", true},
		{"system-backup:system", "scheduled", "0", true},
		{"image-polling", "manual", "0", true},
		{"auto-update", "manual", "remote", true},
		{"auto-update", "remote", "0", false},
		{"image-polling", "scheduled", "0", false},
		{"environment-health:0", "scheduled", "0", false},
		{"activity-sweep", "scheduled", "0", false},
	} {
		t.Run(test.job+"/"+test.trigger+"/"+test.environment, func(t *testing.T) {
			run, err := svc.runs.Submit(t.Context(), scheduler.Request{JobID: test.job, Trigger: test.trigger, EnvironmentID: test.environment})
			require.NoError(t, err)
			require.Equal(t, test.visible, run.ActivityID != "")
			if test.visible {
				require.Equal(t, test.environment, run.ActivityEnvironmentID)
				detail, getActivityDetailErr := activities.GetActivityDetail(t.Context(), test.environment, run.ActivityID, 10)
				require.NoError(t, getActivityDetailErr)
				require.Equal(t, test.environment, detail.Activity.Metadata["environmentId"])
			}
			child, err := activities.StartActivity(svc.runContextInternal(t.Context(), run), activity.StartActivityRequest{Type: activitytypes.TypeImagePull})
			require.NoError(t, err)
			require.Equal(t, run.ID, *child.BatchID)
		})
	}
}

func TestJobActivityRestartRepairsFailedProjection(t *testing.T) {
	svc, activities, db := newJobActivityTestServiceInternal(t)
	ctx := t.Context()
	require.NoError(t, db.Migrator().DropTable(&activity.ActivityMessage{}, &activity.Activity{}))
	run, err := svc.runs.Submit(ctx, scheduler.Request{JobID: "auto-update", Trigger: "scheduled"})
	require.NoError(t, err, "activity failure must not reject accepted work")
	require.NotEmpty(t, run.ActivityID)
	require.NoError(t, svc.runs.UpdateRun(ctx, run, func(current *scheduler.Run) error {
		current.Status = scheduler.NeedsAttention
		current.Outcome = scheduler.Outcome{Message: "interrupted before applying updates"}
		current.UpdatedAt = time.Now().UTC()
		return nil
	}))
	require.NoError(t, db.AutoMigrate(activity.Models()...))
	restarted := NewJobService(db, nil, &config.Config{}, svc.runs, nil, nil, activities)
	require.NoError(t, restarted.runs.Start(ctx, ctx))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		require.NoError(t, restarted.runs.Stop(stopCtx))
	})
	require.Eventually(t, func() bool {
		detail, getErr := activities.GetActivityDetail(ctx, "0", run.ActivityID, 10)
		return getErr == nil && detail.Activity.Status == activitytypes.StatusFailed
	}, 5*time.Second, 10*time.Millisecond)
	detail, err := activities.GetActivityDetail(ctx, "0", run.ActivityID, 10)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusFailed, detail.Activity.Status)
	require.Contains(t, detail.Activity.LatestMessage, "interrupted")
	persisted, err := restarted.runs.Get(ctx, "0", run.JobID, run.ID)
	require.NoError(t, err)
	require.Zero(t, persisted.AttemptCount, "repair must not execute interrupted work")
	require.Equal(t, scheduler.Failed, persisted.Status)
	require.Equal(t, "interrupted before applying updates", persisted.Outcome.Message)
	require.NotNil(t, persisted.FinishedAt)
	require.Equal(t, run.ActivityID, persisted.ActivityID)
}

func TestResolveRemoteRunConfirmsOwnerAfterLostResponse(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		name := "legacy operator"
		if automatic {
			name = "automatic settlement"
		}
		t.Run(name, func(t *testing.T) {
			db := setupSettingsTestDBInternal(t)
			require.NoError(t, db.AutoMigrate(&environment.Environment{}))
			q := newJobCoordinatorForTestInternal(t, db)
			local, err := q.Submit(t.Context(), scheduler.Request{JobID: "scheduled-prune", EnvironmentID: "remote", Trigger: "scheduled"})
			require.NoError(t, err)
			require.NoError(t, q.UpdateRun(t.Context(), local, func(run *scheduler.Run) error {
				run.Status = scheduler.NeedsAttention
				if automatic {
					run.Status = scheduler.Running
				}
				run.RemoteAccepted = true
				run.RemoteDeliveryAttempted = true
				run.Outcome = scheduler.Outcome{Status: scheduler.NeedsAttention, Message: "original failure"}
				return nil
			}))
			agentRun := scheduler.Run{
				ID:            local.ID,
				JobID:         local.JobID,
				EnvironmentID: "0",
				Status:        scheduler.Running,
				Outcome: scheduler.Outcome{
					Status:  scheduler.NeedsAttention,
					Message: "original failure",
					Targets: []scheduler.TargetOutcome{{
						ID:     "completed",
						Status: scheduler.Succeeded,
					}},
				},
			}
			mutations, acknowledgements := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/app-version" {
					require.Equal(t, "example", r.Header.Get("X-Arcane-Agent-Token"))
					require.NoError(t, json.MarshalWrite(w, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}))
					return
				}

				if r.URL.Path == "/api/environments/0/system/health" {
					require.Equal(t, http.MethodHead, r.Method)
					require.Equal(t, "example", r.Header.Get("X-Arcane-Agent-Token"))
					require.Equal(t, "example", r.Header.Get("X-API-Key"))
					w.WriteHeader(http.StatusOK)
					return
				}

				switch {
				case r.Method == http.MethodGet && r.URL.Query().Has("page"):
					raw, marshalErr := json.Marshal(scheduler.RunList{Runs: []scheduler.Run{agentRun}, Total: 1})
					assert.NoError(t, marshalErr)
					w.Header().Set("Content-Type", "application/json")
					_, writeErr := w.Write(raw)
					assert.NoError(t, writeErr)
					return
				case r.Method == http.MethodGet:
				case strings.HasSuffix(r.URL.Path, "/resolve"):
					mutations++
					var body struct {
						ResolvedBy string `json:"resolvedBy"`
					}
					assert.NoError(t, json.UnmarshalRead(r.Body, &body))
					agentRun.Status = scheduler.Canceled
					agentRun.Resolution = &scheduler.RunResolution{ResolvedBy: body.ResolvedBy, ResolvedAt: time.Now().UTC(), Reason: "resolved_after_review"}
					http.Error(w, "lost response", http.StatusServiceUnavailable)
					return
				case strings.HasSuffix(r.URL.Path, "/ack"):
					acknowledgements++
					agentRun.RemoteSettled = true
					if automatic && acknowledgements == 1 {
						http.Error(w, "lost acknowledgement", http.StatusServiceUnavailable)
						return
					}
				default:
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				raw, marshalErr := json.Marshal(agentRun)
				assert.NoError(t, marshalErr)
				w.Header().Set("Content-Type", "application/json")
				_, writeErr := w.Write(raw)
				assert.NoError(t, writeErr)
			}))
			defer server.Close()
			require.NoError(t, db.Create(&environment.Environment{ID: "remote", Name: "agent", ApiUrl: server.URL, AccessToken: new("example"), Enabled: true}).Error)
			environments := environment.NewEnvironmentService(db, server.Client(), nil, nil, nil, nil)
			svc := &JobService{runs: q, environment: environments, remote: remote.New(q, nil, environments)}
			ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.EnvironmentPermissionSet("remote"))
			path := "/api/environments/0/jobs/scheduled-prune/runs/" + local.ID
			if !automatic {
				_, err = svc.ResolveRun(ctx, "remote", local.JobID, local.ID, "operator")
				require.ErrorContains(t, err, "agent run must need attention")
			} else {
				outcome, confirmErr := svc.remote.Confirm(ctx, local, agentRun, path)
				require.NoError(t, confirmErr)
				require.Equal(t, scheduler.Waiting, outcome.Status)
			}
			require.Zero(t, mutations)
			agentRun.Status = scheduler.NeedsAttention
			if !automatic {
				_, err = svc.ResolveRun(ctx, "remote", local.JobID, local.ID, "operator")
				require.Error(t, err)
				unchanged, getErr := q.Get(t.Context(), "remote", local.JobID, local.ID)
				require.NoError(t, getErr)
				require.Equal(t, scheduler.NeedsAttention, unchanged.Status)
				resolved, resolveErr := svc.ResolveRun(ctx, "remote", local.JobID, local.ID, "operator")
				require.NoError(t, resolveErr)
				require.Equal(t, scheduler.Canceled, resolved.Status)
				require.True(t, resolved.RemoteSettled)
				require.Equal(t, "original failure", resolved.Outcome.Message)
			} else {
				outcome, confirmErr := svc.remote.Confirm(ctx, local, agentRun, path)
				require.Error(t, confirmErr)
				require.Equal(t, scheduler.Waiting, outcome.Status)
				outcome, confirmErr = svc.remote.Confirm(ctx, local, agentRun, path)
				require.Error(t, confirmErr)
				require.Equal(t, scheduler.Waiting, outcome.Status)
				outcome, confirmErr = svc.remote.Confirm(ctx, local, agentRun, path)
				require.NoError(t, confirmErr)
				require.Equal(t, scheduler.Failed, outcome.Status)
				require.Equal(t, "original failure", outcome.Message)
				require.Equal(t, scheduler.Succeeded, outcome.Targets[0].Status)
				stored, getErr := q.Get(t.Context(), "remote", local.JobID, local.ID)
				require.NoError(t, getErr)
				require.True(t, stored.RemoteSettled)
				require.Equal(t, scheduler.Failed, stored.RemoteOutcome.Status)
				require.Equal(t, user.SystemUser.Username, stored.Resolution.ResolvedBy)
				require.NoError(t, q.UpdateRun(ctx, stored, func(current *scheduler.Run) error {
					current.Status = scheduler.Failed
					return nil
				}))
				_, retryErr := svc.RetryRun(ctx, "remote", local.JobID, local.ID)
				require.ErrorContains(t, retryErr, "upgrade the agent")
				agentRun.Status = scheduler.Failed
				retried, retryErr := svc.RetryRun(ctx, "remote", local.JobID, local.ID)
				require.NoError(t, retryErr)
				require.Equal(t, scheduler.Queued, retried.Status)
				require.Equal(t, local.ID, retried.ID)

				// Adopt agent-owned history without coalescing its explicit identity.
				agentOwned := agentRun
				agentOwned.ID = "d7a9a3c7-d364-4c36-831a-381249e5727e"
				agentOwned.Status = scheduler.NeedsAttention
				agentOwned.CreatedAt = time.Now().UTC().Add(-time.Hour)
				agentOwned.FinishedAt = new(agentOwned.CreatedAt.Add(time.Minute))
				agentOwned.ActivityID = "agent-summary"
				agentOwned.RemoteSettled = false
				catalog := []jobschedule.JobStatus{{ID: "updates", Children: []jobschedule.JobStatus{{ID: agentOwned.JobID, LastRun: &agentOwned}}}}
				require.NoError(t, svc.remote.ReconcileLegacyCatalog(ctx, "remote", catalog))
				require.NoError(t, svc.remote.ReconcileLegacyCatalog(ctx, "remote", catalog))
				adopted, getErr := q.Get(ctx, "remote", agentOwned.JobID, agentOwned.ID)
				require.NoError(t, getErr)
				require.True(t, adopted.RemoteAccepted)
				require.True(t, adopted.RemoteDeliveryAttempted)
				require.Equal(t, agentOwned.CreatedAt, adopted.CreatedAt)
				require.Equal(t, scheduler.Queued, adopted.Status)
				require.Equal(t, agentOwned.FinishedAt, adopted.FinishedAt)
				require.Equal(t, agentOwned.ActivityID, adopted.ActivityID)
				require.Equal(t, "remote", adopted.ActivityEnvironmentID)
				status := jobschedule.JobStatus{LastRun: &agentOwned}
				runtime.ApplyRunStatus(&status, []scheduler.Run{adopted})
				require.Nil(t, status.CurrentRun)
				require.Equal(t, scheduler.Failed, status.LastRun.Status)
				require.Equal(t, "original failure", status.LastError)
				agentRun = agentOwned
				merged, listErr := svc.ListRuns(ctx, "remote", agentOwned.JobID, 1, 20)
				require.NoError(t, listErr)
				detail, detailErr := svc.GetRun(ctx, "remote", agentOwned.JobID, agentOwned.ID)
				require.NoError(t, detailErr)
				require.Equal(t, scheduler.Failed, detail.Status)
				require.Equal(t, "original failure", detail.Outcome.Message)
				require.Equal(t, agentOwned.ActivityID, detail.ActivityID)
				require.Equal(t, "remote", detail.ActivityEnvironmentID)
				require.Contains(t, merged.Runs, detail)
				history, listErr := q.List(ctx, "remote", agentOwned.JobID, 1, 20)
				require.NoError(t, listErr)
				require.Equal(t, 2, history.Total)
			}
			require.Equal(t, 1, mutations)
		})
	}
}

func TestResolveRunChecksCurrentOperatorPermission(t *testing.T) {
	db := setupSettingsTestDBInternal(t)
	svc := &JobService{runs: newJobCoordinatorForTestInternal(t, db)}
	run, err := svc.runs.Submit(t.Context(), scheduler.Request{JobID: "scheduled-prune", Trigger: "scheduled"})
	require.NoError(t, err)
	require.NoError(t, svc.runs.UpdateRun(t.Context(), run, func(current *scheduler.Run) error {
		current.Status = scheduler.NeedsAttention
		current.Outcome = scheduler.Outcome{Status: scheduler.NeedsAttention, Message: "interrupted"}
		return nil
	}))
	_, err = svc.ResolveRun(t.Context(), "0", "scheduled-prune", run.ID, "operator")
	require.ErrorContains(t, err, "permission denied")
	otherEnvironment := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.EnvironmentPermissionSet("different"))
	_, err = svc.ResolveRun(otherEnvironment, "0", "scheduled-prune", run.ID, "operator")
	require.ErrorContains(t, err, "permission denied")
	authorized := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.EnvironmentPermissionSet("0"))
	_, err = svc.ResolveRun(authorized, "0", "scheduled-prune", run.ID, "")
	require.ErrorContains(t, err, "identity")
	resolved, err := svc.ResolveRun(authorized, "0", "scheduled-prune", run.ID, "operator")
	require.NoError(t, err)
	require.Equal(t, scheduler.Canceled, resolved.Status)
	require.Equal(t, "interrupted", resolved.Outcome.Message)
	require.Equal(t, "operator", resolved.Resolution.ResolvedBy)
}

func TestBackupJobsRequireDockerOnlyForVolumeArchives(t *testing.T) {
	for _, test := range []struct {
		jobID    string
		required bool
	}{
		{"system-backup:daily", false},
		{"system-backup:volumes:daily", true},
		{"volume-backup:daily", true},
	} {
		t.Run(test.jobID, func(t *testing.T) { require.Equal(t, test.required, requiresDockerInternal(test.jobID)) })
	}
}
