// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy contract foundation.

package proxy

import (
	"context"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"gorm.io/gorm"
	"log/slog"
	"time"
)

func (a *Agent) activateObserve(ctx context.Context, i Intent, o dto.Operation, e Engine, activate bool) (dto.Operation, error) {
	var err error
	if activate && i.Reload {
		err = a.effect(ctx, i, &o, dto.Reloading, func() error { return e.Reload(ctx, i) })
	}
	if activate && i.Restart {
		err = a.effect(ctx, i, &o, dto.Restarting, func() error { return e.Restart(ctx, i) })
	}
	if err != nil {
		if errors.Is(err, errCrash) {
			return o, err
		}
		o = withDiagnostic(o, err)
		r := reason(err, dto.ProxyRejected)
		if r == dto.ExecRunning {
			return a.finish(ctx, i, o, dto.RecoveryRequired, r, "")
		}
		return a.restore(ctx, i, o, e, r)
	}
	epoch, changed, unknown := a.observedEpoch(ctx, i.Request.Instance)
	if changed || (!i.Restart && !unknown && epoch != i.Request.Instance.Identity.StartedAt) {
		return a.finish(ctx, i, o, dto.AppliedUnverified, dto.ContainerChanged, "")
	}
	var observed Observation
	err = a.effect(ctx, i, &o, dto.Observing, func() error { var er error; observed, er = e.Observe(ctx, i, false); return er })
	if err == nil && observed.NeedsMarker && i.Request.Instance.Kind != dto.Traefik {
		r := observed.Reason
		if r == "" {
			r = dto.NoObservableDifference
		}
		observed = Observation{State: dto.AppliedUnverified, Reason: r}
	}
	if err == nil && observed.NeedsMarker {
		err = a.effect(ctx, i, &o, dto.Marking, func() error {
			if er := e.Mark(ctx, i); er != nil {
				return er
			}
			var er error
			observed, er = e.Observe(ctx, i, true)
			return er
		})
	}
	if errors.Is(err, errCrash) {
		return o, err
	}
	if err != nil {
		o = withDiagnostic(o, err)
		r := reason(err, dto.NotObservable)
		if r == dto.ExecRunning {
			return a.finish(ctx, i, o, dto.RecoveryRequired, r, "")
		}
		return a.finish(ctx, i, o, dto.AppliedUnverified, r, "")
	}
	current, changed, unread := a.observedEpoch(ctx, i.Request.Instance)
	if changed || (!unknown && !unread && current != epoch) {
		return a.finish(ctx, i, o, dto.AppliedUnverified, dto.ContainerChanged, "")
	}
	if observed.Restore {
		return a.restore(ctx, i, o, e, observed.Reason)
	}
	if unknown || unread { // identity unconfirmed: never verified, never a claimed change
		return a.finish(ctx, i, o, dto.AppliedUnverified, dto.NotObservable, "")
	}
	if observed.State != dto.Verified && observed.State != dto.AppliedUnverified {
		return a.finish(ctx, i, o, dto.RecoveryRequired, dto.InvalidInput, "")
	}
	return a.finish(ctx, i, o, observed.State, observed.Reason, observed.Evidence)
}
func (a *Agent) restore(ctx context.Context, i Intent, o dto.Operation, e Engine, r dto.Reason) (dto.Operation, error) {
	err := a.effect(ctx, i, &o, dto.Restoring, func() error {
		if er := a.Check(ctx, i.Request.Instance); er != nil {
			return er
		}
		current, er := a.Runtime.Target(ctx, i.Request.Instance, i.Request.Target)
		if er != nil {
			return er
		}
		if current.FileState != i.Old.FileState && current.FileState != i.New.FileState {
			return dto.Fail(dto.TargetChanged, 409)
		}
		if current.FileState != i.Old.FileState {
			if er = a.Runtime.Restore(ctx, i); er != nil {
				return er
			}
		}
		if i.Reload {
			return e.Reload(ctx, i)
		}
		if i.Restart {
			return e.Restart(ctx, i)
		}
		return nil
	})
	if errors.Is(err, errCrash) {
		return o, err
	}
	if err != nil {
		return a.finish(ctx, i, withDiagnostic(o, err), dto.RecoveryRequired, reason(err, dto.RestoreFailed), "")
	}
	return a.finish(ctx, i, o, dto.Restored, r, "")
}
func (a *Agent) Query(ctx context.Context, q dto.Query) (dto.Operation, error) {
	a.mu.Lock()
	if err := a.generation(q.Generation); err != nil {
		a.mu.Unlock()
		return dto.Operation{}, err
	}
	j, err := findJournal(ctx, a.DB, q.ID)
	if err != nil {
		a.mu.Unlock()
		if missing(err) {
			return dto.Operation{}, dto.Fail(dto.UnknownOperation, 404)
		}
		return dto.Operation{}, err
	}
	i, o, err := decodeJournal(j)
	if err != nil {
		a.mu.Unlock()
		return o, err
	}
	if q.Registration != j.Registration || q.Instance.ID != i.Request.Instance.ID {
		a.mu.Unlock()
		return o, dto.Fail(dto.RegistrationChanged, 409)
	}
	// Receipts and running work are reported without an identity check; identity
	// guards only new host effects.
	if a.active[q.ID] || o.State.Final() {
		a.mu.Unlock()
		return o, nil
	}
	if o.State == dto.RecoveryRequired {
		a.mu.Unlock()
		if current, er := a.Runtime.Rebind(ctx, i.Request.Instance); er == nil {
			if b, er := a.Runtime.Target(ctx, current, i.Request.Target); er == nil {
				o.Current = &b.FileState
			}
		}
		return o, nil
	}
	a.active[q.ID] = true
	a.mu.Unlock()
	defer a.inactive(q.ID)
	ctx = context.WithoutCancel(ctx)
	if err = a.Check(ctx, i.Request.Instance); err != nil {
		if Transient(err) {
			return o, err // retry on a later query; the journal stays unfinished
		}
		return a.finish(ctx, i, o, dto.RecoveryRequired, reason(err, dto.ContainerChanged), "")
	}
	return a.recover(ctx, i, o)
}
func (a *Agent) recover(ctx context.Context, i Intent, o dto.Operation) (dto.Operation, error) {
	// Remove transient files before every branch, including third states.
	if err := a.cleanup(ctx, i); err != nil {
		return a.finish(ctx, i, withDiagnostic(o, err), dto.RecoveryRequired, reason(err, dto.RestoreFailed), "")
	}
	b, err := a.Runtime.Target(ctx, i.Request.Instance, i.Request.Target)
	if err != nil {
		if Transient(err) {
			return o, err // an unreadable target is not changed bytes
		}
		return a.finish(ctx, i, o, dto.RecoveryRequired, reason(err, dto.TargetChanged), "")
	}
	old, newSide := b.FileState == i.Old.FileState, b.FileState == i.New.FileState
	if !old && !newSide {
		return a.finish(ctx, i, o, dto.RecoveryRequired, dto.TargetChanged, "")
	}
	e, err := a.Engines.For(i.Request.Instance.Kind)
	if err != nil {
		return a.finish(ctx, i, o, dto.RecoveryRequired, dto.EngineUnavailable, "")
	}
	invalidPhase := i.Request.Instance.Kind == dto.Traefik && (o.Phase == dto.Reloading || o.Phase == dto.Restarting) || i.Request.Instance.Kind == dto.Caddy && o.Phase == dto.Marking || i.Restart && o.Phase == dto.Reloading || !i.Restart && o.Phase == dto.Restarting
	if invalidPhase {
		return a.finish(ctx, i, o, dto.RecoveryRequired, dto.JournalLost, "")
	}
	switch o.Phase {
	case dto.Admitted, dto.Staging, dto.Publishing:
		if old {
			return a.finish(ctx, i, o, dto.Rejected, "", "")
		}
		return a.activateObserve(ctx, i, o, e, true)
	case dto.Reloading:
		if old {
			return a.finish(ctx, i, o, dto.RecoveryRequired, dto.TargetChanged, "")
		}
		return a.activateObserve(ctx, i, o, e, true)
	case dto.Restarting:
		if old {
			return a.finish(ctx, i, o, dto.RecoveryRequired, dto.TargetChanged, "")
		}
		running, started, er := a.Runtime.Epoch(ctx, i.Request.Instance)
		if er != nil && Transient(er) {
			return o, er
		}
		if er != nil {
			return a.finish(ctx, i, o, dto.RecoveryRequired, reason(er, dto.ContainerChanged), "")
		}
		if !running {
			return a.restore(ctx, i, o, e, dto.ProxyRejected)
		}
		before, beforeErr := time.Parse(time.RFC3339Nano, i.StartedBefore)
		after, afterErr := time.Parse(time.RFC3339Nano, started)
		if beforeErr != nil || afterErr != nil || !after.After(before) {
			return a.finish(ctx, i, o, dto.RecoveryRequired, dto.ContainerChanged, "")
		}
		return a.activateObserve(ctx, i, o, e, false)
	case dto.Marking:
		if old {
			return a.finish(ctx, i, o, dto.RecoveryRequired, dto.TargetChanged, "")
		}
		return a.activateObserve(ctx, i, o, e, false)
	case dto.Observing:
		if old {
			return a.finish(ctx, i, o, dto.RecoveryRequired, dto.TargetChanged, "")
		}
		return a.activateObserve(ctx, i, o, e, i.Reload)
	case dto.Restoring:
		return a.restore(ctx, i, o, e, dto.ProxyRejected)
	default:
		return a.finish(ctx, i, o, dto.RecoveryRequired, dto.JournalLost, "")
	}
}
func nowUTC() time.Time { return time.Now().UTC() }
func (a *Agent) Settle(ctx context.Context) error {
	var rows []Journal
	if err := a.DB.WithContext(ctx).Where("finished_at IS NULL").Find(&rows).Error; err != nil {
		return err
	}
	// Journal content never fails startup: each row settles on its own, and an
	// undecodable row stays locked until it is queried or released as journal-lost.
	for _, j := range rows {
		i, _, err := decodeJournal(j)
		if err == nil {
			_, err = a.Query(ctx, dto.Query{ID: j.ID, Instance: i.Request.Instance, Generation: j.Generation, Registration: j.Registration})
		}
		if err != nil {
			slog.WarnContext(ctx, "Proxy journal row left for recovery", "operationId", j.ID, "error", err)
		}
	}
	if err := prune(ctx, a.DB, nowUTC()); err != nil {
		slog.WarnContext(ctx, "Proxy journal pruning postponed", "error", err)
	}
	return nil
}
func (a *Agent) Fence(ctx context.Context, q dto.Query) (dto.FenceResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := dto.FenceResult{Generation: a.Generation()}
	if err := a.generation(q.Generation); err != nil {
		return out, err
	}
	// Fencing has no host effect, so it does not depend on the container identity.
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		g, err := databaseGeneration(tx, q.Generation)
		out.Generation = g
		if err != nil {
			return err
		}
		j, err := findJournal(ctx, tx, q.ID)
		if err == nil {
			_, o, e := decodeJournal(j)
			if e != nil {
				return e
			}
			out.Operation = &o
			return nil
		}
		if !missing(err) {
			return err
		}
		var admitted admissionRow
		if er := tx.First(&admitted, "id = ?", q.ID).Error; er == nil {
			return dto.Fail(dto.JournalLost, 409)
		} else if !missing(er) {
			return er
		}
		var f fenceRow
		er := tx.First(&f, "id = ?", q.ID).Error
		if missing(er) {
			er = tx.Create(&fenceRow{ID: q.ID, Generation: a.Generation(), Registration: q.Registration}).Error
		}
		if er == nil {
			out.Fenced = true
		}
		return er
	})
	return out, err
}
func (a *Agent) Release(ctx context.Context, q dto.ReleaseRequest) (dto.Operation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.generation(q.Generation); err != nil {
		return dto.Operation{}, err
	}
	j, err := findJournal(ctx, a.DB, q.ID)
	if missing(err) {
		return dto.Operation{}, dto.Fail(dto.JournalLost, 409)
	}
	if err != nil {
		return dto.Operation{}, err
	}
	i, o, err := decodeJournal(j)
	if err != nil {
		return a.releaseUndecodable(ctx, j, q)
	}
	if q.Registration != j.Registration || q.Instance.ID != i.Request.Instance.ID {
		return o, dto.Fail(dto.RegistrationChanged, 409)
	}
	// A final journal answers with its receipt, which the hub adopts.
	if o.State.Final() {
		return o, nil
	}
	if a.active[q.ID] || o.State != dto.RecoveryRequired {
		return o, dto.Fail(dto.OperationConflict, 409)
	}
	// A recreated container with the same configuration source can confirm the target.
	current, err := a.Runtime.Rebind(ctx, i.Request.Instance)
	if err != nil {
		return o, err
	}
	b, err := a.Runtime.Target(ctx, current, i.Request.Target)
	if err != nil {
		return o, err
	}
	if b.FileState != q.Confirmed {
		return o, dto.Fail(dto.TargetChanged, 409)
	}
	o.Current = &b.FileState
	o.ReleasedBy = q.Actor
	return a.finish(ctx, i, o, dto.Released, "", "")
}

// releaseUndecodable finishes a journal row whose content cannot be decoded,
// without host effects, so its source lock is freed.
func (a *Agent) releaseUndecodable(ctx context.Context, j Journal, q dto.ReleaseRequest) (dto.Operation, error) {
	if q.Registration != j.Registration || a.active[q.ID] {
		return dto.Operation{}, dto.Fail(dto.OperationConflict, 409)
	}
	now := time.Now().UTC()
	o := dto.Operation{ID: j.ID, InstanceID: q.Instance.ID, Generation: j.Generation, Registration: j.Registration, Digest: j.Digest, State: dto.Released, Phase: dto.Admitted, Reason: dto.JournalLost, ReleasedBy: q.Actor, Current: &q.Confirmed, CreatedAt: j.CreatedAt, FinishedAt: &now}
	if j.Phase.Valid() {
		o.Phase = j.Phase
	}
	b, err := marshal(o)
	if err != nil {
		return o, err
	}
	res := a.DB.WithContext(ctx).Model(&Journal{}).Where("id = ? AND generation = ?", j.ID, j.Generation).Updates(map[string]any{"result": b, "state": o.State, "phase": o.Phase, "finished_at": o.FinishedAt})
	if res.Error != nil {
		return o, res.Error
	}
	if res.RowsAffected != 1 {
		return o, dto.Fail(dto.JournalLost, 409)
	}
	return o, nil
}

// Transient reports a failure to look (daemon, transport, context) rather than a typed
// condition the runtime observed; it must never settle an operation by itself.
func Transient(err error) bool {
	_, typed := errors.AsType[*dto.Problem](err)
	return err != nil && !typed && !errors.Is(err, errCrash)
}

// observedEpoch returns the running container's StartedAt. changed is true only for an
// observed difference (typed refusal or a stopped container); unknown marks a transient
// read, which is neither a change nor a confirmed identity.
func (a *Agent) observedEpoch(ctx context.Context, i dto.Instance) (epoch string, changed, unknown bool) {
	running, epoch, err := a.Runtime.Epoch(ctx, i)
	if Transient(err) {
		return "", false, true
	}
	if err != nil || !running {
		return "", true, false
	}
	return epoch, false, false
}
