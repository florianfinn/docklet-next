// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original policy projection for issue #229; persist final consumer-bound native observations, including per-reference digest checks.

package imageupdate

import (
	"context"
	"sort"
	"strings"

	"errors"
	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/pkg/utils/tagpolicy"
	"go.getarcane.app/updater/refs"
)

// ReadUpdateInfo keeps image observations separate from each consumer's selected
// tag policy. Only the final native observation is saved; discovery never applies updates.
func (s *ImageUpdateService) ReadUpdateInfo(ctx context.Context, ref string) (*checkDTO.Response, error) {
	result, err := s.readImageObservation(ctx, ref)
	if err != nil {
		return result, err
	}
	if result.State == "error" || result.State == "unknown" {
		return result, s.saveFailedDigestObservations(ctx, ref, result)
	}
	if result.State == "local" {
		return result, s.invalidateConsumerObservations(ctx, ref, result)
	}
	if strings.Contains(ref, "@") || s.tags == nil {
		return result, nil
	}
	c, err := s.dockerClientInternal(ctx)
	if err != nil {
		return result, s.invalidateConsumerObservations(ctx, ref, result)
	}
	listed, err := c.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		result.State = "unknown"
		result.HasUpdate = false
		result.Code = "update.check-unavailable"
		return result, s.invalidateConsumerObservations(ctx, ref, result)
	}
	baseline, inspectErr := c.ImageInspect(ctx, ref)
	consumers := map[string]string{}
	consumerLabels := map[string]map[string]string{}
	consumerRows := map[string]container.Summary{}
	for _, item := range listed.Items {
		consumers[item.ID] = item.ImageID
		consumerLabels[item.ID] = item.Labels
		consumerRows[item.ID] = item
	}
	policies, err := s.tags.Check(ctx, listed.Items, map[string]bool{refs.NormalizeImageUpdateRef(ref): true}, nil, s.deleteContainerUpdateRecordsInternal, func(context.Context, container.Summary, *checkDTO.Response) error { return nil })
	if err != nil {
		result.State = "error"
		result.HasUpdate = false
		result.Code = "update.check-unavailable"
		result.Error = imageDTO.Failure(result.Code, 503).Error()
		return result, s.invalidateConsumerObservations(ctx, ref, result)
	}
	// The image observation was checked for this reference, independently of
	// other tags sharing its image ID. Replicas reuse that registry result.
	// Every digest consumer is covered whatever its state, like tag consumers:
	// readers and the update action see stopped containers too.
	for _, item := range listed.Items {
		policy, policyErr := tagpolicy.Resolve(item.Image, updater.DefaultLabelPolicy().TagPolicy(item.Labels))
		if !imageref.IsUpdateCheckDisabled(item.Labels) && SameUpdateReference(item.Image, ref) && policyErr == nil && policy.Strategy == "digest" {
			policies[item.ID] = &checkDTO.Response{ImageRef: item.Image}
		}
	}
	if len(policies) == 0 {
		return result, nil
	}
	snapshot := *result
	result.ImageUpdate = &snapshot
	result.ContainerUpdates = map[string]*checkDTO.Response{}
	ids := make([]string, 0, len(policies))
	for id := range policies {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		discovered := policies[id]
		candidateVersion := discovered.LatestVersion
		if parsed, err := tagpolicy.Version(discovered.LatestVersion, updater.DefaultLabelPolicy().TagPolicy(consumerLabels[id])); err == nil {
			candidateVersion = parsed
		}
		projected := snapshot
		projected.ImageRef = discovered.ImageRef
		inspected, containerErr := c.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
		var local identity.LocalImage
		var native *identity.NativeObservation
		if containerErr == nil {
			local, containerErr = identity.ResolveContainer(ctx, c, baseline.InspectResponse, "", inspected.Container.ImageManifestDescriptor)
		}
		policy, policyErr := tagpolicy.Resolve(consumerRows[id].Image, updater.DefaultLabelPolicy().TagPolicy(consumerLabels[id]))
		if containerErr == nil && (policyErr != nil || policy.Strategy != "digest") {
			base, e := s.readImageObservationAt(ctx, ref, local.Platform.String(), inspected.Container.ImageManifestDescriptor)
			if e == nil && base != nil {
				projected = *base
			} else {
				containerErr = imageDTO.Failure("update.check-unavailable", 503)
			}
		}
		if inspectErr != nil || containerErr != nil || inspected.Container.Image != consumers[id] || consumers[id] == "" || consumers[id] != baseline.ID {
			projected.State = "unknown"
			projected.HasUpdate = false
			projected.Code = "update.check-unavailable"
		} else if discovered.Error != "" {
			projected.State = "error"
			projected.HasUpdate = false
			projected.Code = checkCodeString(discovered.Error)
			projected.Error = imageDTO.Failure(projected.Code, 503).Error()
		} else if cmp, valid := imageDTO.CompareRelease(candidateVersion, discovered.CurrentVersion); valid && cmp > 0 {
			named, e := reference.ParseNormalizedNamed(discovered.ImageRef)
			if e == nil {
				var target reference.NamedTagged
				target, e = reference.WithTag(reference.TrimNamed(named), discovered.LatestVersion)
				if e == nil {
					candidate, lookupErr := s.registryService.PlatformImage(ctx, target.String(), local.Platform.Registry())
					latest := ""
					if lookupErr == nil {
						r, e := identity.NewCandidate(candidate.Digest, candidate.ConfigDigest, local.Platform)
						lookupErr = e
						if e == nil {
							k, e := local.CandidateKey(r)
							lookupErr = e
							latest = k.String()
							if observed, err := local.Observe(r); err == nil {
								native = &observed
							}
						}
					}
					info := &imageDTO.UpdateInfo{ImageRef: discovered.ImageRef, CheckTime: snapshot.CheckTime, Platform: local.Platform.String(), CurrentDigest: local.Key.String(), LatestDigest: latest, CurrentVersion: discovered.CurrentVersion, LatestVersion: candidateVersion, AuthState: candidate.AuthState}
					if lookupErr != nil {
						problem := imageDTO.Neutral(lookupErr, "update.check-unavailable", 503)
						info.Error = problem.Error()
						info.Code = problem.Problem.Code
					}
					imageDTO.ProjectUpdate(info)
					projected = *checkDTO.FromInfo(info)
				}
			}
			if e != nil {
				projected.State = "unknown"
				projected.HasUpdate = false
				projected.Code = "update.check-unavailable"
			}
		}
		if err := s.saveConsumerObservation(ctx, consumerRows[id], &projected, local, native); err != nil {
			return nil, err
		}
		result.ContainerUpdates[id] = &projected
		if projected.State == "version" && result.State != "version" {
			imageSnapshot, children := result.ImageUpdate, result.ContainerUpdates
			*result = projected
			result.ImageUpdate = imageSnapshot
			result.ContainerUpdates = children
		}
	}
	return result, nil
}

func checkCodeString(message string) string { return checkCode(errors.New(message)) }
