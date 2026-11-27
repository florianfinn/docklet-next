// Source: docklet-next/issues/676
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original scheduler regression coverage.

package backup

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	sharedbackup "github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduledSystemBackupWaitsAcrossRestartAndCoalesces(t *testing.T) {
	db, cfg := archiveFixture(t)
	activities := activity.NewActivityService(db, nil)
	holder, err := activities.StartGatedActivity(t.Context(), activity.GateRequest{EnvironmentID: "0", Operation: "manual-holder", Scope: sharedbackup.SystemAdmissionScope, Type: activitytypes.TypeResourceAction, ResourceType: "system_backup", ResourceID: "holder", StartedBy: &user.SystemUser})
	require.NoError(t, err)
	policy := &dto.SystemBackupPolicy{ID: "daily", Enabled: true, RetentionCount: 7}
	var created atomic.Int32
	s := NewService(Dependencies{DB: db, Config: cfg, Activity: activities, Store: Store{
		Policy:        func(context.Context, string) (*dto.SystemBackupPolicy, error) { return policy, nil },
		CreateRun:     func(context.Context, *dto.SystemBackupRun) error { created.Add(1); return nil },
		SaveRun:       func(context.Context, *dto.SystemBackupRun) error { return nil },
		CommitRunTx:   func(*gorm.DB, *dto.SystemBackupRun, []string) error { return nil },
		ExpiredRunIDs: func(context.Context, string, int) ([]string, error) { return nil, nil },
	}})
	_, err = s.StartBackup(t.Context(), user.SystemUser, dto.CreateSystemBackupRequest{})
	require.ErrorIs(t, err, ErrSystemBackupAlreadyRunning)
	start := func() (*runs.Coordinator, func()) {
		runtime := francistest.New(t, cfg.DatabaseURL)
		q := runs.New(runtime.Service(), time.UTC)
		require.NoError(t, q.Register(runtime))
		francistest.Start(t, runtime)
		s.coordinator = q
		q.SetExecutor(func(ctx context.Context, _ scheduler.Run) (scheduler.Outcome, error) {
			return s.runScheduledBackup(ctx, policy.ID)
		}, nil)
		require.NoError(t, q.Start(t.Context(), t.Context()))
		q.Activate()
		return q, func() { require.NoError(t, q.Stop(t.Context())); require.NoError(t, runtime.Stop(t.Context())) }
	}
	q, stop := start()
	run, err := q.Submit(t.Context(), scheduler.Request{JobID: "system-backup:daily", Trigger: "scheduled"})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		r, e := q.Get(t.Context(), "0", run.JobID, run.ID)
		return e == nil && r.Status != scheduler.Running && r.Status != scheduler.Queued
	}, 5*time.Second, 10*time.Millisecond)
	waiting, err := q.Get(t.Context(), "0", run.JobID, run.ID)
	require.NoError(t, err)
	require.Equal(t, scheduler.Waiting, waiting.Status)
	require.Equal(t, "run.busy", waiting.Outcome.Code)
	coalesced, err := q.Submit(t.Context(), scheduler.Request{JobID: run.JobID, Trigger: "scheduled"})
	require.NoError(t, err)
	require.Equal(t, run.ID, coalesced.ID)
	require.Zero(t, created.Load())
	stop()
	q, stop = start()
	defer stop()
	stored, err := q.Get(t.Context(), "0", run.JobID, run.ID)
	require.NoError(t, err)
	require.Equal(t, scheduler.Waiting, stored.Status)
	_, err = activities.CompleteActivity(holder.WorkContext, holder.Run.ID, activitytypes.StatusSuccess, "done", nil)
	require.NoError(t, err)
	// Advance the persisted retry due time so shutdown latency cannot add another
	// backoff interval to this restart assertion.
	require.NoError(t, q.UpdateRun(t.Context(), stored, func(r *scheduler.Run) error { r.NextAttempt = nil; return nil }))
	q.Resume()
	require.Eventually(t, func() bool {
		r, e := q.Get(t.Context(), "0", run.JobID, run.ID)
		return e == nil && r.Status.Terminal()
	}, 10*time.Second, 10*time.Millisecond)
	completed, err := q.Get(t.Context(), "0", run.JobID, run.ID)
	require.NoError(t, err)
	require.Equal(t, scheduler.Succeeded, completed.Status, "%+v", completed.Outcome)
	require.Equal(t, int32(1), created.Load())
}
