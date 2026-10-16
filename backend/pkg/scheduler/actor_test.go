// Source: getarcaneapp/arcane/backend/pkg/scheduler/actor_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; SQLite-only local persistence.

package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/francis"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
)

var schedulerTestRuntimesInternal sync.Map

func newTestAdmissionGateInternal(t *testing.T) *runs.Admission {
	t.Helper()
	runtime := francistest.New(t)
	admission := runs.NewAdmission(runtime.Service(), t.Name())
	require.NoError(t, admission.Register(runtime))
	francistest.Start(t, runtime)
	return admission
}

func newTestCoordinatorInternal(t testing.TB,
	ctx context.Context,
	location *time.Location,
	execute func(context.Context,
		schedulertypes.Run) (schedulertypes.Outcome, error),
) (*runs.Coordinator, *francis.Runtime) {
	t.Helper()
	runtime := francistest.New(t)
	coordinator := runs.New(runtime.Service(), location)
	coordinator.SetExecutor(execute, nil)
	require.NoError(t, coordinator.Register(runtime))
	require.NoError(t, runtime.Start(t.Context(), ctx, nil))
	t.Cleanup(func() { require.NoError(t, runtime.Stop(context.WithoutCancel(t.Context()))) })
	require.NoError(t, coordinator.Start(t.Context(), ctx))
	coordinator.Activate()
	t.Cleanup(func() { require.NoError(t, coordinator.Stop(context.WithoutCancel(t.Context()))) })
	return coordinator, runtime
}

func newJobSchedulerForTestInternal(t testing.TB, ctx context.Context, location *time.Location) *jobSchedulerInternal {
	t.Helper()
	var scheduler *jobSchedulerInternal
	coordinator, runtime := newTestCoordinatorInternal(t, ctx, location, func(ctx context.Context, run schedulertypes.Run) (schedulertypes.Outcome, error) {
		job, ok := scheduler.GetJob(run.JobID)
		if !ok {
			return schedulertypes.Outcome{Status: schedulertypes.Skipped}, nil
		}
		return job.Run(jobcontext.WithExecution(ctx, run, nil))
	})
	created, err := NewJobScheduler(ctx, coordinator, location)
	require.NoError(t, err)
	scheduler = created.(*jobSchedulerInternal)
	schedulerTestRuntimesInternal.Store(scheduler, runtime)
	t.Cleanup(func() {
		require.NoError(t, scheduler.Stop(context.WithoutCancel(t.Context())))
		schedulerTestRuntimesInternal.Delete(scheduler)
	})
	return scheduler
}

func stopJobSchedulerForTestInternal(ctx context.Context, scheduler *jobSchedulerInternal) error {
	if err := scheduler.Stop(ctx); err != nil {
		return err
	}
	if err := scheduler.coordinator.Stop(ctx); err != nil {
		return err
	}
	runtime, _ := schedulerTestRuntimesInternal.Load(scheduler)
	return runtime.(*francis.Runtime).Stop(ctx)
}

func newSettingsServiceForTestInternal(t testing.TB, ctx context.Context, db *database.DB) (*settings.SettingsService, error) {
	t.Helper()
	svc, err := settings.NewSettingsService(ctx, db)
	if err == nil {
		t.Cleanup(func() { require.NoError(t, svc.Stop(context.WithoutCancel(t.Context()))) })
	}
	return svc, err
}
