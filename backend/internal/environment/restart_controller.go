// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original application-owned one-shot agent restart controller.

package environment

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	dto "github.com/florianfinn/docklet-next/types/environment"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const agentRestartDeadline = 3 * time.Minute

type restartController struct {
	active  map[string]bool
	service *EnvironmentService
	ctx     context.Context
	cancel  context.CancelFunc
	now     func() time.Time
	wait    func(context.Context, time.Duration) bool
	mu      sync.Mutex
	workers sync.WaitGroup
	stopped bool
}

type restartOutcome struct {
	state, code  string
	acknowledged *time.Time
	observedAt   time.Time
}
type restartPreparation struct {
	env Environment
	old string
}

type restartWork struct {
	prepared  *restartPreparation
	row       AgentRestartRequest
	recovered bool
	pending   *restartOutcome
}

// StartAgentRestarts repairs persisted runs without replay before intake opens.
func (s *EnvironmentService) StartAgentRestarts(ctx context.Context) error {
	appCtx, cancel := context.WithCancel(ctx)
	c := &restartController{service: s, ctx: appCtx, cancel: cancel, now: time.Now, wait: restartWait}
	c.active = map[string]bool{}
	s.restarts = c
	if err := c.scan(); err != nil {
		cancel()
		s.restarts = nil
		return err
	}
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		for c.wait(c.ctx, time.Second) {
			if err := c.scan(); err != nil && c.ctx.Err() == nil {
				slog.WarnContext(c.ctx, "Agent restart recovery scan will retry", "error", err)
			}
		}
	}()

	return nil
}

func restartWait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// StopAgentRestarts joins all observations and sends before the database closes.
func (s *EnvironmentService) StopAgentRestarts() {
	if s.restarts == nil {
		return
	}
	c := s.restarts
	c.mu.Lock()
	c.stopped = true
	c.cancel()
	c.mu.Unlock()
	c.workers.Wait()
}

// The intake/scan lock closes the post-commit handoff gap. Unclaimed rows from
// ambiguous reservation commits are recovered without ever sending.
func (c *restartController) scan() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return nil
	}
	var rows []AgentRestartRequest
	if err := c.service.db.WithContext(c.ctx).Where("settled_at IS NULL").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		c.launchLocked(&restartWork{row: row, recovered: true})
	}
	return nil
}
func (c *restartController) intake(ctx context.Context, id string) (AgentRestartRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped || c.ctx.Err() != nil {
		return AgentRestartRequest{}, errors.New("restart intake is stopping")
	}
	row, err := c.reserve(ctx, id)
	if err == nil {
		c.launchLocked(&restartWork{row: row})
	}
	return row, err
}
func (c *restartController) launchLocked(work *restartWork) {
	if c.stopped || c.active[work.row.RunID] {
		return
	}
	c.active[work.row.RunID] = true
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		defer func() { c.mu.Lock(); delete(c.active, work.row.RunID); c.mu.Unlock() }()
		for c.ctx.Err() == nil {
			done, err := c.advance(c.ctx, work)
			if err != nil && c.ctx.Err() == nil {
				slog.WarnContext(c.ctx, "Agent restart persistence will retry", "error", err)
			}
			if done || !c.wait(c.ctx, time.Second) {
				return
			}
		}
	}()
}

func (c *restartController) reserve(ctx context.Context, id string) (AgentRestartRequest, error) {
	var row AgentRestartRequest
	if id == "0" {
		return row, hostProblem("agent.restart-unsupported", 409)
	}
	now := c.now().UTC()
	row = AgentRestartRequest{RunID: uuid.NewString(), EnvironmentID: id, RegistrationID: id, State: "reserved", CreatedAt: now, UpdatedAt: now, Deadline: now.Add(agentRestartDeadline)}
	err := c.service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var env Environment
		if err := tx.First(&env, "id=?", id).Error; err != nil {
			return err
		}
		row.CredentialRevision = env.CredentialRevision
		var n int64
		if err := tx.Table("runtime_receipts").Where("environment_id=? AND settled_at IS NULL", id).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return hostProblem("host.busy", 409)
		}
		if err := tx.Model(&AgentRestartRequest{}).Where("environment_id=? AND settled_at IS NULL", id).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return hostProblem("host.busy", 409)
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return AgentRestartRequest{}, err
	}
	return row, nil
}

func (c *restartController) bound(ctx context.Context, row AgentRestartRequest) (*Environment, error) {
	var env Environment
	err := c.service.db.WithContext(ctx).First(&env, "id=?", row.EnvironmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, hostProblem("agent.restart-superseded", 409)
	}
	if err != nil {
		return nil, err
	}
	if env.ID != row.RegistrationID || env.CredentialRevision != row.CredentialRevision {
		return nil, hostProblem("agent.restart-superseded", 409)
	}
	return &env, nil
}

func (c *restartController) advance(ctx context.Context, w *restartWork) (bool, error) {
	if w.row.SettledAt != nil {
		return true, nil
	}
	if w.pending != nil {
		if err := c.persist(ctx, w, w.pending); err != nil {
			return false, err
		}
		w.pending = nil
		return w.row.SettledAt != nil, nil
	}
	env, err := c.bound(ctx, w.row)
	if err != nil {
		var p *dto.HostProblem
		if !errors.As(err, &p) {
			return false, err
		}
		w.pending = &restartOutcome{state: "failed", code: p.Code}
		return c.advance(ctx, w)
	}
	if w.row.State == "reserved" {
		if w.recovered {
			w.pending = &restartOutcome{state: "failed", code: "agent.restart-interrupted"}
			return c.advance(ctx, w)
		}
		if w.prepared == nil && !c.now().Before(w.row.Deadline) {
			w.pending = &restartOutcome{state: "failed", code: "host.unreachable"}
			return c.advance(ctx, w)
		}

		if w.prepared == nil {
			old, observeErr := c.observe(ctx, w.row, *env)
			if observeErr != nil || old == "" {
				w.pending = &restartOutcome{state: "failed", code: "host.unreachable"}
				return c.advance(ctx, w)
			}
			w.prepared = &restartPreparation{env: *env, old: old}
		}
		sent, err := c.markSent(ctx, w, w.prepared.old)
		if err != nil {
			return false, err
		}
		if !sent {
			return w.row.SettledAt != nil, nil
		}

		if w.row.SettledAt != nil {
			return true, nil
		}
		// The only send site follows the durable sent commit. Recovery enters below it.
		sendCtx, cancelSend := context.WithTimeout(ctx, w.row.Deadline.Sub(c.now()))
		response, sendErr := c.service.sendAgentRestart(sendCtx, w.prepared.env)
		cancelSend()
		outcome := &restartOutcome{state: "awaiting"}
		if sendErr == nil && response != nil {
			if response.StatusCode == 202 {
				now := c.now().UTC()
				outcome.acknowledged = &now
			}
			if restartUnsupported(response) {
				outcome.state = "failed"
				outcome.code = "agent.restart-unsupported"
				outcome.observedAt = c.now().UTC()
			}
		}
		w.pending = outcome
		return c.advance(ctx, w)
	}
	if !c.now().Before(w.row.Deadline) {
		state, code := "unknown", "agent.restart-unconfirmed"
		if w.row.AcknowledgedAt != nil {
			state, code = "failed", "agent.restart-timeout"
		}
		w.pending = &restartOutcome{state: state, code: code}
		return c.advance(ctx, w)
	}
	instance, err := c.observe(ctx, w.row, *env)
	if err == nil && instance != "" && instance != w.row.OldInstance {
		// Capture the joined proof before any further binding I/O can fail.
		w.pending = &restartOutcome{state: "connected", observedAt: c.now().UTC()}
	}
	if _, fenceErr := c.bound(ctx, w.row); fenceErr != nil {
		var p *dto.HostProblem
		if !errors.As(fenceErr, &p) {
			return false, fenceErr
		}
		w.pending = &restartOutcome{state: "failed", code: p.Code}
		return c.advance(ctx, w)
	}
	if w.pending == nil {
		return false, nil
	}
	return c.advance(ctx, w)
}

func (c *restartController) observe(ctx context.Context, row AgentRestartRequest, env Environment) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, row.Deadline.Sub(c.now()))
	defer cancel()
	return c.service.restartInstance(probeCtx, env)
}

// Every retry checks the tuple inside the transaction; joined outcomes survive a rollback.
func (c *restartController) persist(ctx context.Context, w *restartWork, out *restartOutcome) error {
	var next AgentRestartRequest
	err := c.service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&next, "run_id=?", w.row.RunID).Error; err != nil {
			return err
		}
		if next.SettledAt != nil {
			return nil
		}
		var env Environment
		err := tx.First(&env, "id=?", next.EnvironmentID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		state, code := out.state, out.code
		if err != nil || env.ID != next.RegistrationID || env.CredentialRevision != next.CredentialRevision {
			state, code = "failed", "agent.restart-superseded"
		}
		now := c.now().UTC()
		if out.acknowledged != nil && out.acknowledged.Before(next.Deadline) {
			next.AcknowledgedAt = out.acknowledged
		}
		lateProof := (state == "connected" || state == "failed" && code == "agent.restart-unsupported") && !out.observedAt.Before(next.Deadline)
		if lateProof || state == "awaiting" && !now.Before(next.Deadline) {
			state, code = "unknown", "agent.restart-unconfirmed"
			if next.AcknowledgedAt != nil {
				state, code = "failed", "agent.restart-timeout"
			}
		}
		next.State = state
		next.UpdatedAt = now
		if state == "connected" || state == "failed" || state == "unknown" {
			next.SettledAt = &now
			if code != "" {
				next.Code = &code
			}
		}
		return tx.Model(&AgentRestartRequest{}).Where("run_id=? AND settled_at IS NULL", next.RunID).Updates(map[string]any{"state": next.State, "code": next.Code, "updated_at": next.UpdatedAt, "acknowledged_at": next.AcknowledgedAt, "settled_at": next.SettledAt}).Error
	})
	if err == nil {
		w.row = next
		c.service.NotifyRuntimeStateChanged()
	}
	return err
}

func (c *restartController) markSent(ctx context.Context, w *restartWork, old string) (bool, error) {
	sent := false
	// A transient write failure returns before the single POST. Evidence is read again on retry.
	var next AgentRestartRequest
	err := c.service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&next, "run_id=?", w.row.RunID).Error; err != nil {
			return err
		}
		if next.State != "reserved" {
			return nil
		}
		var env Environment
		err := tx.First(&env, "id=?", next.EnvironmentID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := c.now().UTC()
		if err != nil || env.ID != next.RegistrationID || env.CredentialRevision != next.CredentialRevision {
			next.State = "failed"
			next.Code = new("agent.restart-superseded")
			next.SettledAt = &now
		} else if !now.Before(next.Deadline) {
			next.State = "failed"
			next.Code = new("host.unreachable")
			next.SettledAt = &now
		} else {
			sent = true
			next.State = "sent"
			next.SentAt = &now
			next.OldInstance = old
		}
		next.UpdatedAt = now
		return tx.Model(&AgentRestartRequest{}).Where("run_id=? AND state='reserved'", next.RunID).Updates(map[string]any{"state": next.State, "old_instance": next.OldInstance, "sent_at": next.SentAt, "updated_at": now, "settled_at": next.SettledAt, "code": next.Code}).Error
	})
	if err == nil {
		w.row = next
	}
	return sent && err == nil, err
}
