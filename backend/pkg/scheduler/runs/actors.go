// Source: getarcaneapp/arcane/backend/pkg/scheduler/runs/actors.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; SQLite-only local persistence.

package runs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"uuid"

	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/italypaleale/francis/actor"
	"github.com/italypaleale/francis/host/local"
	"github.com/robfig/cron/v3"
	kit "go.getarcane.app/kit/pkg"

	scheduleutil "github.com/florianfinn/docklet-next/backend/pkg/scheduler/schedule"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/francis"
)

const (
	coordinatorTypeInternal    = "job-coordinator"
	executorTypeInternal       = "job-executor"
	healthExecutorTypeInternal = "job-health-executor"
)

type coordinatorActorInternal struct {
	id string
	q  *Coordinator
}
type executorActorInternal struct {
	q *Coordinator
}

func (q *Coordinator) Register(runtime *francis.Runtime) error {
	q.runtime = runtime
	if err := runtime.RegisterActor(coordinatorTypeInternal, func(id string, _ *actor.Service) actor.Actor {
		return &coordinatorActorInternal{id: id, q: q}
	}); err != nil {
		return err
	}
	executorFactory := func(_ string, _ *actor.Service) actor.Actor {
		return &executorActorInternal{q: q}
	}
	if err := runtime.RegisterActor(
		executorTypeInternal, executorFactory,
		local.WithCapacityGroup("jobs", 4),
		local.WithCompletedJobRetention(7*24*time.Hour),
	); err != nil {
		return err
	}
	return runtime.RegisterActor(
		healthExecutorTypeInternal, executorFactory,
		local.WithCapacityGroup("jobs-health", 2),
		local.WithCompletedJobRetention(7*24*time.Hour),
	)
}

func (a *coordinatorActorInternal) Invoke(ctx context.Context, method string, data actor.Envelope) (any, error) {
	var state st.CoordinatorState
	if err := a.q.service.GetState(ctx, coordinatorTypeInternal, a.id, &state); err != nil && !errors.Is(err, actor.ErrStateNotFound) {
		return nil, err
	}
	normalizeRecordTimesInternal(&state.Record)
	switch method {
	case "replace":
		var next st.CoordinatorState
		if err := data.Decode(&next); err != nil {
			return nil, err
		}
		if next.Revision != state.Revision {
			return false, nil
		}
		next.Revision++
		if err := a.saveStateInternal(ctx, next); err != nil {
			return nil, err
		}
		// Acceptance is persisted; dispatch failures are retried.
		if err := a.flushInternal(ctx, &next); err != nil {
			slog.WarnContext(ctx, "Job dispatch requires reconciliation", "jobId", next.Record.JobID, "environmentId", next.Record.EnvironmentID, "error", err)
			a.q.signalInternal()
		}
		return true, nil
	case "activity-synced":
		var command st.ActivitySyncCommand
		if err := data.Decode(&command); err != nil {
			return nil, err
		}
		version, pending := state.PendingActivitySync[command.RunID]
		if !pending || !version.Equal(command.UpdatedAt) {
			return nil, nil
		}
		delete(state.PendingActivitySync, command.RunID)
		state.Revision++
		return nil, a.saveStateInternal(ctx, state)
	case "repair":
		return nil, a.flushInternal(ctx, &state)
	default:
		return nil, fmt.Errorf("unknown coordinator command: %s", method)
	}
}

func (a *coordinatorActorInternal) flushInternal(ctx context.Context, state *st.CoordinatorState) error {
	if !a.q.enabled.Load() {
		return nil
	}
	record := &state.Record
	if record.Schedule != "" && !record.NextRun.IsZero() {
		name := fmt.Sprintf("schedule-%d-%d", record.Generation, record.Sequence)
		properties := actor.AlarmProperties{
			DueTime: record.NextRun,
			Data:    st.ScheduleOccurrence{Generation: record.Generation, Sequence: record.Sequence},
		}
		if err := a.q.service.SetAlarm(ctx, coordinatorTypeInternal, a.id, name, properties); err != nil {
			return err
		}
	}
	var selected *st.Run
	for i := range record.Runs {
		if record.Runs[i].Status == st.Running {
			selected = &record.Runs[i]
			break
		}
	}
	var retryAt time.Time
	if selected == nil {
		selected, retryAt = a.q.nextRunInternal(record.Runs)
	}
	if selected == nil {
		if retryAt.IsZero() {
			return nil
		}
		name := fmt.Sprintf("retry-%d", retryAt.UnixNano())
		return a.q.service.SetAlarm(ctx, coordinatorTypeInternal, a.id, name, actor.AlarmProperties{DueTime: retryAt})
	}
	if selected.Status == st.Running {
		active, err := a.reconcileDispatchInternal(ctx, state, *selected)
		if err != nil {
			return err
		}
		if active {
			return nil
		}
	}
	return a.dispatchRunInternal(ctx, state, *selected, selected.AttemptCount+1)
}

// reconcileDispatchInternal recovers a lost job ID before considering redelivery.
func (a *coordinatorActorInternal) reconcileDispatchInternal(ctx context.Context, state *st.CoordinatorState, run st.Run) (bool, error) {
	key := fmt.Sprintf("%s-%d", run.ID, run.AttemptCount)
	previous, exists := state.Dispatches[key]
	if !exists {
		return false, nil
	}
	if previous.JobID == "" {
		command := st.ExecutionCommand{
			EnvironmentID: run.EnvironmentID,
			JobID:         run.JobID,
			RunID:         run.ID,
			Attempt:       run.AttemptCount,
		}
		actorType := executorTypeInternal
		if strings.HasPrefix(run.JobID, "environment-health") || run.JobID == "docker-client-refresh" || run.JobID == "activity-sweep" {
			actorType = healthExecutorTypeInternal
		}
		id, _, err := a.q.service.Dispatch(ctx, actorType, a.id, "execute", command, actor.WithIdempotencyKey(key))
		if err != nil {
			return false, err
		}
		previous.JobID = id
		state.Dispatches[key] = previous
		state.Revision++
		if setStateErr := a.saveStateInternal(ctx, *state); setStateErr != nil {
			return false, setStateErr
		}
	}
	job, err := a.q.service.GetJob(ctx, previous.JobID)
	if errors.Is(err, actor.ErrJobNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !job.Status.IsTerminal(), nil
}

func (a *coordinatorActorInternal) dispatchRunInternal(ctx context.Context, state *st.CoordinatorState, run st.Run, attempt int) error {
	key := fmt.Sprintf("%s-%d", run.ID, attempt)
	if state.Dispatches == nil {
		state.Dispatches = make(map[string]st.DispatchIntent)
	}
	actorType := executorTypeInternal
	if strings.HasPrefix(run.JobID, "environment-health") || run.JobID == "docker-client-refresh" || run.JobID == "activity-sweep" {
		actorType = healthExecutorTypeInternal
	}
	intent := state.Dispatches[key]
	if intent.JobID != "" {
		job, err := a.q.service.GetJob(ctx, intent.JobID)
		if err == nil && !job.Status.IsTerminal() {
			return nil
		}
		if err != nil && !errors.Is(err, actor.ErrJobNotFound) {
			return err
		}
		if err == nil {
			if deleteJobErr := a.q.service.DeleteJob(ctx, actorType, a.id, intent.JobID); deleteJobErr != nil && !errors.Is(deleteJobErr, actor.ErrJobNotFound) {
				return deleteJobErr
			}
		}
	}
	intent = st.DispatchIntent{RunID: run.ID, Attempt: attempt}
	state.Dispatches[key] = intent
	state.Revision++
	if err := a.saveStateInternal(ctx, *state); err != nil {
		return err
	}
	options := []actor.JobOption{actor.WithIdempotencyKey(key)}
	if run.NextAttempt != nil && run.NextAttempt.After(time.Now()) {
		options = append(options, actor.WithJobDueTime(*run.NextAttempt))
	}
	command := st.ExecutionCommand{
		EnvironmentID: run.EnvironmentID,
		JobID:         run.JobID,
		RunID:         run.ID,
		Attempt:       attempt,
	}
	id, _, err := a.q.service.Dispatch(ctx, actorType, a.id, "execute", command, options...)
	if err != nil {
		return err
	}
	job, err := a.q.service.GetJob(ctx, id)
	if err != nil {
		return err
	}
	if job.Status.IsTerminal() {
		if deleteJobErr2 := a.q.service.DeleteJob(ctx, actorType, a.id, id); deleteJobErr2 != nil && !errors.Is(deleteJobErr2, actor.ErrJobNotFound) {
			return deleteJobErr2
		}
		id, _, err = a.q.service.Dispatch(ctx, actorType, a.id, "execute", command, options...)
		if err != nil {
			return err
		}
	}
	intent.JobID = id
	state.Dispatches[key] = intent
	state.Revision++
	return a.saveStateInternal(ctx, *state)
}

func (a *coordinatorActorInternal) Alarm(ctx context.Context, name string, data actor.Envelope) error {
	if !a.q.enabled.Load() {
		return nil
	}
	var state st.CoordinatorState
	if err := a.q.service.GetState(ctx, coordinatorTypeInternal, a.id, &state); err != nil && !errors.Is(err, actor.ErrStateNotFound) {
		return err
	}
	normalizeRecordTimesInternal(&state.Record)
	if strings.HasPrefix(name, "retry-") {
		return a.flushInternal(ctx, &state)
	}
	var occurrence st.ScheduleOccurrence
	if err := data.Decode(&occurrence); err != nil {
		return err
	}
	record := &state.Record
	if record.Schedule == "" {
		return nil
	}
	if occurrence.Generation != record.Generation || occurrence.Sequence != record.Sequence {
		return nil
	}
	now := time.Now().UTC()
	schedule, err := scheduleutil.Parser().Parse(record.Schedule)
	if err != nil {
		return err
	}
	if spec, ok := schedule.(*cron.SpecSchedule); ok {
		spec.Location = a.q.location
	}
	trigger := "scheduled"
	nextOccurrence := schedule.Next(record.NextRun.In(a.q.location))
	if !nextOccurrence.After(now) {
		trigger = "recovery"
	}
	pending := false
	for _, run := range record.Runs {
		if separateTriggerInternal(run.Trigger) {
			continue
		}
		if run.Status.Terminal() || run.Status == st.NeedsAttention {
			continue
		}
		pending = true
		break
	}
	if !pending {
		run := st.Run{
			ID:            uuid.New().String(),
			EnvironmentID: record.EnvironmentID,
			JobID:         record.JobID,
			Trigger:       trigger,
			Status:        st.Queued,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		a.q.associateActivityInternal(&run)
		record.Runs = append(record.Runs, run)
		markActivitySyncInternal(&state, run)
	}
	record.LastEnqueuedAt = now
	record.NextRun = schedule.Next(now.In(a.q.location))
	record.Sequence++
	state.Revision++
	if setStateErr := a.saveStateInternal(ctx, state); setStateErr != nil {
		return setStateErr
	}
	return a.flushInternal(ctx, &state)
}

// repairInternal flushes coordinators so schedule alarms and dispatches are restored.
// Unless all is set, it skips records without unfinished runs so idle coordinators can deactivate.
func (q *Coordinator) repairInternal(ctx context.Context, all bool) error {
	records, err := q.Records(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		if !all && !slices.ContainsFunc(record.Runs, func(run st.Run) bool { return !run.Status.Terminal() && run.Status != st.NeedsAttention }) {
			continue
		}
		if _, invokeErr := q.service.Invoke(ctx, coordinatorTypeInternal, kit.SHA256Hex(record.EnvironmentID+"\x00"+record.JobID), "repair", nil); invokeErr != nil {
			return invokeErr
		}
	}
	return nil
}

func (a *executorActorInternal) Job(ctx context.Context, _ string, data actor.Envelope) error {
	q := a.q
	q.mu.Lock()
	lifetime := q.lifecycleCtx
	q.mu.Unlock()
	if lifetime != nil {
		workCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(lifetime, cancel) //nolint:contextcheck // Application shutdown cancels work before the host drains its jobs.
		defer stop()
		defer cancel()
		ctx = workCtx
		if lifetime.Err() != nil {
			cancel()
		}
	}
	if !q.enabled.Load() {
		return actor.ErrJobRejected
	}
	workCtx, admitted, release := q.trackRestoreWorkerInternal(ctx)
	if !admitted {
		return actor.ErrJobRejected
	}
	defer release()
	ctx = workCtx
	var command st.ExecutionCommand
	if err := data.Decode(&command); err != nil {
		return errors.Join(err, actor.ErrJobPermanentFailure)
	}
	claim, err := q.claimExecutionInternal(ctx, command)
	if err != nil {
		return err
	}
	if claim.Deferred {
		return actor.ErrJobRejected
	}
	if !claim.Claimed {
		return nil
	}
	outcome, runErr := q.invokeInternal(ctx, claim.Run, claim.Reconcile)
	if outcome.Status == "" {
		if runErr != nil {
			outcome.Status = st.Failed
		} else {
			outcome.Status = st.Succeeded
		}
	}
	// Domain recovery may still report uncertainty; job runs finish as failures.
	if outcome.Status == st.NeedsAttention {
		outcome.Status = st.Failed
	}
	if runErr != nil && outcome.Message == "" {
		outcome.Message = runErr.Error()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	q.persistOutcomeInternal(ctx, claim.Run, outcome)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func (q *Coordinator) claimExecutionInternal(ctx context.Context, command st.ExecutionCommand) (st.ExecutionClaim, error) {
	var claim st.ExecutionClaim
	err := q.mutateInternal(ctx, command.EnvironmentID, command.JobID, func(record *st.QueueRecord) error {
		claim = st.ExecutionClaim{}
		for i := range record.Runs {
			current := &record.Runs[i]
			if current.ID != command.RunID {
				continue
			}
			if current.Status.Terminal() || current.Status == st.NeedsAttention {
				return nil
			}
			if current.AttemptCount > command.Attempt {
				return nil
			}
			if current.AttemptCount == command.Attempt && current.Status != st.Running {
				return nil
			}
			// Executor turns serialize attempts for this coordinator.
			now := time.Now().UTC()
			if current.NextAttempt != nil && current.NextAttempt.After(now) {
				claim.Deferred = true
				return nil
			}
			claim.Reconcile = current.Status == st.Running
			current.Status = st.Running
			current.Owner = q.owner + "/" + uuid.New().String()
			current.UpdatedAt = now
			// Manager claims track delivery; accepted agent runs keep execution timestamps.
			if current.EnvironmentID == "0" || !current.RemoteAccepted {
				current.StartedAt = &now
			}
			current.NextAttempt = nil
			current.AttemptCount = command.Attempt
			current.Attempts = append(current.Attempts, st.Attempt{Number: current.AttemptCount, StartedAt: now})
			if len(current.Attempts) > 100 {
				current.Attempts = current.Attempts[len(current.Attempts)-100:]
			}
			claim.Run = *current
			claim.Claimed = true
			return nil
		}
		return nil
	})
	return claim, err
}

// Every coordinator write, including dispatch and alarm updates, validates nested
// outcome evidence before handing the atomic state to the actor store.
func (a *coordinatorActorInternal) saveStateInternal(ctx context.Context, state st.CoordinatorState) error {
	if err := state.Record.ValidateReasons(); err != nil {
		return err
	}
	return a.q.service.SetState(ctx, coordinatorTypeInternal, a.id, state, nil)
}
