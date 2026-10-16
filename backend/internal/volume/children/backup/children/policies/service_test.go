// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/children/policies/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; SQLite temp-file persistence fixture.

package policies

import (
	"context"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
)

func noLatestRunInternal(context.Context, string) (*volume.BackupEntry, error) {
	return nil, nil
}

func newVolumeAdmissionForTestInternal(t testing.TB) *runs.Admission {
	t.Helper()
	runtime := francistest.New(t)
	gate := runs.NewAdmission(runtime.Service(), t.Name())
	require.NoError(t, gate.Register(runtime))
	francistest.Start(t, runtime)
	return gate
}

type volumeBackupPolicySchedulerInternal struct {
	submitted []scheduler.Request
	jobs      map[string]scheduler.Job
}

func (s *volumeBackupPolicySchedulerInternal) AddJob(_ context.Context, job scheduler.Job) error {
	s.jobs[job.Name()] = job
	return nil
}

func (s *volumeBackupPolicySchedulerInternal) RemoveJob(_ context.Context, name string) {
	delete(s.jobs, name)
}

func (s *volumeBackupPolicySchedulerInternal) HasJob(name string) bool {
	_, ok := s.jobs[name]
	return ok
}

func TestVolumeBackupPolicy_UpdateRegistersIndependentJobsAndSettings(t *testing.T) {
	gormDB, err := gorm.Open(testsqlite.Dialector(t, "file:volume-backup-schedule?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(&VolumeBackupPolicy{}))
	db := &database.DB{DB: gormDB}
	jobScheduler := &volumeBackupPolicySchedulerInternal{jobs: make(map[string]scheduler.Job)}
	service := NewService(Dependencies{DB: db, LatestRun: noLatestRunInternal})
	gate := newVolumeAdmissionForTestInternal(t)
	require.NoError(t, service.SetScheduler(t.Context(), jobScheduler, gate))

	collection, err := service.UpdateBackupPolicies(t.Context(), "app-data", []volume.UpdateBackupPolicy{
		{Enabled: true, Schedule: "0 */15 * * * *", RetentionCount: 5, StopContainers: true, LocalEnabled: true},
		{Enabled: true, Schedule: "0 0 2 * * *", RetentionCount: 30, LocalEnabled: true},
	})
	require.NoError(t, err)
	require.Len(t, collection.Policies, 2)
	require.Equal(t, 5, collection.Policies[0].RetentionCount)
	require.True(t, collection.Policies[0].StopContainers)
	require.Equal(t, 30, collection.Policies[1].RetentionCount)
	require.Len(t, jobScheduler.jobs, 2)

	firstID := collection.Policies[0].ID
	collection, err = service.UpdateBackupPolicies(t.Context(), "app-data", []volume.UpdateBackupPolicy{{
		ID: firstID, Schedule: "0 */30 * * * *", RetentionCount: 9, LocalEnabled: true,
	}})
	require.NoError(t, err)
	require.Len(t, collection.Policies, 1)
	require.Equal(t, firstID, collection.Policies[0].ID)
	require.Equal(t, 9, collection.Policies[0].RetentionCount)
	require.Empty(t, jobScheduler.jobs)
}

func TestVolumeBackupPolicy_GetReturnsLastRunForEachPolicy(t *testing.T) {
	gormDB, err := gorm.Open(testsqlite.Dialector(t, "file:volume-backup-last-runs?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(&VolumeBackupPolicy{}))
	first := &VolumeBackupPolicy{VolumeName: "app-data", Schedule: "0 0 2 * * *", LocalEnabled: true}
	second := &VolumeBackupPolicy{VolumeName: "app-data", Schedule: "0 0 14 * * *", LocalEnabled: true}
	require.NoError(t, gormDB.Create(first).Error)
	require.NoError(t, gormDB.Create(second).Error)
	latest := map[string]*volume.BackupEntry{
		first.ID:  {VolumeName: "app-data", PolicyID: first.ID, Status: backup.VolumeBackupStatusSucceeded},
		second.ID: {VolumeName: "app-data", PolicyID: second.ID, Status: backup.VolumeBackupStatusFailed},
	}

	service := NewService(Dependencies{
		DB: &database.DB{DB: gormDB},
		LatestRun: func(_ context.Context, policyID string) (*volume.BackupEntry, error) {
			return latest[policyID], nil
		},
	})
	collection, err := service.GetBackupPolicies(t.Context(), "app-data")
	require.NoError(t, err)
	require.Len(t, collection.Policies, 2)
	require.Equal(t, "succeeded", collection.Policies[0].LastRun.Status)
	require.Equal(t, "failed", collection.Policies[1].LastRun.Status)
}

func (s *volumeBackupPolicySchedulerInternal) Submit(_ context.Context, request scheduler.Request) (scheduler.Run, error) {
	s.submitted = append(s.submitted, request)
	return scheduler.Run{ID: request.RunID, JobID: request.JobID, EnvironmentID: request.EnvironmentID, Status: scheduler.Queued}, nil
}
