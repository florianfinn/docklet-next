// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Runtime-owned stack update/restore; mandatory points and exact local images.

package project

import (
	"context"
	"errors"
	compose "github.com/compose-spec/compose-go/v2/types"
	"slices"
	"strings"
	"time"

	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/client"
	"go.getarcane.app/acfs"
	"os"
	"path/filepath"
)

func (s *ProjectService) ExecuteImageLifecycle(ctx context.Context, id, run, registration string, intent *dto.LifecycleIntent, actor user.Actor) (result *dto.LifecycleResult, resultErr error) {
	result = &dto.LifecycleResult{RunID: run, VolumeDataIncluded: false}
	var rollbackSource, rollbackDesired *life.Snapshot
	selectedImages := false
	defer func() {
		if resultErr != nil && selectedImages && intent != nil && intent.Request.PointID == "" {
			if err := s.reconcileFailedRebuild(context.WithoutCancel(ctx), run, rollbackSource, rollbackDesired, intent.Selections); err != nil {
				resultErr = &dto.UncertainImageEffect{Cause: err}
			}
		}
		if resultErr != nil && result.RestorePointID != "" {
			if err := s.lifecycleStore().FailedAttempt(context.WithoutCancel(ctx), result.RestorePointID); err != nil {
				resultErr = &dto.UncertainImageEffect{Cause: err}
			}
		}
		if resultErr != nil {
			result.HTTPStatus = imageDTO.Neutral(resultErr, "update.deploy-failed", 500).Status
			result.Code = imageDTO.Neutral(resultErr, "update.deploy-failed", 500).Problem.Code
		}
		s.recordLifecycleResult(context.WithoutCancel(ctx), run, result)
	}()
	if actor.ID == "" || actor.ID == user.SystemUser.ID {
		return result, lifecycleFailure("update.invalid-target")
	}
	p, current, err := s.lifecycleCurrent(ctx, id, registration)
	if err != nil {
		return result, err
	}
	if intent == nil || current.TargetIdentity != intent.TargetIdentity || current.SavedRevision != intent.SavedRevision || lifecycleObserved(current) != intent.ObservedRevision {
		return result, lifecycleFailure("update.target-changed")
	}
	selections, err := lifecycleExecutableSelections(current, intent.Selections, intent.Request.PointID != "")
	if err != nil {
		return result, err
	}
	owned := *intent
	owned.Selections = selections
	intent = &owned

	if err = s.preflightLifecycleWrites(ctx, p, current); err != nil {
		return result, err
	}
	var desired, source *life.Snapshot
	source = current
	state := "partial-source"
	store := s.lifecycleStore()
	if intent.Request.PointID != "" {
		point, saved, e := store.Point(ctx, intent.Request.PointID)
		if e != nil {
			return result, e
		}
		if point.TargetID != id || point.TargetIdentity != current.TargetIdentity || point.RegistrationID != registration || point.Revision != intent.PointRevision {
			return result, lifecycleFailure("restore.unavailable")
		}
		if !lifecycleFilesEqual(current, saved) && !intent.Request.ReplaceConfiguration {
			return result, lifecycleFailure("update.config-drift")
		}
		if err = uniqueLifecycleReferences(saved); err != nil {
			return result, err
		}
		if err = s.preflightLifecycleWrites(ctx, p, saved); err != nil {
			return result, err
		}
		desired = saved
		if baseline, _, e := store.Baseline(ctx, current); e == nil && baseline.SavedRevision == current.SavedRevision && lifecycleConfiguration(baseline) == lifecycleConfiguration(current) {
			state = "settled"
		}
	} else {
		baseline, revision, e := store.Baseline(ctx, current)
		if e != nil {
			return result, e
		}
		if revision != intent.BaselineRevision || baseline.SavedRevision != current.SavedRevision {
			return result, lifecycleFailure("update.config-drift")
		}
		if lifecycleConfiguration(baseline) != lifecycleConfiguration(current) {
			return result, lifecycleFailure("update.baseline-unavailable")
		}
		if len(intent.Selections) == 0 {
			return result, nil
		}
		state = "settled"
		desired, err = lifecycleVersionFiles(current, intent.Selections)
		if err != nil {
			return result, err
		}
		// Apply the active configuration even if an init service has exited or
		// its container was removed. The pre-point retains actual source state.
		for key, img := range desired.Services {
			img.Present = true
			desired.Services[key] = img
		}
	}
	if _, err = lifecycleOrderedModel(desired); err != nil {
		return result, err
	}
	release, err := s.imageService.ImageGuard().Acquire(ctx)
	if err != nil {
		return result, err
	}
	leased := true
	defer func() {
		if leased {
			release()
		}
	}()
	if intent.Request.PointID != "" {
		point, _, e := store.Point(ctx, intent.Request.PointID)
		if e != nil {
			return result, e
		}
		if e = s.verifyLifecyclePoint(ctx, point, desired); e != nil {
			return result, e
		}
		if e = s.checkLifecycleSharing(ctx, current, desired); e != nil {
			return result, e
		}
	}
	// Refresh both identity and source revisions after lease acquisition, before
	// any point/tag/file effect. No source/candidate is rediscovered here.
	_, latest, err := s.lifecycleCurrent(ctx, id, registration)
	if err != nil || latest.SavedRevision != current.SavedRevision || lifecycleObserved(latest) != lifecycleObserved(current) {
		return result, lifecycleFailure("update.target-changed")
	}
	source.RunID = run
	point, err := store.Prepare(ctx, source, run, intent.Request.PointID, state)
	if err != nil {
		return result, err
	}
	result.RestorePointID = point.ID
	if err = s.lifecyclePinPoint(ctx, point, source); err != nil {
		return result, err
	}
	rollbackSource, rollbackDesired = current, desired
	selectedImages = true
	if err = s.lifecycleSelectImages(ctx, run, desired, intent.Selections, current.SavedRevision, intent.ObservedRevision); err != nil {
		return result, err
	}
	release()
	leased = false
	if !lifecycleFilesEqual(current, desired) {
		if err = store.Effect(ctx, run); err != nil {
			return result, err
		}
		if err = s.lifecycleWrite(ctx, p, current, desired, actor); err != nil {
			return result, err
		}
	}
	fresh, err := s.lifecycleInputs(ctx, p)
	if err != nil {
		return result, err
	}
	if !lifecycleFilesEqual(fresh, desired) || life.Hash(fresh.Model) != life.Hash(desired.Model) {
		return result, lifecycleFailure("update.target-changed")
	}
	// Deploy from actual freshly loaded files, with complete frozen service IDs.
	desired.Model = fresh.Model
	desired.SavedRevision = fresh.SavedRevision
	if err = s.verifyLifecycleReferences(ctx, desired); err != nil {
		return result, err
	}
	if err = store.Invalidate(ctx, id); err != nil {
		return result, lifecycleFailure("update.persistence-failed")
	}
	if err = store.Effect(ctx, run); err != nil {
		return result, err
	}
	if err = s.deployLifecycle(ctx, desired, intent.Request.PointID != "", current); err != nil {
		var problem *imageDTO.OperationError
		if errors.As(err, &problem) {
			return result, problem
		}
		code := "update.deploy-failed"
		if intent.Request.PointID != "" {
			code = "restore.deploy-failed"
		}
		return result, imageDTO.Failure(code, 500)
	}
	observed := *desired
	if err = s.lifecycleObserve(ctx, &observed); err != nil {
		return result, lifecycleFailure("restore.image-mismatch")
	}
	for key, expected := range desired.Services {
		actual := observed.Services[key]
		if expected.Present && actual.Present && !lifecycleMatchesExpected(actual, expected) {
			code := "update.target-changed"
			if intent.Request.PointID != "" {
				code = "restore.image-mismatch"
			}
			return result, lifecycleFailure(code)
		}
	}
	for _, actual := range observed.Containers {
		expected, modeled := desired.Services[actual.Service]
		if !modeled || !expected.Present || actual.OneOff {
			continue
		}
		if !lifecycleMatchesExpected(actual, expected) {
			code := "update.target-changed"
			if intent.Request.PointID != "" {
				code = "restore.image-mismatch"
			}
			return result, lifecycleFailure(code)
		}
	}
	observed.RunID = run
	observed.RegistrationID = registration
	if err = store.SaveBaseline(ctx, &observed); err != nil {
		result.BaselineCode = "update.baseline-unavailable"
		_ = store.Invalidate(context.WithoutCancel(ctx), id)
	}
	result.Verified = true
	return result, nil
}
func (s *ProjectService) preflightLifecycleWrites(ctx context.Context, p *Project, snap *life.Snapshot) error {
	for _, f := range snap.Files {
		entry, err := acfs.Stat(ctx, p.Path, "/"+f.Path, false)
		if errors.Is(err, os.ErrNotExist) {
			if !f.Present {
				continue
			}
			parent := filepath.ToSlash(filepath.Dir(f.Path))
			if parent == "." {
				parent = ""
			}
			entry, err = acfs.Stat(ctx, p.Path, "/"+parent, false)
			if err == nil && entry.IsDirectory && !entry.IsSymlink && os.FileMode(entry.UnixMode).Perm()&0222 != 0 {
				continue
			}
		}
		if err != nil || entry.IsSymlink || entry.IsDirectory || os.FileMode(entry.UnixMode).Perm()&0222 == 0 {
			return lifecycleFailure("update.image-binding-unsupported")
		}
	}
	return nil
}
func (s *ProjectService) verifyLifecycleReferences(ctx context.Context, snap *life.Snapshot) error {
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return lifecycleFailure("update.target-changed")
	}
	for _, img := range snap.Services {
		if !img.Present {
			continue
		}
		found, e := c.ImageInspect(ctx, img.Reference)
		if e != nil || found.InspectResponse.ID != img.ImageID {
			return lifecycleFailure("update.target-changed")
		}
	}
	return ctx.Err()
}
func (s *ProjectService) deployLifecycle(ctx context.Context, snap *life.Snapshot, restore bool, source *life.Snapshot) error {
	// This model is ephemeral: actual files remain the sole source on later deploy.
	model, orderErr := lifecycleOrderedModel(snap)
	if orderErr != nil {
		return orderErr
	}
	present := []string{}
	stopped := []string{}
	for key, svc := range model.Services {
		expected := snap.Services[key]
		if !expected.Present {
			delete(model.Services, key)
			continue
		}
		if svc.Image == "" {
			svc.Image = expected.Reference
		}
		svc.Platform = expected.Platform
		svc.PullPolicy = "never"
		svc.Build = nil
		for dependency := range svc.DependsOn {
			if !snap.Services[dependency].Present {
				delete(svc.DependsOn, dependency)
			}
		}
		model.Services[key] = svc
		present = append(present, key)
		if restore && !expected.Running {
			stopped = append(stopped, key)
		}
	}
	slices.Sort(present)
	if len(present) > 0 {
		_, err := s.composeCoordinator.Deploy(ctx, projectDTO.ComposeDeployment{ProjectID: snap.TargetID, ProjectPath: model.WorkingDir, Options: &projectDTO.DeployOptions{PullPolicy: "never", ForceRecreate: true}, WaitTimeout: time.Minute, Load: func(context.Context) (*compose.Project, error) {
			if err := s.verifyLifecycleReferences(ctx, snap); err != nil {
				return nil, err
			}
			return model, nil
		}, BeforeCreate: func(ctx context.Context) error {
			p, err := s.getMutableProject(ctx, snap.TargetID)
			if err != nil || targetLifecycleIdentity(p) != snap.TargetIdentity {
				return lifecycleFailure("update.target-changed")
			}
			latest, err := s.lifecycleInputs(ctx, p)
			if err != nil || latest.SavedRevision != snap.SavedRevision {
				return lifecycleFailure("update.target-changed")
			}
			observedSource := *source
			if err = s.lifecycleObserve(ctx, &observedSource); err != nil || lifecycleObserved(&observedSource) != lifecycleObserved(source) {
				return lifecycleFailure("update.target-changed")
			}
			if err = s.verifyLifecycleReferences(ctx, snap); err != nil {
				return err
			}
			return s.lifecycleStopSignals(ctx, source)
		}, ResolveImages: func(context.Context) (projectDTO.ComposeImageOperations, error) {
			return projectDTO.ComposeImageOperations{Exists: func(ctx context.Context, ref string) (bool, error) {
				c, e := s.dockerService.GetClient(ctx)
				if e != nil {
					return false, e
				}
				_, e = c.ImageInspect(ctx, ref)
				return e == nil, e
			}}, nil
		}})
		if err != nil {
			return err
		}
	}
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return err
	}
	if restore {
		listed, e := c.ContainerList(ctx, client.ContainerListOptions{All: true})
		if e != nil {
			return e
		}
		for _, item := range listed.Items {
			if item.Labels["com.docker.compose.project"] != model.Name {
				continue
			}
			if strings.EqualFold(item.Labels["com.docker.compose.oneoff"], "true") {
				continue
			}
			key := item.Labels["com.docker.compose.service"]
			expected := snap.Services[key]
			if !expected.Present {
				if _, owned := source.Model.Services[key]; !owned {
					continue
				}
				prior, captured := source.Containers[item.ID]
				if !captured || prior.OneOff || prior.Service != key || prior.ImageID != item.ImageID {
					return lifecycleFailure("restore.image-mismatch")
				}
			}
			if !expected.Present || slices.Contains(stopped, key) {
				signal := model.Services[key].StopSignal
				if signal == "" {
					signal = lifecycleSourceStopSignal(source, key)
				}
				if _, e = c.ContainerStop(ctx, item.ID, client.ContainerStopOptions{Signal: signal}); e != nil {
					return e
				}
			}
			if !expected.Present {
				if _, e = c.ContainerRemove(ctx, item.ID, client.ContainerRemoveOptions{RemoveVolumes: false, Force: false}); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
func (s *ProjectService) reconcileFailedRebuild(ctx context.Context, run string, before, after *life.Snapshot, selections []dto.ImageSelection) error {
	needs := false
	for _, sel := range selections {
		if sel.Kind == "rebuild" {
			needs = true
		}
	}
	if !needs {
		return nil
	}
	release, err := s.imageService.ImageGuard().Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return err
	}
	for _, sel := range selections {
		if sel.Kind != "rebuild" {
			continue
		}
		old := before.Services[sel.Service]
		current, e := c.ImageInspect(ctx, sel.Reference)
		if e != nil {
			return lifecycleFailure("update.target-changed")
		}
		if current.InspectResponse.ID == old.ImageID {
			continue
		}
		if current.InspectResponse.ID != sel.ImageID {
			return lifecycleFailure("update.target-changed")
		}
		if _, e = c.ImageTag(ctx, client.ImageTagOptions{Source: old.ImageID, Target: old.Reference}); e != nil {
			return lifecycleFailure("image.tag-failed")
		}
		verified, e := c.ImageInspect(ctx, old.Reference)
		if e != nil || verified.InspectResponse.ID != old.ImageID {
			return lifecycleFailure("update.target-changed")
		}
	}
	return nil
}

func (s *ProjectService) recordLifecycleResult(ctx context.Context, run string, result *dto.LifecycleResult) {
	if result != nil && result.RunID == run {
		dto.RecordLifecycleResult(ctx, *result)
	}
}
