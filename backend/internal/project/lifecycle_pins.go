// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Host image lease, verified retention pins and shared-reference restore guard.

package project

import (
	"context"
	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	imageService "github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"io"
	"strings"
)

func (s *ProjectService) verifyLifecyclePoint(ctx context.Context, point life.Point, snap *life.Snapshot) error {
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return lifecycleFailure("restore.unavailable")
	}
	for key, img := range snap.Images() {
		if !img.Present {
			continue
		}
		pin, err := c.ImageInspect(ctx, life.Pin("points", point.ID, key))
		if err != nil || pin.InspectResponse.ID != img.ImageID {
			return lifecycleFailure("restore.unavailable")
		}
		found, err := c.ImageInspect(ctx, img.ImageID)
		if err != nil || found.InspectResponse.ID != img.ImageID {
			return lifecycleFailure("restore.unavailable")
		}
		native, e := identity.Resolve(ctx, c, found.InspectResponse, img.Platform, nil)
		if e != nil || native.Digest() != "" && native.Digest() != img.Digest {
			return lifecycleFailure("restore.unavailable")
		}
		if native.Digest() == "" {
			img.Digest = ""
			if id, ok := strings.CutPrefix(key, "container/"); ok {
				snap.Containers[id] = img
			} else {
				snap.Services[key] = img
			}
		}
	}
	return nil
}
func (s *ProjectService) checkLifecycleSharing(ctx context.Context, current, saved *life.Snapshot) error {
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return lifecycleFailure("restore.shared-reference")
	}
	for _, img := range saved.Services {
		if !img.Present {
			continue
		}
		tag, err := c.ImageInspect(ctx, img.Reference)
		if err == nil && tag.InspectResponse.ID == img.ImageID {
			continue
		}
		if err != nil && !errdefs.IsNotFound(err) {
			return lifecycleFailure("restore.shared-reference")
		}
		listed, err := c.ContainerList(ctx, client.ContainerListOptions{All: true})
		if err != nil {
			return lifecycleFailure("restore.shared-reference")
		}
		for _, workload := range listed.Items {
			if workload.Labels["com.docker.compose.project"] == current.Model.Name {
				continue
			}
			if workload.ImageID == "" || workload.Image == "" {
				return lifecycleFailure("restore.shared-reference")
			}
			if equalLifecycleReference(workload.Image, img.Reference) && workload.ImageID != img.ImageID {
				return lifecycleFailure("restore.shared-reference")
			}
		}
	}
	return nil
}
func equalLifecycleReference(a, b string) bool {
	x, e := reference.ParseNormalizedNamed(a)
	y, f := reference.ParseNormalizedNamed(b)
	return e == nil && f == nil && reference.TagNameOnly(x).String() == reference.TagNameOnly(y).String()
}
func (s *ProjectService) lifecyclePinPoint(ctx context.Context, point life.Point, snapshot *life.Snapshot) error {
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return lifecycleFailure("update.snapshot-unavailable")
	}
	for key, img := range snapshot.Images() {
		if !img.Present {
			continue
		}
		tag := life.Pin("points", point.ID, key)
		if _, err = reference.ParseNormalizedNamed(tag); err != nil {
			return lifecycleFailure("update.snapshot-unavailable")
		}
		if _, err = c.ImageTag(ctx, client.ImageTagOptions{Source: img.ImageID, Target: tag}); err != nil {
			return lifecycleFailure("update.snapshot-unavailable")
		}
	}
	point.Available = true
	if err = s.verifyLifecyclePoint(ctx, point, snapshot); err != nil {
		return lifecycleFailure("update.snapshot-unavailable")
	}
	return s.lifecycleStore().Available(ctx, point.ID)
}
func (s *ProjectService) lifecycleSelectImages(ctx context.Context, run string, desired *life.Snapshot, selections []dto.ImageSelection, expectedRevision, expectedObserved string) error {
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return lifecycleFailure("update.target-changed")
	}
	for _, sel := range selections {
		if sel.Kind != "restore" {
			immutable, err := immutableLifecycleReference(sel)
			if err != nil {
				return err
			}
			opts, err := s.imageService.PullOptionsWithAuth(ctx, immutable, nil)
			if err != nil {
				return imageDTO.Neutral(err, "registry.credential-unavailable", 503)
			}
			platform, e := identity.ParsePlatform(sel.Platform)
			if e != nil {
				return e
			}
			opts.Platforms = []ocispec.Platform{platform.OCI()}
			reader, err := c.ImagePull(ctx, immutable, opts)
			if err != nil {
				return imageService.PullError(err, opts.RegistryAuth != "")
			}
			progressErr := imageService.WriteSafePullProgress(reader, io.Discard, opts.RegistryAuth != "")
			closeErr := reader.Close()
			if progressErr != nil {
				return progressErr
			}
			if closeErr != nil {
				return imageService.PullError(closeErr, opts.RegistryAuth != "")
			}

			pulled, err := c.ImageInspect(ctx, immutable)
			if err != nil {
				return lifecycleFailure("update.target-changed")
			}
			native, e := identity.Resolve(ctx, c, pulled.InspectResponse, sel.Platform, nil)
			expected, f := identity.ParseExpected(sel.ImageID)
			if e != nil || f != nil || !identity.MatchesExpected(native.ID, expected) {
				return lifecycleFailure("update.target-changed")
			}
			if native.Digest() != "" && native.Digest() != sel.Digest {
				return lifecycleFailure("update.target-changed")
			}
			img := desired.Services[sel.Service]
			img.Digest = native.Digest()
			desired.Services[sel.Service] = img
		}
		img := desired.Services[sel.Service]
		img.ImageID = sel.ImageID
		img.Reference = sel.Reference
		if sel.Kind == "restore" {
			img.Digest = sel.Digest
		}
		img.Platform = sel.Platform
		desired.Services[sel.Service] = img
	}
	p, err := s.getMutableProject(ctx, desired.TargetID)
	if err != nil {
		return lifecycleFailure("update.target-changed")
	}
	current, err := s.lifecycleInputs(ctx, p)
	if err != nil || current.SavedRevision != expectedRevision {
		return lifecycleFailure("update.target-changed")
	}
	current.RegistrationID = desired.RegistrationID
	if err = s.lifecycleObserve(ctx, current); err != nil || lifecycleObserved(current) != expectedObserved {
		return lifecycleFailure("update.target-changed")
	}
	for key, img := range desired.Services {
		if !img.Present {
			continue
		}
		if err = s.imageService.ImageGuard().Protect(ctx, guard.Protection{ImageID: img.ImageID, Reason: "active-update", OwnerID: run}); err != nil {
			return err
		}
		if _, err = c.ImageTag(ctx, client.ImageTagOptions{Source: img.ImageID, Target: life.Pin("runs", run, key)}); err != nil {
			return lifecycleFailure("image.tag-failed")
		}
		current, e := c.ImageInspect(ctx, img.Reference)
		if e == nil && current.InspectResponse.ID == img.ImageID {
			continue
		}
		if err = s.lifecycleStore().Effect(ctx, run); err != nil {
			return err
		}
		if _, err = c.ImageTag(ctx, client.ImageTagOptions{Source: img.ImageID, Target: img.Reference}); err != nil {
			return lifecycleFailure("image.tag-failed")
		}
	}
	return nil
}

// DropPoint commits metadata deletion first. A cleanup failure retains the
// protection for retry; other point/run owners keep their separate protection.
func (s *ProjectService) deleteLifecyclePoint(ctx context.Context, id string) error {
	return s.lifecycleStore().Delete(ctx, id)
}
func (s *ProjectService) ReconcileImageLifecycle(ctx context.Context) error {
	release, err := s.imageService.ImageGuard().Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	var rows []life.Run
	if err = s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return err
	}
	for _, run := range rows {
		if !run.Settled {
			continue
		}
		if !run.HadEffect {
			held, e := s.lifecycleStore().Held(ctx, run.PointID)
			if e != nil {
				return e
			}
			if !held {
				if e = s.db.WithContext(ctx).Delete(&life.Point{}, "id = ?", run.PointID).Error; e != nil {
					return e
				}
			}
		}
		if err = s.db.WithContext(ctx).Delete(&life.Run{}, "id = ?", run.ID).Error; err != nil {
			return err
		}
	}
	var targets []string
	if err = s.db.WithContext(ctx).Model(&life.Point{}).Distinct("target_id").Pluck("target_id", &targets).Error; err != nil {
		return err
	}
	for _, target := range targets {
		if err = s.trimLifecyclePoints(ctx, target); err != nil {
			return err
		}
	}
	// Metadata-free pins are durable cleanup work, including interrupted preparing
	// points. Never infer run settlement from process or lease age.
	var pins []guard.Protection
	if err = s.db.WithContext(ctx).Where("reason IN ?", []string{"restore-point", "active-update"}).Find(&pins).Error; err != nil {
		return err
	}
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return err
	}
	for _, pin := range pins {
		var n int64
		table := "restore_points"
		if pin.Reason == "active-update" {
			table = "image_lifecycle_runs"
		}
		if err = s.db.WithContext(ctx).Table(table).Where("id = ?", pin.OwnerID).Count(&n).Error; err != nil {
			return err
		}
		if n != 0 {
			continue
		}
		// Pin tags use hashed owners; enumerate only this exact owner's namespace.
		prefix := guard.ReservedHost + "/points/" + life.Hash([]string{"points", pin.OwnerID}) + "/"
		if pin.Reason == "active-update" {
			prefix = guard.ReservedHost + "/runs/" + life.Hash([]string{"runs", pin.OwnerID}) + "/"
		}
		img, e := c.ImageInspect(ctx, pin.ImageID)
		if e != nil && !errdefs.IsNotFound(e) {
			continue
		}
		ok := true
		if e == nil {
			for _, tag := range img.InspectResponse.RepoTags {
				if strings.HasPrefix(tag, prefix) {
					if _, e = c.ImageRemove(ctx, tag, client.ImageRemoveOptions{Force: false, PruneChildren: false}); e != nil {
						ok = false
					}
				}
			}
		}
		if ok {
			if err = s.imageService.ImageGuard().Release(ctx, pin); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *ProjectService) trimLifecyclePoints(ctx context.Context, target string) error {
	var points []life.Point
	if err := s.db.WithContext(ctx).Where("target_id = ? AND available = ?", target, true).Order("created_at DESC, id DESC").Find(&points).Error; err != nil {
		return err
	}
	held := map[string]bool{}
	for _, point := range points {
		active, err := s.lifecycleStore().Held(ctx, point.ID)
		if err != nil {
			return err
		}
		held[point.ID] = active
	}
	for _, id := range life.RetentionDeletions(points, held) {
		if err := s.lifecycleStore().TrimPoint(ctx, target, id); err != nil {
			return err
		}
	}
	return nil
}

func lifecyclePlatform(os, arch, variant string) string {
	value := os + "/" + arch
	if variant != "" {
		value += "/" + variant
	}
	return value
}
