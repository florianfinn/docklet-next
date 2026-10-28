// Source: getarcaneapp/arcane/backend/internal/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; M12 persisted volume lifecycle and legacy restore cut.

// Package backup owns the shared Rustic backup engine used by volume and
// system backups: typed repository operations, per-repository serialization
// and durable run admission.
package backup

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/francis"
	"github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/italypaleale/francis/actor"
	"github.com/italypaleale/francis/host/local"
	"log/slog"
	"maps"
	"sync"
	"time"
)

const backupRunTypeInternal = "backup-run"

type backupRunActorInternal struct {
	id      string
	engine  *Engine
	service *actor.Service
}

// RegisterRunKind binds a domain handler before the actor host starts.
func (e *Engine) RegisterRunKind(kind string, execute func(context.Context, string, []byte, bool) error, failures ...func(context.Context, string, []byte, error) error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[kind] = execute
	if len(failures) > 0 {
		e.failures[kind] = failures[0]
	}
}

func (e *Engine) SetAuthorize(authorize func(context.Context, backup.DurableRunCommand) error) {
	e.authorize = authorize
}

func (e *Engine) SetExecutionReady(ready func() bool) { e.executionReady = ready }

func (e *Engine) Register(runtime *francis.Runtime) error {
	e.service = runtime.Service()
	return runtime.RegisterActor(backupRunTypeInternal, func(id string, service *actor.Service) actor.Actor {
		return &backupRunActorInternal{id: id, engine: e, service: service}
	}, local.WithCapacityGroup("jobs", 4), local.WithCompletedJobRetention(7*24*time.Hour))
}

// SubmitDurableRun persists the accepted command before dispatching its work.
// A nil error means the run was accepted or its outcome is still unresolved;
// an error means the intent was confirmed absent and admission is the caller's.
func (e *Engine) SubmitDurableRun(ctx context.Context, command backup.DurableRunCommand, lease *runs.Lease) error {
	if e == nil || e.service == nil {
		return errors.New("backup actor host is unavailable")
	}
	if e.authorize != nil {
		if err := e.authorize(ctx, command); err != nil {
			return err
		}
	}
	e.mu.Lock()
	if e.stopping {
		e.mu.Unlock()
		return errors.New("backup engine is stopping")
	}
	e.leases[command.RunID] = lease
	e.mu.Unlock()
	ctx = context.WithoutCancel(ctx)
	_, err := e.service.Invoke(ctx, backupRunTypeInternal, command.RunID, "submit", command)
	if err == nil {
		return nil
	}
	var state backup.DurableRunState
	readErr := e.service.GetState(ctx, backupRunTypeInternal, command.RunID, &state)
	switch {
	case readErr == nil && sameDurableCommandInternal(state.Command, command):
		slog.WarnContext(ctx, "backup run accepted after submit error", "runId", command.RunID, "error", err)
		return nil
	case readErr == nil || errors.Is(readErr, actor.ErrStateNotFound):
		e.mu.Lock()
		delete(e.leases, command.RunID)
		e.mu.Unlock()
		return err
	default:
		// Keep admission parked until reconciliation can read the intent back.
		slog.WarnContext(ctx, "backup run outcome unresolved", "runId", command.RunID, "error", err, "readError", readErr)
		e.mu.Lock()
		e.unresolved[command.RunID] = command
		e.mu.Unlock()
		return nil
	}
}

// AcquireDurableRun keeps the original admission, or reacquires it after a restart.
func (e *Engine) AcquireDurableRun(ctx context.Context, runID, scope, resourceID string) (*runs.Lease, bool, error) {
	e.mu.Lock()
	lease := e.leases[runID]
	delete(e.leases, runID)
	e.mu.Unlock()
	if lease != nil {
		return lease, true, nil
	}
	return e.TryAcquireRun(ctx, scope, resourceID)
}

func (a *backupRunActorInternal) Invoke(ctx context.Context, _ string, data actor.Envelope) (any, error) {
	var command backup.DurableRunCommand
	if err := data.Decode(&command); err != nil {
		return nil, err
	}
	var state backup.DurableRunState
	err := a.service.GetState(ctx, backupRunTypeInternal, a.id, &state)
	switch {
	case errors.Is(err, actor.ErrStateNotFound):
		state = backup.DurableRunState{Command: command, Status: scheduler.Queued}
		if setStateErr := a.service.SetState(ctx, backupRunTypeInternal, a.id, state, nil); setStateErr != nil {
			return nil, setStateErr
		}
	case err != nil:
		return nil, err
	case !sameDurableCommandInternal(state.Command, command):
		return nil, errors.New("backup run already exists with a different command")
	}
	if state.Status == scheduler.Succeeded || state.Status == scheduler.Failed || state.Status == scheduler.NeedsAttention {
		return nil, nil
	}
	// The intent is persisted, so ReconcileDispatches repairs a failed dispatch.
	if _, _, dispatchErr := a.service.Dispatch(ctx, backupRunTypeInternal, a.id, "execute", nil, actor.WithIdempotencyKey(a.id)); dispatchErr != nil {
		slog.WarnContext(ctx, "dispatch accepted backup run", "runId", a.id, "error", dispatchErr)
	}
	return nil, nil
}

func sameDurableCommandInternal(a, b backup.DurableRunCommand) bool {
	return a.UserID == b.UserID &&
		a.EnvironmentID == b.EnvironmentID &&
		a.Permission == b.Permission &&
		a.RequestedWithKey == b.RequestedWithKey &&
		a.Kind == b.Kind &&
		a.RunID == b.RunID &&
		a.ActivityID == b.ActivityID &&
		bytes.Equal(a.Payload, b.Payload)
}

func (a *backupRunActorInternal) Job(ctx context.Context, _ string, _ actor.Envelope) error {
	if a.engine.executionReady != nil && !a.engine.executionReady() {
		return actor.ErrJobRejected
	}
	a.engine.mu.Lock()
	if a.engine.stopping {
		a.engine.mu.Unlock()
		return actor.ErrJobRejected
	}
	a.engine.workers.Add(1)
	a.engine.mu.Unlock()
	defer a.engine.workers.Done()
	jobCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(a.engine.lifecycleCtx, cancel) //nolint:contextcheck // Cancels the inherited job context when the engine stops.
	defer stop()
	defer cancel()
	ctx = jobCtx

	var state backup.DurableRunState
	if err := a.service.GetState(ctx, backupRunTypeInternal, a.id, &state); err != nil {
		return err
	}
	if state.Status == scheduler.Succeeded || state.Status == scheduler.Failed || state.Status == scheduler.NeedsAttention {
		return nil
	}
	a.engine.mu.Lock()
	pendingLease := a.engine.leases[a.id]
	a.engine.mu.Unlock()
	defer pendingLease.Release(ctx)
	interrupted := state.Started && len(state.Targets) > 0
	state.Started = true
	state.Status = scheduler.Running
	if err := a.service.SetState(ctx, backupRunTypeInternal, a.id, state, nil); err != nil {
		return err
	}
	var progressMu sync.Mutex
	ctx = jobcontext.WithExecution(ctx, scheduler.Run{ID: a.id, EnvironmentID: "0", Outcome: scheduler.Outcome{Targets: state.Targets}}, func(target scheduler.TargetOutcome) error {
		progressMu.Lock()
		defer progressMu.Unlock()
		updated := false
		for i := range state.Targets {
			if state.Targets[i].ID == target.ID {
				if len(target.RecoveryData) == 0 {
					target.RecoveryData = state.Targets[i].RecoveryData
				}
				state.Targets[i] = target
				updated = true
				break
			}
		}
		if !updated {
			state.Targets = append(state.Targets, target)
		}
		return a.service.SetState(ctx, backupRunTypeInternal, a.id, state, nil)
	})
	runErr := a.executeInternal(ctx, state.Command, interrupted)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	state.Status = scheduler.Succeeded
	if runErr != nil {
		state.Status = scheduler.NeedsAttention
		state.Error = runErr.Error()
	}
	if err := a.service.SetState(ctx, backupRunTypeInternal, a.id, state, nil); err != nil {
		return err
	}
	if runErr != nil {
		return fmt.Errorf("%s: %w", runErr.Error(), actor.ErrJobPermanentFailure)
	}
	return nil
}

func (a *backupRunActorInternal) executeInternal(ctx context.Context, command backup.DurableRunCommand, interrupted bool) (err error) {
	defer utils.RecoverToError(&err, "durable backup")
	a.engine.mu.Lock()
	handler := a.engine.handlers[command.Kind]
	failure := a.engine.failures[command.Kind]
	a.engine.mu.Unlock()
	if handler == nil {
		return errors.New("backup run kind is unavailable")
	}
	if a.engine.authorize != nil {
		if authorizeErr := a.engine.authorize(ctx, command); authorizeErr != nil {
			a.engine.mu.Lock()
			lease := a.engine.leases[a.id]
			delete(a.engine.leases, a.id)
			a.engine.mu.Unlock()
			defer lease.Release(ctx)
			if failure != nil {
				authorizeErr = errors.Join(authorizeErr, failure(ctx, a.id, command.Payload, authorizeErr))
			}
			return authorizeErr
		}
	}
	return handler(ctx, a.id, command.Payload, interrupted)
}

func (e *Engine) activeRunsInternal(ctx context.Context) ([]backup.DurableRunState, error) {
	result := []backup.DurableRunState{}
	for cursor := ""; ; {
		page, err := e.service.ListStates(ctx, backupRunTypeInternal, &actor.ListStatesOpts{IncludeData: true, After: cursor, Limit: 100})
		if err != nil {
			return nil, err
		}
		for _, item := range page.States {
			if item.Data == nil {
				continue
			}
			var state backup.DurableRunState
			if decodeErr := item.Data.Decode(&state); decodeErr != nil {
				return nil, decodeErr
			}
			if state.Status == scheduler.Queued || state.Status == scheduler.Running || state.Status == scheduler.NeedsAttention {
				result = append(result, state)
			}
		}
		cursor = page.AfterID()
		if cursor == "" {
			break
		}
	}
	return result, nil
}

func (e *Engine) ActiveRunIDs(ctx context.Context) ([]string, error) {
	states, err := e.activeRunsInternal(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(states))
	for _, state := range states {
		ids = append(ids, state.Command.RunID)
		for _, target := range state.Targets {
			var checkpoint struct {
				BackupID string `json:"backupId"`
			}
			if len(target.RecoveryData) > 0 && json.Unmarshal(target.RecoveryData, &checkpoint) == nil && checkpoint.BackupID != "" {
				ids = append(ids, checkpoint.BackupID)
			}
		}
	}
	return ids, nil
}

func (e *Engine) ActiveActivityIDs(ctx context.Context) ([]string, error) {
	states, err := e.activeRunsInternal(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(states))
	for _, state := range states {
		if state.Command.ActivityID != "" {
			ids = append(ids, state.Command.ActivityID)
		}
	}
	return ids, nil
}

// ReconcileDispatches resolves unresolved submissions and repairs accepted
// commands whose dispatch was interrupted.
func (e *Engine) ReconcileDispatches(ctx context.Context) error {
	failures := e.resolveUnresolvedRunsInternal(ctx)
	states, err := e.activeRunsInternal(ctx)
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	for _, state := range states {
		if state.Status == scheduler.NeedsAttention || state.Command.Kind == "volume" {
			continue
		}
		if repairErr := e.repairDispatchInternal(ctx, state.Command.RunID); repairErr != nil {
			failures = append(failures, fmt.Errorf("repair backup run %s: %w", state.Command.RunID, repairErr))
		}
	}
	return errors.Join(failures...)
}

func (e *Engine) repairDispatchInternal(ctx context.Context, runID string) error {
	jobs, err := e.service.ListJobs(ctx, backupRunTypeInternal, runID)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if !job.Status.IsTerminal() {
			return nil
		}
		if job.Status == actor.JobStatusDeadLettered {
			_, retryJobErr := e.service.RetryJob(ctx, job.JobID)
			return retryJobErr
		}
		if deleteJobErr := e.service.DeleteJob(ctx, backupRunTypeInternal, runID, job.JobID); deleteJobErr != nil && !errors.Is(deleteJobErr, actor.ErrJobNotFound) {
			return deleteJobErr
		}
	}
	_, _, err = e.service.Dispatch(ctx, backupRunTypeInternal, runID, "execute", nil, actor.WithIdempotencyKey(runID))
	return err
}

// resolveUnresolvedRunsInternal reads back submissions whose outcome was
// unknown. Found intents continue as accepted; confirmed absence releases
// admission and fails the run through its kind's failure handler.
func (e *Engine) resolveUnresolvedRunsInternal(ctx context.Context) []error {
	e.mu.Lock()
	pending := maps.Clone(e.unresolved)
	e.mu.Unlock()
	var failures []error
	for runID, command := range pending {
		var state backup.DurableRunState
		readErr := e.service.GetState(ctx, backupRunTypeInternal, runID, &state)
		if readErr != nil && !errors.Is(readErr, actor.ErrStateNotFound) {
			failures = append(failures, fmt.Errorf("read back backup run %s: %w", runID, readErr))
			continue
		}
		e.mu.Lock()
		lease := e.leases[runID]
		failure := e.failures[command.Kind]
		if readErr != nil {
			delete(e.leases, runID)
		}
		e.mu.Unlock()
		if readErr != nil {
			lease.Release(ctx)
			// Stay unresolved until the failure is recorded, so a later tick retries it.
			if failure != nil {
				if failErr := failure(ctx, runID, command.Payload, errors.New("backup run was not accepted")); failErr != nil {
					failures = append(failures, fmt.Errorf("fail backup run %s: %w", runID, failErr))
					continue
				}
			}
		}
		e.mu.Lock()
		delete(e.unresolved, runID)
		e.mu.Unlock()
	}
	return failures
}
