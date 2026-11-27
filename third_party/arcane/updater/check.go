// Source: getarcaneapp/kit/updater/check.go
// Revision: 119f68956cee6d5aba8a5463561aaf9be9943b65
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed Swarm; optional platform candidate filtering with ordered tag fallback.

package updater

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker/compat"

	"go.getarcane.app/updater/digest"
	"go.getarcane.app/updater/internal/digestcheck"
	"go.getarcane.app/updater/pkg/utils/tagpolicy"
	"go.getarcane.app/updater/refs"
	"go.getarcane.app/updater/types"
)

// CheckImageUpdate discovers an image update without pulling or changing state.
// CurrentDigest should describe the image in use, not a separately cached tag.
func (s *Service) CheckImageUpdate(ctx context.Context, request types.CheckRequest) (types.CheckResult, error) {
	result, err := s.selectImageTagInternal(ctx, request.ImageRef, request.Policy, request.ImageRef, request.Platform)
	if err != nil || result.Reason != "" || result.UpdateAvailable {
		return result, err
	}
	result.UpdateType = string(UpdateTypeDigest)
	if request.CurrentDigest != "" {
		if s.config.RegistryDigestResolver == nil {
			return result, errors.New("remote digest resolver unavailable")
		}
		result.CurrentDigest, err = digest.Normalize(request.CurrentDigest)
		if err != nil {
			return result, err
		}
		remote, resolveErr := s.config.RegistryDigestResolver.ImageDigest(ctx, result.CurrentRef)
		if resolveErr != nil {
			return result, resolveErr
		}
		result.TargetDigest, err = digest.Normalize(remote)
		if err != nil {
			return result, err
		}
		result.UpdateAvailable = result.CurrentDigest != result.TargetDigest
		return result, nil
	}
	dockerClient, err := s.dockerClient(ctx)
	if err != nil {
		return result, err
	}
	check := digestcheck.NewChecker(dockerClient, s.config.RegistryDigestResolver).CheckImageNeedsUpdate(ctx, result.CurrentRef)
	result.CurrentDigest, result.TargetDigest = check.LocalDigest, check.RemoteDigest
	if check.Error != nil {
		return result, check.Error
	}
	result.UpdateAvailable = check.NeedsUpdate
	return result, nil
}

func (s *Service) selectImageTagInternal(ctx context.Context, imageRef string, policy types.Policy, localReference ...string) (types.CheckResult, error) {
	result := types.CheckResult{CurrentRef: imageRef, TargetRef: imageRef}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if refs.IsDigestPinnedReference(imageRef) || refs.IsImageIDLikeReference(imageRef) {
		result.Reason = "immutable image reference"
		return result, nil
	}
	parsed, err := refs.NormalizeReference(imageRef)
	if err != nil {
		return result, err
	}
	result.CurrentRef, result.TargetRef = parsed.NormalizedRef, parsed.NormalizedRef
	policy, err = tagpolicy.Resolve(imageRef, policy)
	if err != nil {
		return result, err
	}
	if policy.Strategy == "digest" {
		return result, nil
	}
	if s.config.RegistryTagLister == nil {
		return result, errors.New("registry tag lister unavailable")
	}
	tags, err := s.config.RegistryTagLister.ListTags(ctx, parsed.NormalizedRef)
	if err != nil {
		return result, fmt.Errorf("list image tags: %w", err)
	}
	selected, err := SelectTagCandidate(ctx, parsed.Tag, tags, policy, func(ctx context.Context, tag string) (bool, error) {
		if s.config.TagCandidateFilter == nil {
			return true, nil
		}
		current := imageRef
		if len(localReference) > 0 && localReference[0] != "" {
			current = localReference[0]
		}
		platform := ""
		if len(localReference) > 1 {
			platform = localReference[1]
		}
		return s.config.TagCandidateFilter.AcceptTag(ctx, types.TagCandidate{CurrentRef: current, Platform: platform, TargetRef: parsed.RegistryHost + "/" + parsed.Repository + ":" + tag})
	})
	if err != nil {
		return result, err
	}
	result.CurrentVersion, err = tagpolicy.Version(parsed.Tag, policy)
	if err != nil {
		return result, err
	}
	result.TargetVersion, err = tagpolicy.Version(selected, policy)
	if err != nil {
		return result, err
	}
	result.TargetRef = parsed.RegistryHost + "/" + parsed.Repository + ":" + selected
	if selected != parsed.Tag {
		result.UpdateType = string(UpdateTypeTag)
		result.UpdateAvailable = true
	}
	return result, nil
}

// CheckContainerUpdate inspects one container and returns its current policy's
// candidate. It does not write pending records or change Docker resources.
func (s *Service) CheckContainerUpdate(ctx context.Context, containerID string) (types.CheckResult, error) {
	dockerClient, err := s.dockerClient(ctx)
	if err != nil {
		return types.CheckResult{}, err
	}
	inspected, err := compat.ContainerInspectWithCompatibility(ctx, dockerClient, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return types.CheckResult{}, err
	}
	inspect := inspected.Container
	result := types.CheckResult{ContainerID: inspect.ID}
	if result.ContainerID == "" {
		result.ContainerID = containerID
	}
	reason, err := s.containerEligibilityInternal(ctx, inspect, enforceSettingsExclusions)
	if err != nil {
		return result, err
	}
	if reason != "" {
		result.Reason = reason
		return result, nil
	}
	imageRef := inspect.Config.Image
	result, err = s.selectImageTagInternal(ctx, imageRef, s.config.LabelPolicy.TagPolicy(inspect.Config.Labels), inspect.Image)
	result.ContainerID = inspect.ID
	if err != nil || result.Reason != "" || result.UpdateAvailable {
		return result, err
	}
	// Compare the running image, even if the configured tag is already cached at
	// a newer digest in the daemon.
	image, err := dockerClient.ImageInspect(ctx, inspect.Image)
	if err != nil {
		return result, err
	}
	if s.config.RegistryDigestResolver == nil {
		return result, errors.New("remote digest resolver unavailable")
	}
	remote, err := s.config.RegistryDigestResolver.ImageDigest(ctx, result.CurrentRef)
	if err != nil {
		return result, err
	}
	result.TargetDigest, err = digest.Normalize(remote)
	if err != nil {
		return result, err
	}
	result.CurrentDigest = image.ID
	result.UpdateType = string(UpdateTypeDigest)
	result.UpdateAvailable = true
	for _, repoDigest := range image.RepoDigests {
		if local, ok := digest.FromReferenceSuffix(repoDigest); ok {
			result.CurrentDigest = local
			if local == result.TargetDigest {
				result.UpdateAvailable = false
				break
			}
		}
	}
	return result, nil
}

// containerEligibilityInternal reports why a container cannot be updated, or
// "" when it can. The label and configuration checks always apply;
// policy decides whether the settings exclusion list is consulted too.
func (s *Service) containerEligibilityInternal(ctx context.Context, inspect container.InspectResponse, policy exclusionPolicy) (string, error) {
	if inspect.Config == nil {
		return "container config unavailable", nil
	}
	labels := inspect.Config.Labels
	if s.config.LabelPolicy.IsUpdateDisabled(labels) {
		return "updates disabled by label", nil
	}
	if policy == ignoreSettingsExclusions {
		return "", nil
	}
	excluded, err := s.excludedContainerSet(ctx)
	if err != nil {
		return "", err
	}
	if excluded[inspect.ID] || excluded[strings.TrimPrefix(inspect.Name, "/")] {
		return "container excluded by settings", nil
	}
	return "", nil
}

// SelectTagCandidate tries acceptable tags newest first. Only an explicit
// platform refusal advances to an older tag; lookup errors remain failures.
func SelectTagCandidate(ctx context.Context, current string, tags []string, policy types.Policy, accept func(context.Context, string) (bool, error)) (string, error) {
	remaining := slices.Clone(tags)
	for {
		selected, err := tagpolicy.Select(current, remaining, policy)
		if err != nil || selected == current {
			return selected, err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		ok, err := accept(ctx, selected)
		if err != nil {
			return "", err
		}
		if ok {
			return selected, nil
		}
		remaining = slices.DeleteFunc(remaining, func(tag string) bool { return tag == selected })
	}
}
