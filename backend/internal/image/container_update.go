// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original narrow bridge to existing fresh checks and native image identity.

package image

import (
	"context"
	"github.com/distribution/reference"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/pkg/utils/tagpolicy"

	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	dto "github.com/florianfinn/docklet-next/types/image"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	"github.com/moby/moby/api/types/container"
)

// CheckContainerImage refreshes the configured reference, preserving this
// container's tag policy instead of the aggregate result of other consumers.
func (s *ImageService) CheckContainerImage(ctx context.Context, current container.InspectResponse) (*checkDTO.Response, error) {
	if s == nil || s.imageUpdateService == nil || current.Config == nil {
		return nil, dto.Failure("update.check-unavailable", 503)
	}
	result, err := s.imageUpdateService.ReadUpdateInfo(ctx, current.Config.Image)
	if err != nil || result == nil {
		return nil, dto.Neutral(err, "update.check-unavailable", 503)
	}
	if result.ContainerUpdates != nil {
		result = result.ContainerUpdates[current.ID]
		if result == nil {
			return nil, dto.Failure("update.check-unavailable", 503)
		}
	} else if result.ImageUpdate != nil {
		result = result.ImageUpdate
	}
	owned := *result
	if owned.State == "version" && updater.DefaultLabelPolicy().TagPolicy(current.Config.Labels).TagPattern != "" {
		// Public observations normalize versions; execution needs the actual tag,
		// including a configured tag-pattern prefix. Reuse the selection engine.
		if s.registryService == nil {
			return nil, dto.Failure("update.check-unavailable", 503)
		}
		policy, err := tagpolicy.Resolve(current.Config.Image, updater.DefaultLabelPolicy().TagPolicy(current.Config.Labels))
		if err != nil {
			return nil, err
		}
		named, err := reference.ParseNormalizedNamed(current.Config.Image)
		if err != nil {
			return nil, err
		}
		tagged, ok := named.(reference.Tagged)
		if !ok {
			return nil, dto.Failure("update.check-unavailable", 503)
		}
		tags, err := s.registryService.ListImageTags(ctx, current.Config.Image, nil)
		if err != nil {
			return nil, err
		}
		selected, err := tagpolicy.Select(tagged.Tag(), tags, policy)
		if err != nil {
			return nil, err
		}
		version, err := tagpolicy.Version(selected, policy)
		if err != nil || version != owned.LatestVersion {
			return nil, dto.Failure("update.target-changed", 409)
		}
		owned.LatestVersion = selected
	}
	return &owned, nil
}

// ExpectedContainerImage resolves the registry candidate in the running
// container's native store/platform. It never exposes registry credentials.
func (s *ImageService) ExpectedContainerImage(ctx context.Context, current container.InspectResponse, ref string) (string, string, error) {
	if s == nil || s.registryService == nil || s.dockerService == nil {
		return "", "", dto.Failure("update.check-unavailable", 503)
	}
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return "", "", err
	}
	img, err := c.ImageInspect(ctx, current.Image)
	if err != nil {
		return "", "", err
	}
	local, err := identity.ResolveContainer(ctx, c, img.InspectResponse, "", current.ImageManifestDescriptor)
	if err != nil {
		return "", "", err
	}
	observed, err := s.registryService.PlatformImage(ctx, ref, local.Platform.Registry())
	if err != nil {
		return "", "", err
	}
	candidate, err := identity.NewCandidate(observed.Digest, observed.ConfigDigest, local.Platform)
	if err != nil {
		return "", "", err
	}
	expected, err := local.Expected(candidate)
	return expected.String(), local.Platform.String(), err
}
