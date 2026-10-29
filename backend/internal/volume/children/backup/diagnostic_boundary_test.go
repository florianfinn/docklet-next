// Source: docklet-next/issues/284
// Revision: 990c3ddd5521c84e81558dd5a2acbbc396312c38
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original backup policy and repository diagnostic boundary tests.

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	jobruntime "github.com/florianfinn/docklet-next/backend/internal/job/children/runtime"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/types/jobschedule"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/volume"
)

const boundarySentinelInternal = "synthetic-query-diagnostic"

// failTableInternal makes every query and row scan on table fail with the sentinel.
func failTableInternal(t *testing.T, db *gorm.DB, table string) *bool {
	t.Helper()
	reached := false
	fail := func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			reached = true
			tx.AddError(errors.New(boundarySentinelInternal))
		}
	}
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:fail-"+table, fail))
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("test:fail-"+table, fail))
	return &reached
}

func TestScheduledPolicyLookupFailureKeepsRunHistoryPrivate(t *testing.T) {
	f := newRuntimePlan(t)
	db := f.service.deps.DB
	require.NoError(t, db.AutoMigrate(&policies.VolumeBackupPolicy{}))
	policy := policies.VolumeBackupPolicy{VolumeName: "data", Enabled: true, Schedule: "0 0 2 * * *", LocalEnabled: true, TargetID: "local"}
	require.NoError(t, db.Create(&policy).Error)
	jobs := &volumeBackupPolicySchedulerInternal{jobs: map[string]scheduler.Job{}}
	require.NoError(t, f.service.SetScheduler(t.Context(), jobs, newVolumeAdmissionForTestInternal(t)))
	f.service.RegisterJobsOnStartup(t.Context())
	job, ok := jobs.jobs["volume-backup:"+policy.ID]
	require.True(t, ok)
	reached := failTableInternal(t, db.DB, "volume_backup_policies")

	runtime := francistest.New(t)
	coordinator := runs.New(runtime.Service(), time.UTC)
	require.NoError(t, coordinator.Register(runtime))
	francistest.Start(t, runtime)
	coordinator.SetExecutor(func(ctx context.Context, _ scheduler.Run) (scheduler.Outcome, error) { return job.Run(ctx) }, nil)
	submitted, err := coordinator.Submit(t.Context(), scheduler.Request{JobID: job.Name(), EnvironmentID: "0", Trigger: "manual"})
	require.NoError(t, err)
	require.NoError(t, coordinator.Start(t.Context(), t.Context()))
	coordinator.Activate()
	defer func() { require.NoError(t, coordinator.Stop(context.WithoutCancel(t.Context()))) }()
	var stored scheduler.Run
	require.Eventually(t, func() bool {
		var getErr error
		stored, getErr = coordinator.Get(t.Context(), "0", job.Name(), submitted.ID)
		return getErr == nil && stored.Status.Terminal()
	}, 5*time.Second, 10*time.Millisecond)

	require.True(t, *reached)
	require.Equal(t, scheduler.Failed, stored.Status)
	status := jobschedule.JobStatus{ID: job.Name()}
	jobruntime.ApplyRunStatus(&status, []scheduler.Run{stored})
	wire, err := json.Marshal(status)
	require.NoError(t, err)
	require.Contains(t, string(wire), "volume.backup.interrupted")
	require.NotContains(t, string(wire), boundarySentinelInternal)
}

func TestPolicyHandlersKeepStorageDiagnosticsPrivate(t *testing.T) {
	for _, operation := range []string{"read", "update"} {
		t.Run(operation, func(t *testing.T) {
			f := newRuntimePlan(t)
			db := f.service.deps.DB
			require.NoError(t, db.AutoMigrate(&policies.VolumeBackupPolicy{}))
			require.NoError(t, db.AutoMigrate(activity.Models()...))
			activities := activity.NewActivityService(db, nil)
			defer activities.CancelAndDrain(t.Context(), "")
			handler := policies.NewHandler(f.service.policies, activities, nil, t.Context())
			reached := failTableInternal(t, db.DB, "volume_backup_policies")

			var err error
			if operation == "read" {
				_, err = handler.GetBackupPolicy(t.Context(), &policies.GetVolumeBackupPolicyInput{EnvironmentID: "0", VolumeName: "data"})
			} else {
				_, err = handler.UpdateBackupPolicy(t.Context(), &policies.UpdateVolumeBackupPolicyInput{EnvironmentID: "0", VolumeName: "data", Body: volume.UpdateBackupPolicies{}})
			}

			require.True(t, *reached)
			var status huma.StatusError
			require.ErrorAs(t, err, &status)
			require.Equal(t, 500, status.GetStatus())
			require.Equal(t, "volume.backup.interrupted", err.Error())
			wire, marshalErr := json.Marshal(err)
			require.NoError(t, marshalErr)
			require.NotContains(t, string(wire), boundarySentinelInternal)
			if operation == "update" {
				var records []activity.Activity
				require.NoError(t, db.Where("resource_type = ?", "volume_backup_policy").Find(&records).Error)
				require.Len(t, records, 1)
				detail, readErr := activities.GetActivityDetail(t.Context(), "0", records[0].ID, 10)
				require.NoError(t, readErr)
				activityWire, encodeErr := json.Marshal(detail)
				require.NoError(t, encodeErr)
				require.NotContains(t, string(activityWire), boundarySentinelInternal)
			}
		})
	}
}

func TestRepositoryMetadataKeepsStorageDiagnosticsPrivate(t *testing.T) {
	for _, table := range []string{"settings", "activities"} {
		t.Run(table, func(t *testing.T) {
			f := newRuntimePlan(t)
			db := f.service.deps.DB
			require.NoError(t, db.AutoMigrate(activity.Models()...))
			handler := &Handler{service: f.service}
			_, err := handler.Repository(t.Context(), &HostBackupInput{EnvironmentID: "0"})
			require.NoError(t, err)
			reached := failTableInternal(t, db.DB, table)
			_, api := humatest.New(t)
			huma.Register(api, huma.Operation{OperationID: "test-repository", Method: "GET", Path: "/environments/{id}/repository"}, handler.Repository)

			response := api.Get("/environments/0/repository")

			require.True(t, *reached)
			require.Equal(t, 500, response.Code)
			require.Contains(t, response.Body.String(), "volume.backup.engine-unavailable")
			require.NotContains(t, response.Body.String(), boundarySentinelInternal)
		})
	}
}
