// Source: docklet-next/issues/440
// Revision: 92ca1a3ea43cfdc211ca561a968e09c8e8f747b1
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original conservative recovery from the observed stack run's own retained point.

package project

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"

	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"go.getarcane.app/acfs"
)

// recoverOwnPoint returns only uncertain errors to the wrapper. Known recovery
// outcomes leave the original update failure and its runtime status intact.
func (s *ProjectService) recoverOwnPoint(ctx context.Context, run, registration string, execution *lifecycleExecution, intended *life.Snapshot, actor user.Actor, result *dto.LifecycleResult) (uncertain error) {
	execution.recoveryCancel = ctx
	effectCtx := context.WithoutCancel(ctx)
	finish := func(err error) error {
		if lifecycleUncertain(err) {
			result.Recovery = nil
			return err
		}
		switch {
		case ctx.Err() != nil:
			result.Recovery = &dto.RecoveryResult{Outcome: "interrupted"}
		case execution.recoveryChanged:
			result.Recovery = &dto.RecoveryResult{Outcome: "restore-failed", Code: imageDTO.Neutral(err, "restore.deploy-failed", 500).Problem.Code}
		default:
			code := imageDTO.Neutral(err, "restore.unavailable", 409).Problem.Code
			if code != "restore.files-changed" && code != "restore.shared-reference" && code != "image.busy" {
				code = "restore.unavailable"
			}
			result.Recovery = &dto.RecoveryResult{Outcome: "unavailable", Code: code}
		}
		return nil
	}
	if err := ctx.Err(); err != nil {
		return finish(err)
	}
	store := s.lifecycleStore()
	point, saved, err := store.Point(ctx, execution.point.ID)
	if err != nil || point.ID != result.RestorePointID || point.TargetID != execution.point.TargetID || point.TargetIdentity != execution.point.TargetIdentity || point.RegistrationID != registration || point.SourceRunID != run || point.Revision != execution.point.Revision || saved == nil || life.Hash(saved) != point.Revision {
		return finish(lifecycleFailure("restore.unavailable"))
	}
	p, err := s.getMutableProject(ctx, point.TargetID)
	if err != nil || targetLifecycleIdentity(p) != point.TargetIdentity || intended == nil {
		return finish(lifecycleFailure("restore.unavailable"))
	}
	files, err := recoveryFiles(ctx, p, saved, intended)
	if err != nil {
		return finish(err)
	}
	// Capture a fresh source after the failure. In particular, a completed deploy
	// has new container IDs and cannot use the old source as its create barrier.
	_, source, err := s.lifecycleCurrent(ctx, point.TargetID, registration)
	if err != nil || life.Hash(files) != life.Hash(source.Files) {
		return finish(lifecycleFailure("restore.files-changed"))
	}
	if source.TargetIdentity != point.TargetIdentity {
		return finish(lifecycleFailure("restore.unavailable"))
	}
	if err = s.preflightLifecycleWrites(ctx, p, saved); err != nil {
		return finish(err)
	}
	if err = uniqueLifecycleReferences(saved); err != nil {
		return finish(err)
	}
	release, err := s.imageService.ImageGuard().Acquire(ctx)
	if err != nil {
		return finish(err)
	}
	err = func() error {
		defer release()
		if err := s.verifyLifecyclePoint(ctx, point, saved); err != nil {
			return err
		}
		if err := s.checkLifecycleSharing(ctx, source, saved); err != nil {
			return err
		}
		selections := []dto.ImageSelection{}
		keys := []string{}
		for key := range saved.Services {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			img := saved.Services[key]
			if img.Present {
				selections = append(selections, dto.ImageSelection{Service: key, SourceReference: img.Reference, Reference: img.Reference, Digest: img.Digest, ImageID: img.ImageID, Platform: img.Platform, Kind: "restore"})
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return s.lifecycleSelectImages(effectCtx, run, saved, selections, source.SavedRevision, lifecycleObserved(source))
	}()
	if err != nil {
		return finish(err)
	}
	if err = ctx.Err(); err != nil {
		return finish(err)
	}
	if !lifecycleFilesEqual(source, saved) {
		if err = s.lifecycleWrite(effectCtx, p, source, saved, actor); err != nil {
			return finish(err)
		}
	}
	if err = ctx.Err(); err != nil {
		return finish(err)
	}
	fresh, err := s.lifecycleInputs(ctx, p)
	if err != nil || !lifecycleFilesEqual(fresh, saved) {
		return finish(lifecycleFailure("restore.files-changed"))
	}
	saved.Model, saved.SavedRevision = fresh.Model, fresh.SavedRevision
	if execution.enteredDeploy {
		if err = ctx.Err(); err != nil {
			return finish(err)
		}
		if err = s.deployLifecycle(effectCtx, saved, true, source); err != nil {
			return finish(err)
		}
	}
	if err = ctx.Err(); err != nil {
		return finish(err)
	}
	_, restored, err := s.lifecycleCurrent(ctx, point.TargetID, registration)
	if err != nil || !lifecycleFilesEqual(restored, saved) {
		return finish(lifecycleFailure("restore.image-mismatch"))
	}
	if err = s.verifyLifecycleReferences(ctx, saved); err != nil {
		return finish(lifecycleFailure("restore.image-mismatch"))
	}
	if execution.enteredDeploy {
		if err = verifyRecoveredLifecycle(saved, restored); err != nil {
			return finish(err)
		}
		verdict := s.observeLifecycle(ctx, saved, true, nil)
		if !verdict.Pass {
			return finish(lifecycleFailure("restore.deploy-failed"))
		}
	}
	if err = ctx.Err(); err != nil {
		return finish(err)
	}
	// Persist the engine state after the observation, including completed jobs.
	_, restored, err = s.lifecycleCurrent(ctx, point.TargetID, registration)
	if err != nil || !lifecycleFilesEqual(restored, saved) {
		return finish(lifecycleFailure("restore.image-mismatch"))
	}
	if execution.enteredDeploy {
		if err = verifyRecoveredLifecycle(saved, restored); err != nil {
			return finish(err)
		}
	}
	restored.RunID = run
	if err = store.SaveBaseline(effectCtx, restored); err != nil {
		if lifecycleUncertain(err) {
			return finish(err)
		}
		result.BaselineCode = "update.baseline-unavailable"
		_ = store.Invalidate(effectCtx, point.TargetID)
	}
	result.Recovery = &dto.RecoveryResult{Outcome: "restored"}
	return nil
}

func verifyRecoveredLifecycle(saved, restored *life.Snapshot) error {
	for key, expected := range saved.Services {
		actual := restored.Services[key]
		if expected.Present != actual.Present || expected.Present && (!lifecycleMatchesExpected(actual, expected) || !expected.Running && actual.Running) {
			return lifecycleFailure("restore.image-mismatch")
		}
	}
	for _, actual := range restored.Containers {
		if actual.OneOff {
			continue
		}
		expected, modeled := saved.Services[actual.Service]
		if modeled && (!expected.Present || !lifecycleMatchesExpected(actual, expected) || !expected.Running && actual.Running) {
			return lifecycleFailure("restore.image-mismatch")
		}
	}
	return nil
}

// Read the complete union before parsing YAML or changing any tag or file. An
// unknown byte sequence, including an unchanged input, belongs to another writer.
func recoveryFiles(ctx context.Context, p *Project, before, intended *life.Snapshot) ([]life.File, error) {
	pre, post := map[string]life.File{}, map[string]life.File{}
	paths := []string{}
	for _, f := range before.Files {
		pre[f.Path] = f
		paths = append(paths, f.Path)
	}
	for _, f := range intended.Files {
		post[f.Path] = f
		paths = append(paths, f.Path)
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	files := []life.File{}
	for _, path := range paths {
		content, err := acfs.ReadFile(ctx, p.Path, "/"+path)
		present := !errors.Is(err, os.ErrNotExist)
		if !present {
			content, err = nil, nil
		}
		actual := life.File{Path: path, Content: content, Present: present}
		matches := func(want life.File) bool {
			return actual.Present == want.Present && bytes.Equal(actual.Content, want.Content)
		}
		if err != nil || !matches(pre[path]) && !matches(post[path]) {
			return nil, lifecycleFailure("restore.files-changed")
		}
		files = append(files, actual)
	}
	return files, nil
}
