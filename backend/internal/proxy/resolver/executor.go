// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

package resolver

import (
	"context"
	"errors"
	"time"

	dto "github.com/florianfinn/docklet-next/types/proxy"
)

type Executor struct {
	Host        Host
	Backups     string
	Save        func(context.Context, dto.Operation) error
	AfterCommit func(dto.Phase) error
	AfterEffect func(dto.Phase) error
}

// ErrInterrupted belongs exclusively to the deterministic crash harness.
var ErrInterrupted = errors.New("resolver process interruption")

func code(err error, fallback dto.Reason) dto.Reason {
	if p, ok := errors.AsType[*dto.Problem](err); ok {
		return p.Code
	}
	return fallback
}
func (e *Executor) phase(ctx context.Context, o *dto.Operation, p dto.ResolverPhase) error {
	o.Phase = dto.Phase(p)
	o.Resolver.Phase = p
	if err := e.Save(ctx, *o); err != nil {
		return err
	}
	if e.AfterCommit != nil {
		return e.AfterCommit(o.Phase)
	}
	return nil
}
func (e *Executor) effect(ctx context.Context, o *dto.Operation, p dto.ResolverPhase, fn func() error) error {
	if err := e.phase(ctx, o, p); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	if e.AfterEffect != nil {
		return e.AfterEffect(dto.Phase(p))
	}
	return nil
}
func (e *Executor) finish(ctx context.Context, o dto.Operation, state dto.State, reason dto.Reason) (dto.Operation, error) {
	o.State, o.Reason = state, reason
	if reason == dto.ComposeLoadFailed || reason == dto.ComposeUpFailed || reason == dto.StaticParseFailed {
		o.Resolver.Diagnostic = &dto.ResolverDiagnostic{Code: reason}
	}
	if state == dto.RecoveryRequired {
		if t, err := Load(e.Backups, o.ID, o.Resolver.Files); err == nil {
			if current, err := Current(t); err == nil {
				o.Current = &current
			}
		}
	}
	p := dto.ResolverRecoveryRequired
	switch state {
	case dto.Applied:
		p = dto.ResolverApplied
	case dto.RolledBack:
		p = dto.ResolverRolledBack
	case dto.Refused:
		p = dto.ResolverRefused
	case dto.Released:
		p = o.Resolver.Phase
	}
	if state.Final() {
		now := time.Now().UTC()
		o.FinishedAt = &now
	} else {
		o.FinishedAt = nil
	}
	if err := e.phase(ctx, &o, p); err != nil {
		return o, err
	}
	// Receipts are durable before backup deletion. Cleanup failure can safely be
	// retried at startup or on a final receipt query without any workload effect.
	if state.Final() {
		if err := Remove(e.Backups, o.ID); err != nil {
			return o, dto.Fail(dto.BackupInvalid, 503)
		}
	}
	return o, nil
}
func (e *Executor) Run(ctx context.Context, q dto.ResolverApplyRequest, o dto.Operation) (dto.Operation, error) {
	t, err := e.Host.Prepare(ctx, q)
	if err != nil {
		if l, ok := errors.AsType[*LayoutError](err); ok {
			o.Resolver.LayoutReason = l.Reason
			return e.finish(ctx, o, dto.Refused, dto.UnsupportedLayout)
		}
		return e.finish(ctx, o, dto.Refused, code(err, dto.ComposeLoadFailed))
	}
	o.Resolver.Files = []dto.ResolverFile{}
	for _, f := range t.Files {
		o.Resolver.Files = append(o.Resolver.Files, f.ResolverFile)
	}
	err = e.effect(ctx, &o, dto.ResolverBackup, func() error { return Publish(e.Backups, o.ID, t) })
	if errors.Is(err, ErrInterrupted) {
		return o, err
	}
	if err != nil {
		return e.finish(ctx, o, dto.Refused, dto.BackupInvalid)
	}
	written := 0
	err = e.effect(ctx, &o, dto.ResolverWriting, func() error {
		if err := e.Host.Preflight(ctx, t); err != nil {
			return err
		}
		current, err := e.Host.Replacement(ctx, t, q.Instance)
		if err != nil {
			return err
		}
		if current.Identity.ContainerID != q.Instance.Identity.ContainerID || current.Identity.StartedAt != q.Instance.Identity.StartedAt {
			return dto.Fail(dto.ContainerChanged, 409)
		}
		// Read every target before the first write so third versions cannot leave
		// an otherwise untouched multi-file plan partially installed.
		for _, f := range t.Files {
			b, er := Read(t.Directory, f.Path)
			if er != nil {
				return er
			}
			if b.Old != f.Old {
				return dto.Fail(dto.ThirdVersion, 409)
			}
		}
		for _, f := range t.Files {
			if f.Old == f.New {
				continue
			}
			mode := f.Mode
			if f.WriteMode != 0 {
				mode = f.WriteMode
			}
			if er := Replace(t.Directory, f.Path, f.Old, f.After, f.New.Present, mode); er != nil {
				return er
			}
			written++
			if e.AfterEffect != nil {
				if er := e.AfterEffect(dto.Phase(dto.ResolverWriting)); er != nil {
					return er
				}
			}
		}
		return nil
	})
	if errors.Is(err, ErrInterrupted) {
		return o, err
	}
	if err != nil {
		if written == 0 {
			return e.finish(ctx, o, dto.Refused, code(err, dto.TargetUnclear))
		}
		return e.restore(ctx, q.Instance, o, t, code(err, dto.RestoreFailed))
	}
	o.Resolver.RecreateCommitted = true
	start := time.Now().UTC()
	o.Resolver.RecreateStartedAt = &start
	err = e.effect(ctx, &o, dto.ResolverRecreating, func() error { return e.Host.Recreate(ctx, t) })
	if errors.Is(err, ErrInterrupted) {
		return o, err
	}
	if err != nil {
		return e.restore(ctx, q.Instance, o, t, code(err, dto.ComposeUpFailed))
	}
	replacement, err := e.Host.Replacement(ctx, t, q.Instance)
	if err != nil {
		return e.finish(ctx, o, dto.RecoveryRequired, code(err, dto.IdentityUnclear))
	}
	err = e.effect(ctx, &o, dto.ResolverObserving, func() error { return e.Host.Observe(ctx, replacement, q.Request.Name, true, start) })
	if errors.Is(err, ErrInterrupted) {
		return o, err
	}
	if err != nil {
		return e.restore(ctx, q.Instance, o, t, code(err, dto.ProxyRejected))
	}
	for _, file := range t.Files {
		current, err := Read(t.Directory, file.Path)
		if err != nil {
			return e.finish(ctx, o, dto.RecoveryRequired, dto.TargetUnclear)
		}
		if current.Old != file.New {
			return e.finish(ctx, o, dto.RecoveryRequired, dto.ThirdVersion)
		}
	}
	o.Resolver.Replacement = receiptReplacement(t, replacement)
	return e.finish(ctx, o, dto.Applied, "")
}
func receiptReplacement(t Target, i dto.Instance) *dto.ResolverReplacement {
	return &dto.ResolverReplacement{ContainerID: i.Identity.ContainerID, Name: i.Identity.ContainerName, StartedAt: i.Identity.StartedAt, Project: t.Project, Service: t.Service, MountTarget: i.Source.MountTarget, Canonical: i.Source.Canonical, Kind: i.Source.Kind}
}
func (e *Executor) Resume(ctx context.Context, envelope dto.ResolverEnvelope, o dto.Operation) (dto.Operation, error) {
	if o.State.Final() {
		return o, Remove(e.Backups, o.ID)
	}
	if o.State == dto.RecoveryRequired {
		return o, nil
	}
	if o.Resolver.Phase == dto.ResolverAdmitted {
		return e.finish(ctx, o, dto.Refused, dto.Interrupted)
	}
	t, err := Load(e.Backups, o.ID, o.Resolver.Files)
	if err != nil {
		if o.Resolver.Phase == dto.ResolverBackup {
			return e.finish(ctx, o, dto.Refused, dto.Interrupted)
		}
		return e.finish(ctx, o, dto.RecoveryRequired, dto.BackupInvalid)
	}
	return e.restore(ctx, envelope.Instance, o, t, dto.Interrupted)
}
func (e *Executor) restore(ctx context.Context, original dto.Instance, o dto.Operation, t Target, reason dto.Reason) (dto.Operation, error) {
	if o.Resolver.RecreateCommitted {
		if err := e.Host.Settle(ctx, t); err != nil {
			if code(err, "") == dto.AgentUnavailable {
				return o, err
			}
			return e.finish(ctx, o, dto.RecoveryRequired, code(err, dto.EffectUnsettled))
		}
	}

	// Re-read and validate the entire backup before touching even the first target.
	checked, err := Load(e.Backups, o.ID, o.Resolver.Files)
	if err != nil {
		return e.finish(ctx, o, dto.RecoveryRequired, dto.BackupInvalid)
	}
	t = checked
	if err := e.Host.Preflight(ctx, t); err != nil {
		if code(err, "") == dto.AgentUnavailable {
			return o, err
		}
		return e.finish(ctx, o, dto.RecoveryRequired, code(err, dto.TargetUnclear))
	}
	states := make([]dto.FileState, len(t.Files))
	for j, f := range t.Files {
		b, err := Read(t.Directory, f.Path)
		if _, refused := errors.AsType[*dto.Problem](err); refused {
			// Changed metadata, type or size is a third version, not a transient read.
			return e.finish(ctx, o, dto.RecoveryRequired, dto.ThirdVersion)
		}
		if err != nil {
			return o, dto.Fail(dto.AgentUnavailable, 503)
		} // transient I/O: retry, same journal
		states[j] = b.Old
		if b.Old != f.Old && b.Old != f.New {
			return e.finish(ctx, o, dto.RecoveryRequired, dto.ThirdVersion)
		}
	}
	err = e.effect(ctx, &o, dto.ResolverRestoring, func() error {
		for j, f := range t.Files {
			if states[j] == f.Old {
				continue
			}
			if err := Replace(t.Directory, f.Path, states[j], f.Before, f.Old.Present, f.Mode); err != nil {
				return err
			}
			if e.AfterEffect != nil {
				if err := e.AfterEffect(dto.Phase(dto.ResolverRestoring)); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if errors.Is(err, ErrInterrupted) {
		return o, err
	}
	if err != nil {
		return e.finish(ctx, o, dto.RecoveryRequired, code(err, dto.RestoreFailed))
	}
	if !o.Resolver.RecreateCommitted {
		current, err := e.Host.Replacement(ctx, t, original)
		if err != nil {
			return e.finish(ctx, o, dto.RecoveryRequired, code(err, dto.IdentityUnclear))
		}
		if current.Identity == original.Identity {
			return e.finish(ctx, o, dto.RolledBack, reason)
		}
	}
	start := time.Now().UTC()
	if err = e.Host.Recreate(ctx, t); err != nil {
		return e.finish(ctx, o, dto.RecoveryRequired, dto.RestoreFailed)
	}
	replacement, err := e.Host.Replacement(ctx, t, original)
	if err != nil {
		return e.finish(ctx, o, dto.RecoveryRequired, dto.IdentityUnclear)
	}
	if err = e.Host.Observe(ctx, replacement, o.Resolver.Intent.Name, false, start); err != nil {
		return e.finish(ctx, o, dto.RecoveryRequired, dto.RestoreFailed)
	}
	o.Resolver.Replacement = receiptReplacement(t, replacement)
	return e.finish(ctx, o, dto.RolledBack, reason)
}
func (e *Executor) Release(ctx context.Context, envelope dto.ResolverEnvelope, o dto.Operation, q dto.ReleaseRequest) (dto.Operation, error) {
	if o.State.Final() {
		return o, Remove(e.Backups, o.ID)
	}
	if o.State != dto.RecoveryRequired {
		return o, dto.Fail(dto.OperationConflict, 409)
	}
	// Confirmation is the hash of the complete current file-state vector. It
	// includes presence and path; a one-file confirmation cannot release a plan.
	t, err := e.Confirmation(ctx, envelope, o)
	if err != nil {
		return o, err
	}
	state, err := Current(t)
	if err != nil {
		return o, err
	}
	if q.Confirmed != state {
		return o, dto.Fail(dto.TargetChanged, 409)
	}
	o.Current = &state
	o.ReleasedBy = q.Actor
	return e.finish(ctx, o, dto.Released, "")
}

func (e *Executor) Confirmation(ctx context.Context, en dto.ResolverEnvelope, o dto.Operation) (Target, error) {
	if reader, ok := e.Host.(interface {
		Confirmation(context.Context, dto.Instance, []dto.ResolverFile) (Target, error)
	}); ok {
		return reader.Confirmation(ctx, en.Instance, o.Resolver.Files)
	}
	return Load(e.Backups, o.ID, o.Resolver.Files)
}
