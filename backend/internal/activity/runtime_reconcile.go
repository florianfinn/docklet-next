// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package activity

import (
	"context"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *RuntimeService) remoteUnknownInternal(ctx context.Context, ref RuntimeRunReference, reason string) (activitytypes.RuntimeSnapshot, error) {
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	lock := s.activity.publishLockInternal(ref.RunID)
	lock.Lock()
	defer lock.Unlock()
	var snapshot activitytypes.RuntimeSnapshot
	var message *ActivityMessage
	err := s.ownerTransactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		row, err := runtimeReferenceRowTxInternal(tx, ref)
		if err != nil {
			return err
		}
		if row.SettledAt != nil {
			snapshot, err = runtimeSnapshotTxInternal(tx, row)
			return err
		}
		unchanged := row.UnknownReason != nil && *row.UnknownReason == reason
		row, err = runtimeEvidenceTxInternal(tx, now, runtimeRowIdentityInternal(row), runtimeEvidenceInternal{State: "unknown", UnknownReason: new(reason), StartedAt: row.ExecutorStartedAt})
		if err != nil {
			return err
		}
		if *row.ExecutorState != "unknown" || row.UnknownReason == nil || *row.UnknownReason != reason {
			return &runtimeConflictInternal{}
		}
		if !unchanged {
			message = &ActivityMessage{BaseModel: database.BaseModel{ID: uuid.NewString(), CreatedAt: now, UpdatedAt: new(now)}, ActivityID: row.RunID, Code: string(activitytypes.CodeRuntimeOutcomeUnknown), Level: activitytypes.MessageLevelWarning, Message: "Runtime outcome unknown", Payload: database.JSON{"reason": reason, "operation": *row.OperationID}}
			if err := runtimeReceiverCreateTxInternal(tx, message); err != nil {
				return err
			}
			changed := tx.Model(&Activity{}).Where("id = ? AND status IN ?", row.RunID, []activitytypes.Status{activitytypes.StatusQueued, activitytypes.StatusRunning}).UpdateColumns(map[string]any{"status": activitytypes.StatusRunning, "message_code": string(activitytypes.CodeRuntimeOutcomeUnknown), "latest_message": "Runtime outcome unknown", "updated_at": now})
			if changed.Error != nil {
				return changed.Error
			}
			if changed.RowsAffected != 1 {
				return &runtimeConflictInternal{}
			}
		}
		snapshot, err = runtimeSnapshotTxInternal(tx, row)
		return err
	})
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	if message != nil {
		s.activity.publishMessageInternal(snapshot.EnvironmentID, snapshot.Type, activityMessageToDTOInternal(message))
		s.activity.publishActivityInternal(snapshot.Activity)
	}
	return snapshot, nil
}

// Refresh shares one application-owned bounded query cycle per run. Viewer
// cancellation detaches only its wait. A failed host has no recurring poll loop.
// Refresh joins an in-flight dispatch instead of querying: a query that reaches
// the executor before Execute would fence a run that is still being sent.
func (c *RuntimeRemoteCoordinator) Refresh(ctx context.Context, ref RuntimeRunReference) (activitytypes.RuntimeSnapshot, error) {
	return c.refreshInternal(ctx, ref, false, true)
}
func (c *RuntimeRemoteCoordinator) refreshInternal(ctx context.Context, ref RuntimeRunReference, force, joinDispatch bool) (activitytypes.RuntimeSnapshot, error) {
	snapshot, task, err := c.prepareRefreshInternal(ctx, ref, force, joinDispatch)
	if err != nil || task == nil {
		return snapshot, err
	}
	select {
	case <-ctx.Done():
		return activitytypes.RuntimeSnapshot{}, ctx.Err()
	case <-task.done:
		return c.Snapshot(ctx, ref)
	}
}

func (c *RuntimeRemoteCoordinator) scheduleRefreshInternal(ctx context.Context, ref RuntimeRunReference) error {
	_, _, err := c.prepareRefreshInternal(ctx, ref, false, false)
	return err
}

func (c *RuntimeRemoteCoordinator) prepareRefreshInternal(ctx context.Context, ref RuntimeRunReference, force, joinDispatch bool) (activitytypes.RuntimeSnapshot, *runtimeRemoteTaskInternal, error) {
	snapshot, err := c.Snapshot(ctx, ref)
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, nil, err
	}

	c.mu.Lock()
	if c.paused {
		c.mu.Unlock()
		return snapshot, nil, ErrRuntimeNotReady
	}
	if c.recovering[ref.RunID] {
		c.mu.Unlock()
		return snapshot, nil, ErrRuntimeRecoveryInProgress
	}
	if dispatch := c.dispatch[ref.RunID]; joinDispatch && dispatch != nil {
		c.mu.Unlock()
		return snapshot, dispatch, nil
	}
	task := c.queries[ref.RunID]
	if task == nil {
		if !force && snapshot.OutcomeState == activitytypes.RuntimeOutcomeKnown && !isTerminalActivityStatusInternal(snapshot.Status) {
			if last, ok := c.lastQuery[ref.RunID]; ok && c.schedule.Now().Sub(last) < 2*time.Second {
				c.mu.Unlock()
				return snapshot, nil, nil
			}
		}
		taskCtx, cancel := context.WithCancel(c.lifetime)
		task = &runtimeRemoteTaskInternal{cancel: cancel, done: make(chan struct{})}
		c.queries[ref.RunID] = task
		c.lastQuery[ref.RunID] = c.schedule.Now()
		go func() {
			defer func() {
				cancel()
				snapshot, err := c.Snapshot(context.WithoutCancel(taskCtx), ref)
				c.mu.Lock()
				delete(c.queries, ref.RunID)
				if err == nil && isTerminalActivityStatusInternal(snapshot.Status) {
					delete(c.stamps, ref.RunID)
					delete(c.lastQuery, ref.RunID)
				}
				// Any query that leaves a known active run (startup, transport
				// recovery or an explicit refresh) hands it to one follower.
				follow := c.registerFollowLockedInternal(ref, snapshot, err)
				close(task.done)
				c.mu.Unlock()
				c.startFollowInternal(ref, follow)
			}()
			c.reconcileInternal(taskCtx, ref)
		}()
	}
	c.mu.Unlock()
	return snapshot, task, nil
}
func (c *RuntimeRemoteCoordinator) reconcileInternal(ctx context.Context, ref RuntimeRunReference) {
	for attempt, delay := range []time.Duration{0, 2 * time.Second, 5 * time.Second} {
		if attempt > 0 && c.schedule.Wait(ctx, delay) != nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
		id, cancelled, terminal, err := c.queryIdentityInternal(ctx, ref)
		if err != nil {
			return
		}
		if terminal {
			return
		}
		budget, cancel := context.WithTimeout(ctx, 5*time.Second)
		started := c.schedule.Now()
		transport, err := c.transport(budget, ref.Store)
		var outcome activitytypes.RuntimeExecutionOutcome
		if err == nil && transport == nil {
			err = ErrRuntimeTransport
		}
		if err == nil {
			outcome, err = transport.Query(budget, id)
		}
		if err == nil && budget.Err() != nil {
			err = budget.Err()
		}
		recentQuery := c.schedule.Now().Sub(started) >= 0 && c.schedule.Now().Sub(started) < 5*time.Second
		cancel()
		if err != nil {
			_, _ = c.service.remoteUnknownInternal(ctx, ref, "transport")
			continue
		}
		if activitytypes.ValidateRuntimeOutcome(outcome, id) != nil {
			_, _ = c.service.remoteUnknownInternal(ctx, ref, "invalid-evidence")
			continue
		}
		if outcome.State == activitytypes.RuntimeAnswerNotReceived {
			if recentQuery && c.negativeRecentInternal(ref.RunID) {
				_, _ = c.service.remoteDispositionInternal(ctx, ref, runtimeDispositionInternal{Disposition: "not_received", By: "runtime-reconciler", Reason: "Executor committed matching not-received fence"})
			} else {
				_, _ = c.service.remoteUnknownInternal(ctx, ref, "evidence-expired")
			}
			return
		}
		if _, err = c.service.applyRemoteInternal(ctx, ref, outcome, true); err != nil {
			_, _ = c.service.remoteUnknownInternal(ctx, ref, "storage")
			return
		}
		if outcome.State == activitytypes.RuntimeAnswerTerminal {
			return
		}
		if !cancelled {
			return
		}
		// Absent Cancel is never authority. Forward durable intent again after a
		// matching active query, attributed to the reconciler at the executor.
		// The hub audit retains the original caller.
		cancelCtx, cancelCall := context.WithTimeout(ctx, 5*time.Second)
		answer, cancelErr := transport.Cancel(cancelCtx, activitytypes.RuntimeCancellation{Identity: id, Attribution: activitytypes.RuntimeAttribution{ID: "runtime-reconciler", Username: "Runtime reconciler"}})
		cancelCall()
		if cancelErr == nil && activitytypes.ValidateRuntimeOutcome(answer, id) == nil && answer.State != activitytypes.RuntimeAnswerNotReceived {
			if _, err = c.service.applyRemoteInternal(ctx, ref, answer, true); err != nil {
				return
			}
			if answer.State == activitytypes.RuntimeAnswerTerminal {
				return
			}
		} else {
			_, _ = c.service.remoteUnknownInternal(ctx, ref, "transport")
		}
	}
}
func (c *RuntimeRemoteCoordinator) queryIdentityInternal(ctx context.Context, ref RuntimeRunReference) (activitytypes.RuntimeRunIdentity, bool, bool, error) {
	s := c.service
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	var id activitytypes.RuntimeRunIdentity
	var cancel, terminal bool
	err := s.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		row, err := runtimeReferenceRowTxInternal(tx, ref)
		if err != nil {
			return err
		}
		id = runtimeWireIdentityInternal(row)
		cancel = row.CancelRequested
		terminal = row.ExecutorSettledAt != nil || (row.HubDisposition != nil && *row.HubDisposition != "released")
		return nil
	})
	return id, cancel, terminal, err
}
func (c *RuntimeRemoteCoordinator) negativeRecentInternal(runID string) bool {
	c.mu.Lock()
	stamp, ok := c.stamps[runID]
	c.mu.Unlock()
	if !ok {
		return false
	}
	elapsed := c.schedule.Now().Sub(stamp.mono)
	wall := c.service.activity.nowInternal().Sub(stamp.wall)
	// Detect store-clock discontinuity locally; no executor/browser clock is read.
	drift := wall - elapsed
	if drift < 0 {
		drift = -drift
	}
	return elapsed >= 0 && elapsed < time.Hour && drift < time.Second
}
func (c *RuntimeRemoteCoordinator) Cancel(ctx context.Context, ref RuntimeRunReference, actor usertypes.Actor) (activitytypes.RuntimeSnapshot, error) {
	snapshot, err := c.service.Cancel(ctx, ref, actor)
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	if isTerminalActivityStatusInternal(snapshot.Status) {
		return snapshot, nil
	}
	id, _, _, err := c.queryIdentityInternal(ctx, ref)
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	_ = id
	// A pending receipt is settled by Start's no-send cancellation transaction.
	c.service.activity.runningMu.Lock()
	var pending bool
	err = c.service.transactionInternal(ctx, func(tx *gorm.DB, _ time.Time) error {
		row, err := runtimeReferenceRowTxInternal(tx, ref)
		if err == nil {
			pending = *row.DispatchState == "pending"
		}
		return err
	})
	c.service.activity.runningMu.Unlock()
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	if pending {
		return snapshot, nil
	}
	// Cancellation keeps querying at once: a fence before Execute is the cancellation.
	return c.refreshInternal(ctx, ref, true, false)
}
