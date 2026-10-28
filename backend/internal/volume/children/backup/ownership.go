// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original non-waiting durable-admission then workspace-lock ownership.

package backup

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/volume/operationlock"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"uuid"
)

type operationOwnership struct {
	release func()
	ctx     context.Context
}

func (o *operationOwnership) Release(context.Context) {
	if o != nil {
		o.release()
	}
}

func (s *Service) acquireOperation(ctx context.Context, source, destination string) (context.Context, *operationOwnership, error) {
	names := []string{source}
	if destination != "" && destination != source {
		names = append(names, destination)
	}
	slices.Sort(names)
	releases := []func(){}
	release := sync.OnceFunc(func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	})
	for _, name := range names {
		if err := s.checkWriterFence(ctx, name); err != nil {
			release()
			return ctx, nil, err
		}
		lease, ok, err := s.deps.Engine.TryAcquireRun(ctx, backup.VolumeAdmissionScope, name)
		if err != nil {
			release()
			return ctx, nil, dto.NewProblem(503, "volume.backup.engine-unavailable", err)
		}
		if !ok {
			release()
			return ctx, nil, s.busyProblem(ctx, name)
		}
		releases = append(releases, func() { lease.Release(context.WithoutCancel(ctx)) })
		unlock, ok := s.deps.Locks.TryLock(name)
		if !ok {
			release()
			return ctx, nil, dto.NewProblem(409, "volume.backup.busy", s.deps.AlreadyRunning)
		}
		releases = append(releases, unlock)
	}
	ctx = rustic.WithOperation(ctx, source, uuid.New().String())
	ctx, unlock := operationlock.WithContext(ctx, source, release)
	return context.WithValue(ctx, lockHeldKeyInternal{}, lockHeldInternal{s, source}), &operationOwnership{release: unlock, ctx: ctx}, nil
}
func (s *Service) busyProblem(ctx context.Context, name string) error {
	problem := dto.NewProblem(409, "volume.backup.busy", s.deps.AlreadyRunning)
	if s.deps.DB != nil {
		var running struct{ ActivityID string }
		if err := s.deps.DB.WithContext(ctx).Table("volume_backups").Where("volume_name = ? AND status = ?", name, "running").Order("created_at DESC").Take(&running).Error; err == nil {
			problem.ActivityID = running.ActivityID
		}
		if problem.ActivityID == "" {
			_ = s.deps.DB.WithContext(ctx).Table("volume_restore_runs").Where("volume_name = ? AND status = ?", name, "running").Order("started_at DESC").Take(&running).Error
			problem.ActivityID = running.ActivityID
		}
	}
	return problem
}
func (s *Service) owns(ctx context.Context, name string) bool {
	held, _ := ctx.Value(lockHeldKeyInternal{}).(lockHeldInternal)
	return held.service == s && held.volumeName == name
}

// checkWriterFence is replaced by a confirmed startup/admission sweep; an
// unconfirmed writer prevents SQL settlement as well as new admissions.
func (s *Service) checkWriterFence(ctx context.Context, name string) error {
	if s.isWriterFenced(name) {
		if err := s.ReconcileStartup(ctx, false); err != nil {
			return dto.NewProblem(409, "volume.backup.writer-unconfirmed", err)
		}
		if s.isWriterFenced(name) {
			return dto.NewProblem(409, "volume.backup.writer-unconfirmed", nil)
		}
	}
	return ctx.Err()
}
func neutralBackupError(err error, code string) error {
	if err == nil {
		return nil
	}
	if access := agentAccessProblem(err); access != err {
		return access
	}
	var p *dto.Problem
	if errors.As(err, &p) {
		return p
	}
	return dto.NewProblem(500, code, err)
}

// retainWriterFence keeps both operation locks until a later confirmed sweep.
func (s *Service) retainWriterFence(ctx context.Context, name string) {
	s.fenceMu.Lock()
	defer s.fenceMu.Unlock()
	if s.writerFences[name] {
		return
	}
	s.writerFences[name] = true
	delete(s.confirmedWriters, name)
	release := operationlock.Retain(ctx, name)
	if s.writerFenceRelease == nil {
		s.writerFenceRelease = map[string]func(){}
	}
	s.writerFenceRelease[name] = release
}

// agentAccessError carries the host compatibility envelope the environment
// proxy sends for the same verdict, so central backup routes refuse alike.
type agentAccessError struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	status  int
	cause   error
}

func (e *agentAccessError) Error() string  { return e.Code }
func (e *agentAccessError) Unwrap() error  { return e.cause }
func (e *agentAccessError) GetStatus() int { return e.status }

// agentAccessProblem maps an agent access verdict and returns other errors unchanged.
func agentAccessProblem(err error) error {
	if !errors.Is(err, remenv.ErrAgentUpdateRequired) && !errors.Is(err, remenv.ErrAgentInvalidToken) {
		return err
	}
	status, code := remenv.AgentAccessFailure(err)
	return &agentAccessError{Code: code, status: status, cause: err}
}

// remoteBackupJSON keeps the agent access verdict that the shared proxy
// translation reports as an internal error.
func remoteBackupJSON[T any](ctx context.Context, proxy handlerutil.RemoteJSONProxy, environmentID, method, path string, body any) (*T, error) {
	var raw error
	record := handlerutil.RemoteJSONProxy(func(ctx context.Context, id, m, p string, b []byte, out any) error {
		raw = proxy(ctx, id, m, p, b, out)
		return raw
	})
	output, err := record.JSON[T](ctx, environmentID, method, path, body)
	if err != nil && raw != nil {
		if access := agentAccessProblem(raw); access != raw {
			return nil, access
		}
	}
	return output, err
}
