// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded remote runtime implementation; production activation is separate.

package activity

import (
	"context"
	"errors"
	"sync"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"gorm.io/gorm"
)

type RuntimeTransportFactory func(context.Context, RuntimeStoreBinding) (RuntimeRemoteTransport, error)
type RuntimeReconcileSchedule struct {
	Now  func() time.Time
	Wait func(context.Context, time.Duration) error
}
type runtimeRemoteTaskInternal struct {
	cancel context.CancelFunc
	done   chan struct{}
}
type runtimeDispatchStampInternal struct{ mono, wall time.Time }

type RuntimeRemoteCoordinator struct {
	lifecycleMu sync.Mutex
	paused      bool
	service     *RuntimeService
	lifetime    context.Context
	transport   RuntimeTransportFactory
	schedule    RuntimeReconcileSchedule
	mu          sync.Mutex
	dispatch    map[string]*runtimeRemoteTaskInternal
	queries     map[string]*runtimeRemoteTaskInternal
	follows     map[string]*runtimeRemoteTaskInternal
	// followEvery spaces reader-independent follow queries in real time.
	followEvery time.Duration
	stamps      map[string]runtimeDispatchStampInternal
	lastQuery   map[string]time.Time
	recovering  map[string]bool
}

func NewRuntimeRemoteCoordinator(service *RuntimeService, lifetime context.Context, transport RuntimeTransportFactory, schedule RuntimeReconcileSchedule) (*RuntimeRemoteCoordinator, error) {
	if err := service.checkInternal(); err != nil {
		return nil, err
	}
	if lifetime == nil || transport == nil {
		return nil, errRuntimeInvalidInternal
	}
	if schedule.Now == nil {
		schedule.Now = time.Now
	}
	if schedule.Wait == nil {
		schedule.Wait = runtimeWaitInternal
	}
	service.activity.runningMu.Lock()
	defer service.activity.runningMu.Unlock()
	if service.remote != nil {
		return nil, errRuntimeInvalidInternal
	}
	c := &RuntimeRemoteCoordinator{service: service, lifetime: lifetime, transport: transport, schedule: schedule, dispatch: make(map[string]*runtimeRemoteTaskInternal), queries: make(map[string]*runtimeRemoteTaskInternal), follows: make(map[string]*runtimeRemoteTaskInternal), followEvery: 2 * time.Second, stamps: make(map[string]runtimeDispatchStampInternal), lastQuery: make(map[string]time.Time), recovering: make(map[string]bool)}
	service.remote = c
	return c, nil
}
func runtimeWaitInternal(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Start commits dispatching (or admitted cancellation) before constructing any
// network request. A retry never sends Execute again, even after process reopen.
func (c *RuntimeRemoteCoordinator) Start(ctx context.Context, ref RuntimeRunReference, options RuntimeOptions) (activitytypes.RuntimeSnapshot, error) {
	s := c.service
	s.activity.runningMu.Lock()
	var snapshot activitytypes.RuntimeSnapshot
	var envelope activitytypes.RuntimeExecutionEnvelope
	send := false
	var final *ActivityMessage
	stamp := runtimeDispatchStampInternal{mono: c.schedule.Now()}
	lock := s.activity.publishLockInternal(ref.RunID)
	lock.Lock()
	// Keep pause/recovery and dispatch COMMIT/task registration in one order.
	// No holder of mu waits for runningMu or for a task while holding mu.
	c.mu.Lock()
	if c.paused || c.recovering[ref.RunID] || c.lifetime.Err() != nil {
		c.mu.Unlock()
		lock.Unlock()
		s.activity.runningMu.Unlock()
		return activitytypes.RuntimeSnapshot{}, ErrRuntimeRecoveryInProgress
	}
	err := s.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		stamp.wall = now
		if s.intakePaused {
			return ErrRuntimeNotReady
		}
		row, err := runtimeReferenceRowTxInternal(tx, ref)
		if err != nil {
			return err
		}
		if row.EnvironmentID == "0" || row.ExecutorGeneration == row.StoreGeneration {
			return errRuntimeInvalidInternal
		}
		if err := runtimeTargetPresentTxInternal(tx, ref.Store); err != nil {
			return err
		}
		if row.SettledAt != nil || *row.DispatchState != "pending" {
			snapshot, err = runtimeSnapshotTxInternal(tx, row)
			return err
		}
		if row.CancelRequested {
			snapshot, final, err = runtimeHubDispositionTxInternal(tx, now, row, runtimeDispositionInternal{Disposition: "cancelled", By: *row.CallerID, Reason: "Runtime cancelled before dispatch"})
			return err
		}
		canonical, err := runtimeCanonicalizeSemanticInternal(RuntimeCommand{Store: ref.Store, ProposedRunID: row.RunID, Operation: RuntimeOperation(*row.OperationID), Target: RuntimeTarget{Kind: *row.TargetKind, ID: *row.TargetID, Name: row.TargetName, ParentProjectID: row.ParentProjectID}, Options: options, Actor: usertypes.Actor{ID: *row.CallerID, Username: *row.CallerName}})
		if err != nil {
			return err
		}
		if canonical.digest != row.RequestDigest {
			return &runtimeConflictInternal{}
		}
		var model Activity
		if err := tx.First(&model, "id = ?", row.RunID).Error; err != nil {
			return err
		}
		envelope = activitytypes.RuntimeExecutionEnvelope{Identity: runtimeWireIdentityInternal(row), Operation: canonical.command.Operation, Target: canonical.command.Target, Options: canonical.command.Options, Attribution: activitytypes.RuntimeAttribution{ID: *row.CallerID, Username: *row.CallerName, DisplayName: model.StartedByDisplayName}}
		row, err = runtimeDispatchTxInternal(tx, now, runtimeRowIdentityInternal(row), false)
		if err != nil {
			return err
		}
		if *row.DispatchState != "dispatching" || row.DispatchedAt == nil {
			return &runtimeConflictInternal{}
		}
		snapshot, err = runtimeSnapshotTxInternal(tx, row)
		send = err == nil
		return err
	})
	if err == nil && final != nil {
		s.activity.publishMessageInternal(snapshot.EnvironmentID, snapshot.Type, activityMessageToDTOInternal(final))
		s.activity.publishActivityInternal(snapshot.Activity)
	}
	if err == nil && send {
		taskCtx, cancel := context.WithTimeout(c.lifetime, 30*time.Minute)
		task := &runtimeRemoteTaskInternal{cancel: cancel, done: make(chan struct{})}
		c.dispatch[ref.RunID] = task
		c.stamps[ref.RunID] = stamp
		go c.dispatchInternal(taskCtx, ref, envelope, task)
	}
	c.mu.Unlock()
	lock.Unlock()
	s.activity.runningMu.Unlock()
	if err != nil {
		return activitytypes.RuntimeSnapshot{}, err
	}
	return snapshot, nil
}
func (c *RuntimeRemoteCoordinator) dispatchInternal(ctx context.Context, ref RuntimeRunReference, envelope activitytypes.RuntimeExecutionEnvelope, task *runtimeRemoteTaskInternal) {
	defer func() {
		task.cancel()
		snapshot, err := c.Snapshot(context.WithoutCancel(ctx), ref)
		c.mu.Lock()
		if c.dispatch[ref.RunID] == task {
			delete(c.dispatch, ref.RunID)
		}
		if err == nil && isTerminalActivityStatusInternal(snapshot.Status) {
			delete(c.stamps, ref.RunID)
			delete(c.lastQuery, ref.RunID)
		}
		// Register the follower before this owned task ends, so Close always
		// either joins it or prevents it.
		follow := c.registerFollowLockedInternal(ref, snapshot, err)
		close(task.done)
		c.mu.Unlock()
		c.startFollowInternal(ref, follow)
	}()
	transport, err := c.transport(ctx, ref.Store)
	var outcome activitytypes.RuntimeExecutionOutcome
	if err == nil && transport == nil {
		err = ErrRuntimeTransport
	}
	if err == nil {
		outcome, err = transport.Execute(ctx, envelope)
	}
	if err == nil {
		if outcome.State == activitytypes.RuntimeAnswerNotReceived {
			err = ErrRuntimeTransport
		} else {
			_, err = c.service.applyRemoteInternal(ctx, ref, outcome, false)
		}
	}
	if err == nil {
		return
	}
	var refusal *RuntimeExecutionRefused
	if errors.As(err, &refusal) && (activitytypes.RuntimeExecutionRefusal{Code: refusal.Code}).Safe() {
		_, _ = c.service.remoteDispositionInternal(ctx, ref, runtimeDispositionInternal{Disposition: "refused", By: envelope.Attribution.ID, Reason: string(refusal.Code)})
		return
	}
	_, _ = c.service.remoteUnknownInternal(ctx, ref, "transport")
	if ctx.Err() == nil {
		_, _ = c.Refresh(ctx, ref)
	}
}
func runtimeFollowableInternal(snapshot activitytypes.RuntimeSnapshot) bool {
	return snapshot.OutcomeState == activitytypes.RuntimeOutcomeKnown && !isTerminalActivityStatusInternal(snapshot.Status)
}

type runtimeFollowInternal struct {
	task *runtimeRemoteTaskInternal
	ctx  context.Context
}

// registerFollowLockedInternal claims reader-independent following for a known
// active run. Callers hold c.mu inside an owned dispatch or query task.
func (c *RuntimeRemoteCoordinator) registerFollowLockedInternal(ref RuntimeRunReference, snapshot activitytypes.RuntimeSnapshot, err error) *runtimeFollowInternal {
	if err != nil || !runtimeFollowableInternal(snapshot) || c.paused || c.lifetime.Err() != nil || c.follows[ref.RunID] != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(c.lifetime, 30*time.Minute)
	task := &runtimeRemoteTaskInternal{cancel: cancel, done: make(chan struct{})}
	c.follows[ref.RunID] = task
	return &runtimeFollowInternal{task: task, ctx: ctx}
}

// startFollowInternal settles a claimed run without any reader. It only uses
// the bounded Refresh path (one query in flight, real-time spacing), stops when
// the run is terminal or unknown, and never resends the effect.
func (c *RuntimeRemoteCoordinator) startFollowInternal(ref RuntimeRunReference, follow *runtimeFollowInternal) {
	if follow == nil {
		return
	}
	go func() {
		defer func() {
			follow.task.cancel()
			c.mu.Lock()
			if c.follows[ref.RunID] == follow.task {
				delete(c.follows, ref.RunID)
			}
			close(follow.task.done)
			c.mu.Unlock()
		}()
		timer := time.NewTimer(c.followEvery)
		defer timer.Stop()
		for {
			select {
			case <-follow.ctx.Done():
				return
			case <-timer.C:
			}
			snapshot, err := c.Refresh(follow.ctx, ref)
			if err != nil || !runtimeFollowableInternal(snapshot) {
				return
			}
			timer.Reset(c.followEvery)
		}
	}()
}

func (c *RuntimeRemoteCoordinator) WaitDispatch(ctx context.Context, runID string) error {
	c.mu.Lock()
	task := c.dispatch[runID]
	c.mu.Unlock()
	if task == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-task.done:
		return nil
	}
}
func (c *RuntimeRemoteCoordinator) Snapshot(ctx context.Context, ref RuntimeRunReference) (activitytypes.RuntimeSnapshot, error) {
	s := c.service
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	return s.boundSnapshotInternal(ctx, ref)
}
func (c *RuntimeRemoteCoordinator) InvalidateNegativeContinuity() {
	c.mu.Lock()
	clear(c.stamps)
	c.mu.Unlock()
}
func (c *RuntimeRemoteCoordinator) Close(ctx context.Context) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	return c.closeInternal(ctx)
}

func (c *RuntimeRemoteCoordinator) closeInternal(ctx context.Context) error {
	c.mu.Lock()
	c.paused = true
	var tasks []*runtimeRemoteTaskInternal
	for _, task := range c.dispatch {
		task.cancel()
		tasks = append(tasks, task)
	}
	for _, task := range c.queries {
		task.cancel()
		tasks = append(tasks, task)
	}
	for _, task := range c.follows {
		task.cancel()
		tasks = append(tasks, task)
	}
	c.mu.Unlock()
	for _, task := range tasks {
		// A task that already ended never turns an expired budget into an error.
		select {
		case <-task.done:
			continue
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-task.done:
		}
	}
	return nil
}

func (c *RuntimeRemoteCoordinator) forgetTerminalInternal(runID string) {
	c.mu.Lock()
	delete(c.stamps, runID)
	delete(c.lastQuery, runID)
	c.mu.Unlock()
}
