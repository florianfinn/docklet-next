// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original standalone image update and manual previous-image restore. M21 #438: candidate scans and verified manifest binding.

package container

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"go.getarcane.app/docker/compat"
	dockertypes "go.getarcane.app/docker/types"
)

type containerImagePort interface {
	CheckContainerImage(context.Context, dockercontainer.InspectResponse) (*checkDTO.Response, error)
	ExpectedContainerImage(context.Context, dockercontainer.InspectResponse, string) (string, string, error)
	ImageGuard() *guard.Service
	PullOptionsWithAuth(context.Context, string, []containerregistry.Credential) (client.ImagePullOptions, error)
}

type expectedContainerImageKey struct{}

// ExecuteContainerImage receives the full name and ID frozen by admission.
// Name lookup must still identify that exact instance before any image effect.
func (s *ContainerService) ExecuteContainerImage(ctx context.Context, id, name string, restore bool, actor user.Actor) (resultErr error) {
	run := updaterDTO.ExecutionIdentityFromContext(ctx).RunID
	result := updaterDTO.LifecycleResult{RunID: run}
	defer func() {
		if resultErr != nil {
			p := imageDTO.Neutral(resultErr, "update.deploy-failed", 500)
			result.Code, result.HTTPStatus = p.Problem.Code, p.Status
			var originProblem *dto.Problem
			if errors.As(resultErr, &originProblem) {
				result.Code, result.HTTPStatus = originProblem.Code, originProblem.Status
			}
		} else {
			result.Verified = true
		}
		updaterDTO.RecordLifecycleResult(ctx, result)
	}()
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return err
	}
	old, err := s.imageUpdateInstance(ctx, c, id, name)
	if err != nil {
		return err
	}
	if s.containerImages == nil || run == "" {
		return imageDTO.Failure("update.check-unavailable", 503)
	}
	g := s.containerImages.ImageGuard()
	release, err := g.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	ref, expected, platform := "", "", ""
	if restore {
		slot, e := s.previousImage(ctx, name)
		if e != nil {
			return e
		}
		if slot == nil || slot.OwnerContainerID != old.ID || slot.ImageID == old.Image {
			return imageDTO.Failure("update.target-changed", 409)
		}
		ref, expected = slot.ImageRef, slot.ImageID
		if err := guard.ValidateReference(ref); err != nil {
			return err
		}
		inspected, e := c.ImageInspect(ctx, ref)
		if e != nil && !errdefs.IsNotFound(e) {
			return imageDTO.Failure("image.usage-unavailable", 503)
		}
		if e != nil || inspected.ID != expected {
			if _, err := s.imageUpdateInstance(ctx, c, id, name); err != nil {
				return err
			}
			if _, err := c.ImageTag(ctx, client.ImageTagOptions{Source: expected, Target: ref}); err != nil {
				return imageDTO.Neutral(err, "image.tag-failed", 502)
			}
		}
	} else {
		checked, err := s.containerImages.CheckContainerImage(ctx, *old)
		if err != nil {
			return err
		}
		if checked == nil || checked.State == "unknown" || checked.State == "error" || checked.Error != "" {
			return imageDTO.Failure("update.check-unavailable", 503)
		}
		if !checked.HasUpdate && updaterDTO.ExpectedManifestFromContext(ctx) == "" {
			return imageDTO.Failure("job.skipped", 409)
		}
		ref = old.Config.Image
		// Only a tag-policy version moves the reference; a rebuild keeps it.
		if checked.State == "version" && checked.LatestVersion != "" {
			named, err := reference.ParseNormalizedNamed(ref)
			if err != nil {
				return imageDTO.Failure("update.invalid-target", 400)
			}
			tagged, err := reference.WithTag(reference.TrimNamed(named), checked.LatestVersion)
			if err != nil {
				return imageDTO.Failure("update.invalid-target", 400)
			}
			ref = reference.FamiliarString(tagged)
		}
		if err := guard.ValidateReference(ref); err != nil {
			return err
		}
		verified := updaterDTO.ExpectedManifestFromContext(ctx)
		if verified != "" {
			resolver, ok := s.containerImages.(interface {
				CandidateContainerImage(context.Context, dockercontainer.InspectResponse, string) (string, string, string, error)
			})
			if !ok || !updaterDTO.ValidManifest(verified) {
				return imageDTO.Failure("update.check-unavailable", 503)
			}
			var manifest string
			expected, platform, manifest, err = resolver.CandidateContainerImage(ctx, *old, ref)
			if err == nil && manifest != verified {
				return imageDTO.Failure("update.candidate-changed", 409)
			}
		} else {
			expected, platform, err = s.containerImages.ExpectedContainerImage(ctx, *old, ref)
		}
		if err != nil {
			return err
		}
		if !checked.HasUpdate {
			return imageDTO.Failure("job.skipped", 409)
		}
		if expected == "" || expected != checked.LatestDigest {
			return imageDTO.Failure("update.target-changed", 409)
		}
		if verified != "" {
			named, err := reference.ParseNormalizedNamed(ref)
			if err != nil {
				return imageDTO.Failure("update.invalid-target", 400)
			}
			ref = reference.TrimNamed(named).String() + "@" + verified
		}
		options, err := s.containerImages.PullOptionsWithAuth(ctx, ref, nil)
		if err != nil {
			return err
		}
		parsed, err := identity.ParsePlatform(platform)
		if err != nil {
			return err
		}
		options.Platforms = []ocispec.Platform{parsed.OCI()}
		timeout := timeouts.DefaultDockerImagePull
		if s.settingsService != nil {
			timeout = timeouts.GetDuration(s.settingsService.GetSettingsConfig().DockerImagePullTimeout.AsInt(), timeouts.DefaultDockerImagePull)
		}
		pullCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		// Registry/tag reads may outlive the admitted instance. Prove it again
		// immediately before the first image mutation, then before recreate.
		if _, err := s.imageUpdateInstance(pullCtx, c, id, name); err != nil {
			return err
		}
		reader, err := c.ImagePull(pullCtx, ref, options)
		if err != nil {
			return imageDTO.Neutral(err, "image.pull-failed", 502)
		}
		writer, _ := ctx.Value(dockertypes.ProgressWriterKey{}).(io.Writer)
		if writer == nil {
			writer = io.Discard
		}
		err = image.WriteSafePullProgress(reader, writer, options.RegistryAuth != "")
		_ = reader.Close()
		if err != nil {
			return err
		}
		pulled, err := c.ImageInspect(ctx, ref)
		if err != nil {
			return imageDTO.Failure("update.target-changed", 409)
		}
		// The candidate names a native key; the recreated container runs the local ID.
		if key, e := identity.PulledNativeKey(ctx, c, pulled.InspectResponse, platform); e != nil || key != expected {
			return imageDTO.Failure("update.target-changed", 409)
		}
		expected = pulled.ID
		if _, err := s.imageUpdateInstance(ctx, c, id, name); err != nil {
			return err
		}
		if err := s.replacePreviousImage(ctx, name, old.Image, old.Config.Image, old.ID, run); err != nil {
			return err
		}
	}
	// Recheck before recreate, including after a slow pull or retag.
	if _, err := s.imageUpdateInstance(ctx, c, id, name); err != nil {
		return err
	}
	ctx = context.WithValue(ctx, expectedContainerImageKey{}, expected)
	config := *old.Config
	config.Image = ref
	host := dockercontainer.HostConfig{}
	if old.HostConfig != nil {
		host = *old.HostConfig
	}
	api := compat.DetectDockerAPIVersion(ctx, c)
	_, err = s.recreateDurable(ctx, c, *old, name, &config, &host, buildCleanNetworkingConfigInternal(*old, api), api)
	var problem *dto.Problem
	if errors.As(err, &problem) {
		if problem.Code == "container.recreate-recovered" {
			return imageDTO.Failure("update.deploy-failed", 500)
		}
		if problem.Code == "container.recreate-ambiguous" {
			return &recreateUncertain{cause: err}
		}
	}
	return err
}
func (s *ContainerService) imageUpdateInstance(ctx context.Context, c *client.Client, id, name string) (*dockercontainer.InspectResponse, error) {
	current, err := recreateInspect(ctx, c, name)
	if err != nil {
		return nil, err
	}
	if current == nil || current.ID != id || strings.TrimPrefix(current.Name, "/") != name {
		return nil, imageDTO.Failure("update.target-changed", 409)
	}
	if err := effectOriginGuard(*current); err != nil {
		return nil, err
	}
	return current, nil
}
