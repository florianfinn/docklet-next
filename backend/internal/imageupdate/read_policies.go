// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original read-only policy projection for issue #229.

package imageupdate

import (
	"context"
	"sort"
	"strings"

	"errors"
	"github.com/distribution/reference"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/pkg/utils/tagpolicy"
	"go.getarcane.app/updater/refs"
)

// ReadUpdateInfo keeps image observations separate from each consumer's selected
// tag policy. The adopted discovery engine receives no persistence/apply callback.
func (s *ImageUpdateService) ReadUpdateInfo(ctx context.Context, ref string) (*checkDTO.Response, error) {
	result, err := s.readImageObservation(ctx, ref)
	if err != nil || result.State == "error" || result.State == "unknown" || result.State == "local" || strings.Contains(ref, "@") || s.tags == nil {
		return result, err
	}
	c, err := s.dockerClientInternal(ctx)
	if err != nil {
		return result, nil
	}
	listed, err := c.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		result.State = "unknown"
		result.HasUpdate = false
		result.Code = "update.check-unavailable"
		return result, nil
	}
	baseline, inspectErr := c.ImageInspect(ctx, ref)
	consumers := map[string]string{}
	consumerLabels := map[string]map[string]string{}
	for _, item := range listed.Items {
		consumers[item.ID] = item.ImageID
		consumerLabels[item.ID] = item.Labels
	}
	policies, err := s.tags.Check(ctx, listed.Items, map[string]bool{refs.NormalizeImageUpdateRef(ref): true}, nil, func(context.Context, []string) error { return nil }, func(context.Context, container.Summary, *checkDTO.Response) error { return nil })
	if err != nil {
		result.State = "error"
		result.HasUpdate = false
		result.Code = "update.check-unavailable"
		result.Error = imageDTO.Failure(result.Code, 503).Error()
		return result, nil
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
		if inspectErr != nil || consumers[id] == "" || consumers[id] != baseline.ID {
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
					parts := strings.Split(snapshot.Platform, "/")
					platform := v1.Platform{OS: parts[0], Architecture: parts[1]}
					if len(parts) > 2 {
						platform.Variant = parts[2]
					}
					candidate, lookupErr := s.registryService.PlatformImage(ctx, target.String(), platform)
					info := &imageDTO.UpdateInfo{ImageRef: discovered.ImageRef, CheckTime: snapshot.CheckTime, Platform: snapshot.Platform, CurrentDigest: snapshot.CurrentDigest, LatestDigest: candidate.Digest, CurrentVersion: discovered.CurrentVersion, LatestVersion: candidateVersion, AuthState: candidate.AuthState}
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
