// Source: getarcaneapp/arcane/backend/pkg/scheduler/scheduler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; M18 live scheduling, policy and contract foundation.

package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	st "github.com/florianfinn/docklet-next/types/scheduler"

	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	scheduleutil "github.com/florianfinn/docklet-next/backend/pkg/scheduler/schedule"
)

var (
	errJobSchedulerStoppedInternal = errors.New("job scheduler stopped")
)

type watcherRegistrationInternal struct {
	watcher st.BusWatcher
	done    chan struct{}
}
type jobSchedulerInternal struct {
	mu          sync.RWMutex
	jobsByID    map[string]st.Job
	watchers    map[string]st.BusWatcher
	allWatchers map[string]watcherRegistrationInternal
	supervisors map[string]*watcherSupervisorInternal
	context     context.Context
	cancel      context.CancelFunc
	zones       scheduleutil.ZoneProvider
	coordinator *runs.Coordinator
	stopping    bool
	started     bool
	now         func() time.Time
}

func NewJobScheduler(ctx context.Context, coordinator *runs.Coordinator, location *time.Location) (st.JobScheduler, error) {
	if ctx == nil || coordinator == nil {
		return nil, errors.New("scheduler dependencies unavailable")
	}
	if location == nil {
		location = time.UTC
	}
	lifetime, cancel := context.WithCancel(ctx)
	return &jobSchedulerInternal{
			context:     lifetime,
			cancel:      cancel,
			zones:       scheduleutil.FixedZone(location),
			coordinator: coordinator,
			jobsByID:    map[string]st.Job{},
			watchers:    map[string]st.BusWatcher{},
			allWatchers: map[string]watcherRegistrationInternal{},
			supervisors: map[string]*watcherSupervisorInternal{},
		},
		nil
}

func (js *jobSchedulerInternal) RegisterJob(job st.Job) error {
	if job == nil {
		return errors.New("scheduler job unavailable")
	}
	js.mu.Lock()
	defer js.mu.Unlock()
	if js.stopping {
		return errJobSchedulerStoppedInternal
	}
	js.jobsByID[job.Name()] = job
	return nil
}

func (js *jobSchedulerInternal) RegisterBusWatcher(watcher st.BusWatcher, manual bool) error {
	js.mu.Lock()
	defer js.mu.Unlock()
	if js.stopping {
		return errJobSchedulerStoppedInternal
	}
	if _, found := js.allWatchers[watcher.Name()]; found {
		return errors.New("watcher already registered")
	}
	supervisor := newWatcherSupervisorInternal(watcher)
	done := make(chan struct{})
	js.supervisors[watcher.Name()] = supervisor
	js.allWatchers[watcher.Name()] = watcherRegistrationInternal{watcher: watcher, done: done}
	if manual {
		js.watchers[watcher.Name()] = watcher
	}
	go func() {
		defer close(done)
		if err := supervisor.runInternal(js.context); err != nil && js.context.Err() == nil {
			slog.ErrorContext(js.context, "Watcher stopped", "name", watcher.Name(), "error", err)
		}
	}()
	return nil
}

func (js *jobSchedulerInternal) RunBusWatcherNow(ctx context.Context, id string) error {
	js.mu.RLock()
	watcher, found := js.watchers[id]
	js.mu.RUnlock()
	if !found {
		return errors.New("watcher is not manually runnable")
	}
	return watcher.RunNow(ctx)
}

func (js *jobSchedulerInternal) GetJob(id string) (st.Job, bool) {
	js.mu.RLock()
	defer js.mu.RUnlock()
	job, ok := js.jobsByID[id]
	return job, ok
}
func (js *jobSchedulerInternal) HasJob(id string) bool { _, ok := js.GetJob(id); return ok }
func (js *jobSchedulerInternal) GetJobRuntimeState(id string) (st.JobRuntimeState, bool) {
	if !js.HasJob(id) {
		return st.JobRuntimeState{}, false
	}
	record, err := js.coordinator.ScheduleState(js.context, id)
	if err != nil {
		return st.JobRuntimeState{}, true
	}
	state := st.JobRuntimeState{Schedule: record.Schedule, Scheduled: !record.NextRun.IsZero()}
	if state.Scheduled {
		state.NextRun = new(record.NextRun)
	}
	return state, true
}

func (js *jobSchedulerInternal) StartScheduler(ctx context.Context) error {
	js.mu.Lock()
	if js.stopping {
		js.mu.Unlock()
		return errJobSchedulerStoppedInternal
	}
	js.started = true
	js.mu.Unlock()
	records, err := js.coordinator.Records(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		js.mu.RLock()
		_, watching := js.watchers[record.JobID]
		js.mu.RUnlock()
		if record.EnvironmentID == "0" && record.Schedule != "" && !js.HasJob(record.JobID) && !watching {
			if checkpointErr := js.coordinator.Checkpoint(ctx, record.JobID, "", js.GetLocation().String(), time.Time{}); checkpointErr != nil {
				return checkpointErr
			}
		}
	}
	var schedulingErr error
	for _, job := range js.ListRegisteredJobs() {
		if installErr := js.installInternal(ctx, job); installErr != nil {
			schedulingErr = errors.Join(schedulingErr, fmt.Errorf("%s: %w", "schedule "+job.Name(), installErr))
		}
	}
	return schedulingErr
}

func (js *jobSchedulerInternal) installInternal(ctx context.Context, job st.Job) error {
	next := time.Time{}
	location := js.GetLocation()
	schedule := job.Schedule(ctx)
	should := true
	if conditional, ok := job.(st.ConditionalJob); ok {
		should = conditional.ShouldSchedule(ctx)
	}
	if should {
		parsed, err := scheduleutil.Parse(schedule, location)
		if err != nil {
			return err
		}
		now := time.Now()
		if js.now != nil {
			now = js.now()
		}
		next = parsed.Next(now)
	}
	if !should {
		schedule = ""
	}
	return js.coordinator.Checkpoint(ctx, job.Name(), schedule, location.String(), next)
}

func (js *jobSchedulerInternal) AddJob(ctx context.Context, job st.Job) error {
	if job == nil {
		return errors.New("scheduler job unavailable")
	}
	js.mu.Lock()
	defer js.mu.Unlock()
	if js.stopping {
		return errJobSchedulerStoppedInternal
	}
	js.jobsByID[job.Name()] = job
	if !js.started {
		return nil
	}
	return js.installInternal(ctx, job)
}

func (js *jobSchedulerInternal) RescheduleJob(ctx context.Context, job st.Job) error {
	return js.AddJob(ctx, job)
}

func (js *jobSchedulerInternal) RemoveJob(ctx context.Context, id string) {
	js.mu.Lock()
	defer js.mu.Unlock()
	delete(js.jobsByID, id)
	started := js.started
	if started {
		if err := js.coordinator.Checkpoint(ctx, id, "", js.GetLocation().String(), time.Time{}); err != nil {
			slog.ErrorContext(ctx, "Unschedule failed", "job", id, "error", err)
		}
	}
}
func (js *jobSchedulerInternal) GetLocation() *time.Location { return js.zones.Location() }

func (js *jobSchedulerInternal) SetZoneProvider(provider scheduleutil.ZoneProvider) {
	js.mu.Lock()
	defer js.mu.Unlock()
	js.zones = provider
}

// Reinstall moves future occurrences together; queued and running work retains
// its existing ownership and is never caught up because of a zone change.
func (js *jobSchedulerInternal) Reinstall(ctx context.Context) error {
	js.mu.Lock()
	defer js.mu.Unlock()
	var err error
	for _, job := range js.jobsByID {
		err = errors.Join(err, js.installInternal(ctx, job))
	}
	return err
}
func (js *jobSchedulerInternal) Stop(ctx context.Context) error {
	js.mu.Lock()
	js.stopping = true
	js.cancel()
	watchers := make([]watcherRegistrationInternal, 0, len(js.allWatchers))
	for _, w := range js.allWatchers {
		watchers = append(watchers, w)
	}
	js.mu.Unlock()
	var err error
	for _, w := range watchers {
		select {
		case <-w.done:
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		}
		if stop, ok := w.watcher.(st.StoppableBusWatcher); ok {
			err = errors.Join(err, stop.Stop(ctx))
		}
	}
	return err
}
