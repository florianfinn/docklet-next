// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Read-only frozen lifecycle preflight for trusted runtime admission.

package project

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"go.getarcane.app/updater/refs"
)

func (s *ProjectService) SetLifecycleChecker(check func(context.Context, string) (*projectDTO.UpdateInfo, error)) {
	s.lifecycleCheck = check
}
func lifecycleObserved(snap *life.Snapshot) string {
	images := map[string]any{}
	for key, img := range snap.Services {
		images[key] = []any{img.ContainerID, img.ContainerName, img.ImageID, img.Present, img.Running}
	}
	return life.Hash([]any{snap.TargetIdentity, images})
}
func (s *ProjectService) lifecycleCurrent(ctx context.Context, id, registration string) (*Project, *life.Snapshot, error) {
	p, err := s.getMutableProject(ctx, id)
	if err != nil {
		return nil, nil, lifecycleFailure("update.target-not-found")
	}
	snap, err := s.lifecycleInputs(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	snap.RegistrationID = registration
	if err = s.lifecycleObserve(ctx, snap); err != nil {
		return nil, nil, err
	}
	return p, snap, nil
}
func uniqueLifecycleReferences(snapshot *life.Snapshot) error {
	refs := map[string]string{}
	for _, img := range snapshot.Services {
		if !img.Present {
			continue
		}
		if old, ok := refs[img.Reference]; ok && old != img.ImageID {
			return lifecycleFailure("restore.unavailable")
		}
		refs[img.Reference] = img.ImageID
	}
	return nil
}
func (s *ProjectService) PrepareImageLifecycle(ctx context.Context, id, registration string, request dto.LifecycleRequest) (*dto.LifecycleIntent, error) {
	_, current, err := s.lifecycleCurrent(ctx, id, registration)
	if err != nil {
		return nil, err
	}
	if _, err = lifecycleOrderedModel(current); err != nil {
		return nil, err
	}
	if request.ExpectedRevision == "" || request.ExpectedRevision != current.SavedRevision {
		return nil, lifecycleFailure("update.target-changed")
	}
	intent := &dto.LifecycleIntent{Request: request, TargetIdentity: current.TargetIdentity, SavedRevision: current.SavedRevision, ObservedRevision: lifecycleObserved(current), Selections: []dto.ImageSelection{}}
	if request.PointID != "" {
		point, saved, err := s.lifecycleStore().Point(ctx, request.PointID)
		if err != nil {
			return nil, err
		}
		if _, err = lifecycleOrderedModel(saved); err != nil {
			return nil, err
		}
		if point.TargetID != id || point.TargetIdentity != current.TargetIdentity || point.RegistrationID != registration {
			return nil, lifecycleFailure("restore.unavailable")
		}
		if !lifecycleFilesEqual(saved, current) && !request.ReplaceConfiguration {
			return nil, lifecycleFailure("update.config-drift")
		}
		if err = uniqueLifecycleReferences(saved); err != nil {
			return nil, err
		}
		if err = s.verifyLifecyclePoint(ctx, point, saved); err != nil {
			return nil, err
		}
		intent.PointRevision = point.Revision
		for _, key := range slices.Sorted(maps.Keys(saved.Services)) {
			img := saved.Services[key]
			if !img.Present {
				continue
			}
			intent.Selections = append(intent.Selections, dto.ImageSelection{Service: key, SourceReference: img.Reference, Reference: img.Reference, Digest: img.Digest, ImageID: img.ImageID, Platform: img.Platform, Kind: "restore"})
		}
		if err = s.checkLifecycleSharing(ctx, current, saved); err != nil {
			return nil, err
		}
		return intent, nil
	}
	baseline, revision, err := s.lifecycleStore().Baseline(ctx, current)
	if err != nil {
		return nil, err
	}
	if baseline.SavedRevision != current.SavedRevision {
		return nil, lifecycleFailure("update.config-drift")
	}
	if lifecycleObserved(baseline) != lifecycleObserved(current) {
		return nil, lifecycleFailure("update.baseline-unavailable")
	}
	if uniqueLifecycleReferences(baseline) != nil {
		return nil, lifecycleFailure("update.baseline-unavailable")
	}
	intent.BaselineRevision = revision
	if s.lifecycleCheck == nil || s.containerRegistryService == nil {
		return nil, lifecycleFailure("update.check-unavailable")
	}
	checked, err := s.lifecycleCheck(ctx, id)
	if err != nil || checked == nil {
		return nil, lifecycleFailure("update.check-unavailable")
	}
	for _, key := range slices.Sorted(maps.Keys(current.Model.Services)) {
		img := current.Services[key]
		row, ok := checked.ServiceUpdates[key]
		if !ok || row.UpdateInfo == nil {
			return nil, lifecycleFailure("update.check-unavailable")
		}
		info := row.UpdateInfo
		if !info.HasUpdate {
			if info.Error != "" || info.State == "unknown" || info.State == "error" {
				return nil, lifecycleFailure("update.check-unavailable")
			}
			continue
		}
		if !img.Present || current.Model.Services[key].Build != nil {
			return nil, lifecycleFailure("update.image-binding-unsupported")
		}
		selected := img.Reference
		kind := "rebuild"
		if info.LatestVersion != "" && info.LatestVersion != info.CurrentVersion {
			normalized, e := refs.NormalizeReference(img.Reference)
			if e != nil {
				return nil, lifecycleFailure("update.image-binding-unsupported")
			}
			selected = normalized.RegistryHost + "/" + normalized.Repository + ":" + info.LatestVersion
			kind = "version"
		}
		if err = guard.ValidateReference(selected); err != nil {
			return nil, err
		}
		platform, err := v1.ParsePlatform(img.Platform)
		if err != nil {
			return nil, lifecycleFailure("update.check-unavailable")
		}
		observation, err := s.containerRegistryService.PlatformImage(ctx, selected, *platform)
		if err != nil {
			return nil, err
		}
		if observation.Digest == "" || observation.ConfigDigest == "" {
			return nil, lifecycleFailure("update.check-unavailable")
		}
		intent.Selections = append(intent.Selections, dto.ImageSelection{Service: key, SourceReference: img.Reference, Reference: selected, Digest: observation.Digest, ImageID: observation.ConfigDigest, Platform: img.Platform, Kind: kind})
	}
	if _, err = lifecycleVersionFiles(current, intent.Selections); err != nil {
		return nil, err
	}
	return intent, nil
}
func immutableLifecycleReference(sel dto.ImageSelection) (string, error) {
	named, err := reference.ParseNormalizedNamed(sel.Reference)
	if err != nil || !strings.HasPrefix(sel.Digest, "sha256:") {
		return "", lifecycleFailure("update.target-changed")
	}
	return reference.TrimNamed(named).String() + "@" + sel.Digest, nil
}
