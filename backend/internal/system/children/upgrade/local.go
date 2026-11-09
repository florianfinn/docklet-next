// Source: getarcaneapp/arcane/backend/internal/system/children/upgrade/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; replace Arcane upgrader and fleet jobs with correlated Compose self-updates; fallback container search uses the Docklet system label (#235).

package upgrade

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/containerd/errdefs"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	dto "github.com/florianfinn/docklet-next/types/system"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"gorm.io/gorm"
)

func (s *Service) StartUpdate(ctx context.Context, id string, input dto.SelfUpdateEnvelope, userID, username string) (dto.SelfUpdateResponse, error) {
	if id != "0" {
		return s.startRemote(ctx, id, input, userID, username)
	}
	if s.role() == "hub" {
		if input.OpID != "" || input.TargetReference != "" || input.HubVersion != "" {
			return dto.SelfUpdateResponse{}, core.Refuse("update.invalid-target")
		}
		check, err := s.Check(ctx, id)
		if err != nil {
			return dto.SelfUpdateResponse{}, err
		}
		if check.Eligibility.State != "available" {
			code := check.Eligibility.Code
			if code == "" {
				code = "update.invalid-target"
			}
			return dto.SelfUpdateResponse{}, core.Refuse(code)
		}
		candidate := check.Eligibility.Candidate
		if input.TargetVersion != "" && input.TargetVersion != candidate.Version {
			return dto.SelfUpdateResponse{}, core.Refuse("update.invalid-target")
		}
		ref, err := s.resolve(ctx, repository("hub"), candidate.Version, s.transport)
		if err != nil {
			return dto.SelfUpdateResponse{}, err
		}
		input = dto.SelfUpdateEnvelope{OpID: uuid.NewString(), TargetVersion: candidate.Version, TargetReference: ref, HubVersion: s.runningVersion()}
	} else {
		// Idempotency is checked before the newer-than-running-version rule: after
		// replacement the same accepted operation is still a read-only response.
		previous, err := s.store.Read(ctx)
		if err != nil {
			return dto.SelfUpdateResponse{}, err
		}
		if previous != nil && previous.OpID == input.OpID {
			if previous.TargetRef != input.TargetReference || previous.TargetVersion != input.TargetVersion {
				return dto.SelfUpdateResponse{}, core.Refuse("update.invalid-target")
			}
			return response(*previous), nil
		}
		if err := core.ValidateEnvelope(input, s.runningVersion(), repository("agent")); err != nil {
			return dto.SelfUpdateResponse{}, err
		}
		check, err := s.Check(ctx, id)
		if err != nil {
			return dto.SelfUpdateResponse{}, err
		}
		if check.Eligibility.State != "current" && check.Eligibility.State != "available" {
			code := check.Eligibility.Code
			if code == "" {
				code = "update.invalid-target"
			}
			return dto.SelfUpdateResponse{}, core.Refuse(code)
		}
	}
	if s.backupBusy != nil {
		busy, err := s.backupBusy(ctx)
		if err != nil {
			return dto.SelfUpdateResponse{}, err
		}
		if busy {
			return dto.SelfUpdateResponse{}, core.Refuse("host.busy")
		}
	}
	current, _, err := s.inspect(ctx)
	if err != nil {
		return dto.SelfUpdateResponse{}, err
	}
	replay, err := core.ReplayFromLabels(current.Config.Labels, s.role(), current.Config.Image)
	if err != nil {
		return dto.SelfUpdateResponse{}, err
	}
	mounts, err := core.HelperMounts(current, s.endpoint(), s.data, replay)
	if err != nil {
		return dto.SelfUpdateResponse{}, err
	}
	op := core.Operation{OpID: input.OpID, Role: s.role(), EnvironmentID: "0", OldContainerID: current.ID, TargetRef: input.TargetReference, TargetVersion: input.TargetVersion, FromVersion: s.runningVersion(), Deadline: time.Now().UTC().Add(core.OperationTimeout)}
	op, created, err := s.reserveRun(ctx, s.store, op, userID, username)
	if err != nil {
		return dto.SelfUpdateResponse{}, err
	}
	if created {
		request := core.Request{OpID: op.OpID, Role: op.Role, OldContainerID: op.OldContainerID, Replay: replay, TargetReference: op.TargetRef, TargetVersion: op.TargetVersion, Deadline: op.Deadline}
		s.launch("local-"+op.OpID, func(ctx context.Context) {
			if err := s.dispatchLocal(ctx, op, request, mounts); err != nil {
				// A failed create/start reply can follow a real effect. Only a failure
				// before dispatch is reported as pre-effect by dispatchLocal itself.
			}
			s.observer().Run(ctx)
		})
	}
	return response(op), nil
}
func (s *Service) dispatchLocal(parent context.Context, op core.Operation, request core.Request, mounts []mount.Mount) error {
	ctx, cancel := context.WithDeadline(parent, op.Deadline)
	defer cancel()
	beforeEffect := func(err error) error {
		_ = os.MkdirAll(filepath.Join(s.data, "self-update"), 0750)
		_ = core.WriteJSON(core.ResultPath(s.data, op.OpID), core.Result{OpID: op.OpID, TargetRef: op.TargetRef, Phase: "failed-pre-effect", UpdatedAt: time.Now().UTC()})
		return err
	}
	engine, err := s.docker.GetClient(ctx)
	if err != nil {
		return beforeEffect(err)
	}
	pull, err := engine.ImagePull(ctx, op.TargetRef, client.ImagePullOptions{})
	if err != nil {
		return beforeEffect(err)
	}
	_, pullErr := io.Copy(io.Discard, pull)
	closeErr := pull.Close()
	if err = errors.Join(pullErr, closeErr); err != nil {
		return beforeEffect(err)
	}
	if err = os.MkdirAll(filepath.Join(s.data, "self-update"), 0750); err != nil {
		return beforeEffect(err)
	}
	if err = core.WriteJSON(core.RequestPath(s.data, op.OpID), request); err != nil {
		return beforeEffect(err)
	}
	created, err := engine.ContainerCreate(ctx, core.HelperCreate(request, mounts))
	if err != nil {
		return err
	}
	_, err = engine.ContainerStart(ctx, created.ID, client.ContainerStartOptions{})
	if err != nil {
		return err
	}
	_, err = s.store.Change(ctx, op.OpID, func(_ *gorm.DB, current *core.Operation) error { current.Phase = "helper-started"; return nil })
	return err
}
func (s *Service) helperRunning(ctx context.Context, op core.Operation) (bool, error) {
	engine, err := s.docker.GetClient(ctx)
	if err != nil {
		return false, err
	}
	result, err := engine.ContainerInspect(ctx, core.HelperName(op.OpID), client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return result.Container.State != nil && result.Container.State.Running, nil
}
func (s *Service) cleanup(ctx context.Context, op core.Operation) error {
	engine, err := s.docker.GetClient(ctx)
	if err != nil {
		return err
	}
	_, err = engine.ContainerRemove(ctx, core.HelperName(op.OpID), client.ContainerRemoveOptions{Force: true})
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return core.RemoveRequest(s.data, op.OpID)
}
func (s *Service) observer() core.Observer {
	return core.Observer{Store: s.store, Data: s.data, Version: func() string { return s.runningVersion() }, Running: s.helperRunning, Cleanup: s.cleanup, Settle: s.settleRun}
}
