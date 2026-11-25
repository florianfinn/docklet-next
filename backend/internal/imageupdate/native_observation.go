// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Checked native provenance at record and resolver boundaries.

package imageupdate

import (
	"context"
	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/moby/moby/client"
	"strings"
	"time"
)

func (s *ImageUpdateService) ResolveLocal(ctx context.Context, ref, platform string) (identity.LocalImage, error) {
	c, err := s.dockerClientInternal(ctx)
	if err != nil {
		return identity.LocalImage{}, err
	}
	img, err := c.ImageInspect(ctx, ref)
	if err != nil {
		return identity.LocalImage{}, err
	}
	return identity.Resolve(ctx, c, img.InspectResponse, platform, nil)
}

// NativeResult carries a native observation with the image config build times
// of the local image and the candidate.
type NativeResult struct {
	Observation                   identity.NativeObservation
	CurrentCreated, LatestCreated *time.Time
}

func (s *ImageUpdateService) NativeObservation(ctx context.Context, ref, target, platform string) (NativeResult, error) {
	local, err := s.nativeReference(ctx, ref, platform)
	if err != nil {
		return NativeResult{}, err
	}
	if target == "" {
		target = ref
	}
	if s.registryService == nil {
		return NativeResult{}, imageDTO.Failure("update.check-unavailable", 503)
	}
	candidate, err := s.registryService.PlatformImage(ctx, target, local.Platform.Registry())
	if err != nil {
		return NativeResult{}, err
	}
	r, err := identity.NewCandidate(candidate.Digest, candidate.ConfigDigest, local.Platform)
	if err != nil {
		return NativeResult{}, err
	}
	observed, err := local.Observe(r)
	if err != nil {
		return NativeResult{}, err
	}
	return NativeResult{Observation: observed, CurrentCreated: local.Created, LatestCreated: imageDTO.BuildTime(candidate.Created)}, nil
}
func (i *ImageUpdateRecord) SetNativeObservation(n identity.NativeObservation) {
	p, current, latest := n.Storage()
	i.Platform = &p
	i.CurrentDigest = &current
	i.LatestDigest = &latest
}

// NativeServiceObservation keeps a configured check at the actual containers'
// platform. A moved tag or mixed replica identity cannot borrow a newer image's
// observation. With no containers, the effective Compose platform is used.
func (s *ImageUpdateService) NativeServiceObservation(ctx context.Context, ref, target, platform, project, service string) (NativeResult, error) {
	if s.registryService == nil {
		return NativeResult{}, imageDTO.Failure("update.check-unavailable", 503)
	}
	if project == "" {
		return s.NativeObservation(ctx, ref, target, platform)
	}
	c, err := s.dockerClientInternal(ctx)
	if err != nil {
		return NativeResult{}, err
	}
	img, err := c.ImageInspect(ctx, ref)
	if err != nil {
		return NativeResult{}, err
	}
	if !repositoryEvidence(ref, img.RepoDigests) {
		return NativeResult{}, imageDTO.Failure("update.check-unavailable", 503)
	}
	listed, err := c.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return NativeResult{}, err
	}
	var current *identity.LocalImage
	for _, item := range listed.Items {
		if item.Labels["com.docker.compose.project"] != project || item.Labels["com.docker.compose.service"] != service || strings.EqualFold(item.Labels["com.docker.compose.oneoff"], "true") {
			continue
		}
		inspected, e := c.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
		if e != nil || inspected.Container.Image != img.ID || item.ImageID != img.ID {
			return NativeResult{}, imageDTO.Failure("update.check-unavailable", 503)
		}
		local, e := identity.ResolveContainer(ctx, c, img.InspectResponse, platform, inspected.Container.ImageManifestDescriptor)
		if e != nil {
			return NativeResult{}, e
		}
		if current != nil && (current.ID != local.ID || current.Key != local.Key || !current.Platform.Matches(local.Platform)) {
			return NativeResult{}, imageDTO.Failure("update.check-unavailable", 503)
		}
		current = &local
	}
	if current == nil {
		return s.NativeObservation(ctx, ref, target, platform)
	}
	if target == "" {
		target = ref
	}
	next, err := s.registryService.PlatformImage(ctx, target, current.Platform.Registry())
	if err != nil {
		return NativeResult{}, err
	}
	r, err := identity.NewCandidate(next.Digest, next.ConfigDigest, current.Platform)
	if err != nil {
		return NativeResult{}, err
	}
	observed, err := current.Observe(r)
	if err != nil {
		return NativeResult{}, err
	}
	return NativeResult{Observation: observed, CurrentCreated: current.Created, LatestCreated: imageDTO.BuildTime(next.Created)}, nil
}

func repositoryEvidence(ref string, digests []string) bool {
	named, e := reference.ParseNormalizedNamed(ref)
	if e != nil {
		return false
	}
	for _, raw := range digests {
		saved, e := reference.ParseNormalizedNamed(raw)
		if e == nil && reference.TrimNamed(named).String() == reference.TrimNamed(saved).String() {
			return true
		}
	}
	return false
}
func (s *ImageUpdateService) nativeReference(ctx context.Context, ref, platform string) (identity.LocalImage, error) {
	c, e := s.dockerClientInternal(ctx)
	if e != nil {
		return identity.LocalImage{}, e
	}
	img, e := c.ImageInspect(ctx, ref)
	if e != nil {
		return identity.LocalImage{}, e
	}
	if !repositoryEvidence(ref, img.RepoDigests) {
		return identity.LocalImage{}, imageDTO.Failure("update.check-unavailable", 503)
	}
	return identity.Resolve(ctx, c, img.InspectResponse, platform, nil)
}
