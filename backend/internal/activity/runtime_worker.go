// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: One local owner, application lifetime, and joined settlement.

package activity

import (
	"context"
	"errors"
	"io"
	"time"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	containerDTO "github.com/florianfinn/docklet-next/types/container"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
)

// RuntimeExecutor must return only after its workflow and all known producers
// and children have joined. The runtime service adapter supplies this port;
// returning an error does not imply rollback or verified daemon state.
type RuntimeExecutor interface {
	ExecuteRuntime(context.Context, RuntimeCommand, io.Writer) error
}

// RuntimeLocalConfig is trusted construction, never a request execution toggle.
// The store binding must describe this continuous local executor, not a remote.
type RuntimeLocalConfig struct {
	ApplicationContext context.Context
	Store              RuntimeStoreBinding
	Executor           RuntimeExecutor
}

const runtimeWorkflowLimit = 30 * time.Minute

var ErrRuntimeNotSettled = errors.New("runtime owner has not settled")

// All mutable fields are protected by ActivityService.runningMu. attemptDone
// joins the initial worker attempt, including output and its terminal write.
// Failed persistence retains this owner and its immutable result for retry.
type runtimeOwnerInternal struct {
	command         runtimeCanonicalCommandInternal
	id              runtimeIdentityInternal
	ctx             context.Context
	cancel          context.CancelCauseFunc
	deadlineCancel  context.CancelFunc
	attemptDone     chan struct{}
	result          *runtimeLocalResultInternal
	err             error
	startedAt       *time.Time
	cancelRequested bool
}

// Caller holds runningMu after admission COMMIT. Registration cannot fail the
// already accepted request. A failed reread becomes a retained owner failure.
func (s *RuntimeService) registerLocalInternal(request context.Context, command runtimeCanonicalCommandInternal, runID string) {
	// Detach viewer cancellation metadata as well as Done; otherwise a request
	// cancel cause can leak through the helper's value lookup on later app stop.
	ctx := utils.WithAppLifecycleContext(utils.ActivityRuntimeContext(context.WithoutCancel(request), s.localExecution.ApplicationContext))
	ctx = context.WithValue(ctx, gateRunContextKey{}, runID)
	ctx, deadlineCancel := context.WithTimeout(ctx, runtimeWorkflowLimit)
	ctx, cancel := context.WithCancelCause(ctx)
	owner := &runtimeOwnerInternal{command: command, id: command.identity(runID), ctx: ctx,
		cancel: cancel, deadlineCancel: deadlineCancel, attemptDone: make(chan struct{})}
	s.owners[runID] = owner
	owner.err = s.ownerTransactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		row, err := runtimeMutableReceiptTxInternal(tx, owner.id)
		if err != nil {
			return err
		}
		if row.CancelRequested {
			owner.cancelRequested = true
			owner.cancel(activitylib.ErrCanceled)
		}
		return nil
	})
	go s.runLocalInternal(owner)
}

// A panic terminates the process: an executor may have live children whose
// completion is unknown. Recovering here cannot authorize settlement or release.
// Startup interrupts local receipts only after the prior process has ended.
func (s *RuntimeService) runLocalInternal(owner *runtimeOwnerInternal) {
	defer close(owner.attemptDone)
	s.activity.runningMu.Lock()
	phaseErr := owner.err
	s.activity.runningMu.Unlock()
	execute := false
	if phaseErr == nil {
		execute, phaseErr = s.acquireLocalSlotInternal(owner)
	}
	var outputErr error
	var imageResult *updaterDTO.LifecycleResult
	var recreateResult *containerDTO.RecreateResult
	if execute && phaseErr == nil && owner.ctx.Err() == nil {
		output, err := s.captureInternal(owner.ctx, owner.id.RunID)
		if err != nil {
			phaseErr = err
		} else {
			// The executor's return is its child/producer barrier, never a timer.
			command := owner.command.command
			command.ProposedRunID = owner.id.RunID
			execution := updaterDTO.WithResultSink(owner.ctx, func(result updaterDTO.LifecycleResult) { owned := result; imageResult = &owned })
			execution = containerDTO.WithRecreateResultSink(execution, func(result containerDTO.RecreateResult) { owned := result; recreateResult = &owned })
			phaseErr = s.localExecution.Executor.ExecuteRuntime(execution, command, output)
			outputErr = runtimeCaptureFailureInternal(output, phaseErr)
			outputErr = errors.Join(outputErr, runtimeFinishOutputInternal(owner.ctx, output))
		}
	}
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	var uncertain interface{ RuntimeUncertain() bool }
	if (errors.As(phaseErr, &uncertain) && uncertain.RuntimeUncertain()) || s.recreateUnresolvedInternal(owner) {
		owner.err = s.ownerTransactionInternal(owner.ctx, func(tx *gorm.DB, now time.Time) error {
			if imageResult != nil {
				if err := runtimeImageResultTx(tx, owner.id.RunID, imageResult); err != nil {
					return err
				}
			}
			_, err := runtimeEvidenceTxInternal(tx, now, owner.id, runtimeEvidenceInternal{State: "unknown", UnknownReason: new("runtime.outcome-unknown"), StartedAt: owner.startedAt})
			return err
		})
		if owner.err == nil {
			delete(s.owners, owner.id.RunID)
			owner.cancel(nil)
			owner.deadlineCancel()
			s.activity.releaseSlotInternal(owner.id.RunID)
		}
		return
	}
	owner.result = runtimeResultInternal(owner, phaseErr, outputErr, recreateResult != nil, runtimeTimeInternal(s.activity.nowInternal()))
	owner.result.imageLifecycle = imageResult
	owner.result.recreate = recreateResult
	_, owner.err = s.settleOwnerInternal(owner)
	if owner.err == nil {
		if settled, ok := s.localExecution.Executor.(interface{ RuntimeSettled(context.Context) }); ok {
			settled.RuntimeSettled(context.WithoutCancel(owner.ctx))
		}
	}
}

// An unresolved recreate journal is never settled terminally by the attempt;
// an unreadable journal counts as unresolved.
func (s *RuntimeService) recreateUnresolvedInternal(owner *runtimeOwnerInternal) bool {
	if !runtimeRecreateOperation(owner.command.command.Operation) {
		return false
	}
	unresolved, err := runtimeRecreateUnresolvedTxInternal(s.activity.db.WithContext(context.WithoutCancel(owner.ctx)), owner.id.RunID)
	return err != nil || unresolved
}

// Every phase rereads durable cancellation under the same serialization used by
// cancellation. The executing commit is the effect-start decision; subsequent
// cancellation signals the owned context before any further owner transition.
func (s *RuntimeService) localPhaseInternal(owner *runtimeOwnerInternal, executing bool) (bool, error) {
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	lock := s.activity.publishLockInternal(owner.id.RunID)
	lock.Lock()
	defer lock.Unlock()
	execute := false
	var startedAt *time.Time
	var snapshot activitytypes.RuntimeSnapshot
	err := s.ownerTransactionInternal(owner.ctx, func(tx *gorm.DB, now time.Time) error {
		execute, startedAt = false, nil
		snapshot = activitytypes.RuntimeSnapshot{}
		row, err := runtimeMutableReceiptTxInternal(tx, owner.id)
		if err != nil {
			return err
		}
		if row.CancelRequested {
			owner.cancelRequested = true
			owner.cancel(activitylib.ErrCanceled)
			return nil
		}
		if owner.ctx.Err() != nil {
			return nil
		}
		if executing {
			row, err = runtimeLocalExecutingTxInternal(tx, now, owner.id)
		} else {
			row, err = runtimeLocalAcceptTxInternal(tx, now, owner.id)
		}
		if err != nil {
			return err
		}
		if executing && runtimeQueuesInternal(owner.command.command.Operation) {
			changed := tx.Model(&Activity{}).Where("id = ? AND environment_id = ? AND status = ?", row.RunID, row.EnvironmentID,
				activitytypes.StatusQueued).UpdateColumns(map[string]any{"status": activitytypes.StatusRunning, "updated_at": now})
			if changed.Error != nil {
				return changed.Error
			}
			if changed.RowsAffected != 1 {
				return &runtimeConflictInternal{}
			}
			snapshot, err = runtimeSnapshotTxInternal(tx, row)
			if err != nil {
				return err
			}
		}
		startedAt = copyPtrInternal(row.ExecutorStartedAt)
		execute = true
		return nil
	})
	if err == nil {
		owner.startedAt = startedAt
		if snapshot.ID != "" {
			s.activity.publishActivityInternal(snapshot.Activity)
		}
	}
	return execute && err == nil, err
}

// A recreate past its first journal write runs detached; its own outcome (result
// or recreate problem) wins over a later cancellation or run deadline.
func runtimeResultInternal(owner *runtimeOwnerInternal, workflowErr, outputErr error, recreated bool, now time.Time) *runtimeLocalResultInternal {
	result := &runtimeLocalResultInternal{state: "success", code: activitytypes.CodeActivitySuccess,
		outputLost: outputErr != nil, message: "Runtime workflow completed", settledAt: now, startedAt: copyPtrInternal(owner.startedAt)}
	var problem *containerDTO.Problem
	recreateProblem := errors.As(workflowErr, &problem) && (problem.Code == "container.recreate-recovered" || problem.Code == "container.recreate-ambiguous")
	detached := (recreated && workflowErr == nil) || recreateProblem
	switch {
	case owner.cancelRequested && !detached:
		result.state, result.code, result.message = "cancelled", activitytypes.CodeRuntimeCancelled, "Runtime workflow cancelled; effects unverified"
	case owner.ctx.Err() != nil && !detached:
		result.state, result.code, result.message = "failed", activitytypes.CodeRuntimeInterrupted, "Runtime workflow interrupted; effects unverified"
		result.errorText = new(result.message)
	case workflowErr != nil || outputErr != nil:
		result.state, result.code, result.message = "failed", activitytypes.CodeRuntimeFailed, "Runtime workflow or output failed; effects unverified"
		// Persist bounded coded evidence, not arbitrary SDK errors containing secrets.
		result.errorText = new(result.message)
	}
	if result.state == "failed" && recreateProblem {
		result.code = activitytypes.Code(problem.Code)
		result.message = problem.Code
		if step := problem.Step(); step != "" {
			result.message = problem.Code + ": " + step + " failed"
		}
		result.errorText = new(result.message)
	}
	return result
}

// WaitLocal requires current activity-read grants for the referenced run.
// It is viewer-only: timeout never cancels work or frees a scope.
func (s *RuntimeService) WaitLocal(ctx context.Context, ref RuntimeRunReference) (activitytypes.RuntimeSnapshot, error) {
	if err := s.checkInternal(); err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	s.activity.runningMu.Lock()
	snapshot, err := s.boundSnapshotInternal(ctx, ref)
	owner := s.owners[ref.RunID]
	s.activity.runningMu.Unlock()
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	if owner == nil {
		return snapshot, nil
	}
	select {
	case <-owner.attemptDone:
	case <-ctx.Done():
		return activitytypes.RuntimeSnapshot{}, ctx.Err()
	}
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	snapshot, err = s.boundSnapshotInternal(ctx, ref)
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	return snapshot, owner.err
}

func (s *RuntimeService) boundSnapshotInternal(ctx context.Context, ref RuntimeRunReference) (activitytypes.RuntimeSnapshot, error) {
	var snapshot activitytypes.RuntimeSnapshot
	err := s.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		row, err := runtimeReferenceRowTxInternal(tx, ref)
		if err != nil {
			return err
		}
		snapshot, err = runtimeSnapshotTxInternal(tx, row)
		return err
	})
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	return snapshot, nil
}
