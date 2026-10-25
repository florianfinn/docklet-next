// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package imageupdate

import (
	"context"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/moby/client"
)

// ReadUpdateInfo is the PR1 producer: registry/Docker reads and an observation
// write only. It never invokes the updater, policy apply or notification engine.
func (s *ImageUpdateService) readImageObservation(ctx context.Context, ref string) (*checkDTO.Response, error) {
	if err := guard.ValidateReference(ref); err != nil {
		return nil, err
	}
	observedID := ""
	info := &imageDTO.UpdateInfo{ImageRef: ref, CheckTime: time.Now().UTC(), AuthState: "unknown"}
	finish := func() (*checkDTO.Response, error) {
		imageDTO.ProjectUpdate(info)
		if s.db != nil && !strings.Contains(ref, "@") {
			named, e := reference.ParseNormalizedNamed(ref)
			if e != nil {
				return nil, imageDTO.Failure("image.invalid-reference", 400)
			}
			record := ImageUpdateRecord{ID: observedID, ImageID: observedID, Repository: reference.TrimNamed(named).String(), Tag: "latest", CheckTime: info.CheckTime, CurrentVersion: info.CurrentVersion, LatestVersion: &info.LatestVersion, CurrentDigest: &info.CurrentDigest, LatestDigest: &info.LatestDigest, UpdateType: info.UpdateType, HasUpdate: info.HasUpdate, Platform: &info.Platform, AuthMethod: &info.AuthState, UsedCredential: info.UsedCredential}
			if info.Error != "" {
				record.LastError = &info.Error
			}
			if tag, ok := named.(reference.Tagged); ok {
				record.Tag = tag.Tag()
			}
			if observedID == "" {
				// Without a usable local identity, invalidate matching cached eligibility
				// instead of retaining a previous current result under this reference.
				var lastError any
				if info.Error != "" {
					lastError = info.Error
				}
				e = s.db.WithContext(ctx).Model(&ImageUpdateRecord{}).Where("repository = ? AND tag = ?", record.Repository, record.Tag).Updates(map[string]any{"platform": nil, "current_digest": nil, "latest_digest": nil, "last_error": lastError, "check_time": info.CheckTime, "has_update": false, "update_type": info.UpdateType, "auth_method": info.AuthState}).Error
			} else {
				e = s.db.WithContext(ctx).Save(&record).Error
			}
			if e != nil {
				return nil, imageDTO.Failure("update.persistence-failed", 500)
			}
		}
		result := checkDTO.FromInfo(info)
		return result, nil
	}
	c, err := s.dockerClientInternal(ctx)
	if err != nil {
		info.Error = "unavailable"
		info.Code = "update.check-unavailable"
		return finish()
	}
	local, err := c.ImageInspect(ctx, ref)
	if err != nil {
		info.State = "unknown"
		return finish()
	}
	observedID = local.ID
	if strings.Contains(ref, "@sha256:") {
		return finish()
	}
	if len(local.RepoDigests) == 0 {
		info.UpdateType = UpdateTypeLocal
		return finish()
	}
	platform := v1.Platform{OS: local.Os, Architecture: local.Architecture, Variant: local.Variant}
	info.Platform = platform.OS + "/" + platform.Architecture
	if platform.OS == "" || platform.Architecture == "" {
		info.Platform = ""
		return finish()
	}
	if platform.Variant != "" {
		info.Platform += "/" + platform.Variant
	}
	if local.Config != nil {
		info.CurrentVersion = local.Config.Labels["org.opencontainers.image.version"]
	}
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return nil, imageDTO.Failure("image.invalid-reference", 400)
	}
	if info.CurrentVersion == "" {
		if tag, ok := named.(reference.Tagged); ok {
			info.CurrentVersion = tag.Tag()
		}
	}
	if s.registryService == nil {
		info.Error = "unavailable"
		return finish()
	}
	currentRef := ""
	for _, saved := range local.RepoDigests {
		candidate, e := reference.ParseNormalizedNamed(saved)
		if e == nil && reference.TrimNamed(candidate).String() == reference.TrimNamed(named).String() {
			currentRef = saved
			break
		}
	}
	if currentRef == "" {
		return finish()
	}
	current, err := s.registryService.PlatformImage(ctx, currentRef, platform)
	if err != nil {
		problem := imageDTO.Neutral(err, "update.check-unavailable", 503)
		info.AuthState = current.AuthState
		info.Error = problem.Error()
		info.Code = problem.Problem.Code
		return finish()
	}
	if current.ConfigDigest != local.ID {
		info.Code = "update.check-unavailable"
		return finish()
	}
	next, err := s.registryService.PlatformImage(ctx, ref, platform)
	info.AuthState = next.AuthState
	if err != nil {
		problem := imageDTO.Neutral(err, "update.check-unavailable", 503)
		info.Error = problem.Error()
		info.Code = problem.Problem.Code
		return finish()
	}
	info.CurrentDigest = current.Digest
	info.LatestDigest = next.Digest
	info.LatestVersion = next.Version
	if info.LatestVersion == "" {
		info.LatestVersion = info.CurrentVersion
	}
	imageDTO.ProjectUpdate(info)
	return finish()
}
func (s *ImageUpdateService) ReadUpdateByID(ctx context.Context, id string) (*checkDTO.Response, error) {
	c, err := s.dockerClientInternal(ctx)
	if err != nil {
		return nil, imageDTO.Failure("update.check-unavailable", 503)
	}
	local, err := c.ImageInspect(ctx, id)
	if err != nil {
		return nil, imageDTO.Failure("image.not-found", 404)
	}
	public := guard.PublicTags(local.RepoTags)
	if len(public) == 0 {
		return checkDTO.FromInfo(&imageDTO.UpdateInfo{State: "local", CheckTime: time.Now().UTC(), CheckedAt: time.Now().UTC(), AuthState: "unknown", ImageRef: id}), nil
	}
	return s.ReadUpdateInfo(ctx, public[0])
}
func (s *ImageUpdateService) ReadMultiple(ctx context.Context, refs []string) (checkDTO.BatchResponse, error) {
	if len(refs) > 256 {
		return nil, imageDTO.Failure("update.invalid-target", 400)
	}
	out := checkDTO.BatchResponse{}
	for _, ref := range refs {
		r, err := s.ReadUpdateInfo(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[ref] = r
	}
	return out, nil
}
func (s *ImageUpdateService) ReadAll(ctx context.Context) (checkDTO.BatchResponse, error) {
	c, err := s.dockerClientInternal(ctx)
	if err != nil {
		return nil, imageDTO.Failure("update.check-unavailable", 503)
	}
	items, err := c.ImageList(ctx, client.ImageListOptions{})
	if err != nil {
		return nil, imageDTO.Failure("update.check-unavailable", 503)
	}
	refs := []string{}
	for _, item := range items.Items {
		refs = append(refs, guard.PublicTags(item.RepoTags)...)
	}
	return s.ReadMultiple(ctx, refs)
}
