// Source: getarcaneapp/arcane/backend/pkg/scheduler/runs/actors.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; SQLite-only local persistence; M18 live scheduling, policy and contract foundation.

package runs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/italypaleale/francis/actor"
)

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
	if !orphanedDispatchInternal(job, a.q.clock()) {
		return !job.Status.IsTerminal(), nil
	}
	return false, a.deleteOrphanInternal(ctx, state, job)
}

func (a *coordinatorActorInternal) dispatchRunInternal(ctx context.Context, state *st.CoordinatorState, run st.Run, attempt int) error {
	for _, current := range state.Record.Runs {
		if current.ID == run.ID && current.Status.Terminal() {
			return nil
		}
	}
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
			if !orphanedDispatchInternal(job, a.q.clock()) {
				return nil
			}
			if err := a.deleteOrphanInternal(ctx, state, job); err != nil {
				return err
			}
			for _, current := range state.Record.Runs {
				if current.ID == run.ID && current.Status.Terminal() {
					return nil
				}
			}
			err = actor.ErrJobNotFound
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

const orphanedDispatchAge = 15 * time.Minute

func orphanedDispatchInternal(job actor.JobInfo, now time.Time) bool {
	return job.Status == actor.JobStatusPending && !job.DueTime.After(now.Add(-orphanedDispatchAge))
}

// Live-only deletion preserves a concurrent completion; reload its atomic run state.
func (a *coordinatorActorInternal) deleteOrphanInternal(ctx context.Context, state *st.CoordinatorState, job actor.JobInfo) error {
	err := a.q.service.DeleteJob(ctx, job.ActorType, a.id, job.JobID, actor.WithLiveJobsOnly())
	if err != nil && !errors.Is(err, actor.ErrJobNotFound) {
		return err
	}
	return a.q.service.GetState(ctx, coordinatorTypeInternal, a.id, state)
}
