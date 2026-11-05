// Source: docklet-next/issues/319
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original real scheduler live-zone, running-job and startup proofs.

package scheduler

import (
	"context"
	scheduleutil "github.com/florianfinn/docklet-next/backend/pkg/scheduler/schedule"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerLiveZoneReinstallKeepsRunningRun(t *testing.T) {
	js := newJobSchedulerForTestInternal(t, t.Context(), time.UTC)
	var zone atomic.Pointer[time.Location]
	zone.Store(time.UTC)
	js.SetZoneProvider(scheduleutil.ZoneFunc(func() *time.Location { return zone.Load() }))
	now := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	js.now = func() time.Time { return now }
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	job := &testSchedulerJob{name: "zone-proof", schedule: "0 0 4 * * *", run: func(context.Context) { close(entered); <-release }}
	require.NoError(t, js.RegisterJob(job))
	require.NoError(t, js.StartScheduler(t.Context()))
	before, e := js.coordinator.ScheduleState(t.Context(), job.Name())
	require.NoError(t, e)
	require.Equal(t, "UTC", before.Zone)
	manual, e := js.Submit(t.Context(), st.Request{JobID: job.Name(), Trigger: "manual"})
	require.NoError(t, e)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("manual run did not enter")
	}
	tokyo, e := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, e)
	zone.Store(tokyo)
	require.NoError(t, js.Reinstall(t.Context()))
	after, e := js.coordinator.ScheduleState(t.Context(), job.Name())
	require.NoError(t, e)
	require.Equal(t, "Asia/Tokyo", after.Zone)
	require.Greater(t, after.Generation, before.Generation)
	require.True(t, time.Date(2027, 1, 1, 19, 0, 0, 0, time.UTC).Equal(after.NextRun))
	require.Len(t, after.Runs, 1)
	require.Equal(t, manual.ID, after.Runs[0].ID)
	require.Equal(t, st.Running, after.Runs[0].Status)
	once.Do(func() { close(release) })
	require.Eventually(t, func() bool {
		run, e := js.coordinator.Get(t.Context(), "0", job.Name(), manual.ID)
		return e == nil && run.Status == st.Succeeded
	}, 5*time.Second, 5*time.Millisecond)
	// An ordinary schedule resave still goes through the same parse/checkpoint path.
	job.schedule = "0 30 4 * * *"
	require.NoError(t, js.RescheduleJob(t.Context(), job))
	state, _ := js.GetJobRuntimeState(job.Name())
	require.Equal(t, job.schedule, state.Schedule)
}

func TestSchedulerStartupUsesCurrentProviderAfterSkippedReinstall(t *testing.T) {
	js := newJobSchedulerForTestInternal(t, t.Context(), time.UTC)
	now := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	js.now = func() time.Time { return now }
	tokyo, e := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, e)
	// A persisted settings provider is bound before startup, even when the previous
	// process died between committing settings and sending its reinstall event.
	js.SetZoneProvider(scheduleutil.FixedZone(tokyo))
	job := &testSchedulerJob{name: "restart-zone", schedule: "0 0 4 * * *"}
	require.NoError(t, js.RegisterJob(job))
	require.NoError(t, js.StartScheduler(t.Context()))
	state, e := js.coordinator.ScheduleState(t.Context(), job.Name())
	require.NoError(t, e)
	require.Equal(t, "Asia/Tokyo", state.Zone)
	require.True(t, time.Date(2027, 1, 1, 19, 0, 0, 0, time.UTC).Equal(state.NextRun))
	require.Empty(t, state.Runs)
}
