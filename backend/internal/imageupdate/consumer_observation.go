// Source: docklet-next/issues/409
// Revision: e683d402f358e87b8eef5b81a71b530fd70cc0fe
// Retrieved: 2026-11-10
// License: Apache-2.0
// Deviations: Original persistence of final consumer-bound native update evidence; retain per-reference digest failures without retrying tag discovery.

package imageupdate

import (
	"context"
	"strings"

	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/pkg/utils/tagpolicy"
)

func (s *ImageUpdateService) saveConsumerObservation(ctx context.Context, consumer container.Summary, result *checkDTO.Response, local identity.LocalImage, native *identity.NativeObservation) error {
	if s.db == nil {
		return nil
	}
	named, err := reference.ParseNormalizedNamed(consumer.Image)
	if err != nil {
		return imageDTO.Failure("image.invalid-reference", 400)
	}
	tagged, ok := reference.TagNameOnly(named).(reference.Tagged)
	if !ok {
		return imageDTO.Failure("image.invalid-reference", 400)
	}
	// Registry reads can outlive this instance or policy. Save a non-actionable
	// observation on a mismatch, replacing any earlier candidate for this ID.
	c, err := s.dockerClientInternal(ctx)
	if err != nil {
		return imageDTO.Failure("update.check-unavailable", 503)
	}
	current, err := c.ContainerInspect(ctx, consumer.ID, client.ContainerInspectOptions{})
	key := imageref.UpdatePolicyKey(consumer.Image, consumer.Labels)
	if err != nil || current.Container.ID != consumer.ID || current.Container.Image != consumer.ImageID || current.Container.Config == nil || imageref.UpdatePolicyKey(current.Container.Config.Image, current.Container.Config.Labels) != key {
		result.State, result.HasUpdate, result.Code = "unknown", false, "update.check-unavailable"
	}
	record := ImageUpdateRecord{ID: "container::" + consumer.ID, ContainerID: consumer.ID, ImageID: consumer.ImageID, PolicyKey: key, Repository: reference.TrimNamed(named).String(), Tag: tagged.Tag(), CheckTime: result.CheckTime, CurrentVersion: result.CurrentVersion, LatestVersion: &result.LatestVersion, UpdateType: result.UpdateType, HasUpdate: result.HasUpdate, AuthMethod: &result.AuthState}
	if result.State != "unknown" && result.State != "error" && result.Error == "" && local.ID.String() == consumer.ImageID {
		if native == nil {
			// The same-tag observation was already resolved for this consumer's store.
			marker, current, _ := local.CurrentObservation().Storage()
			if current == result.CurrentDigest {
				if observed, e := identity.DecodeObservation(marker, result.CurrentDigest, result.LatestDigest); e == nil {
					native = &observed
				}
			}
		}
		if native != nil {
			record.SetNativeObservation(*native)
		}
	}
	if result.Error != "" {
		record.LastError = &result.Error
	}
	if result.Code != "" && record.LastError == nil {
		record.LastError = new(imageDTO.Failure(result.Code, 503).Error())
	}
	if err := s.db.WithContext(ctx).Save(&record).Error; err != nil {
		return imageDTO.Failure("update.persistence-failed", 500)
	}
	return nil
}

// A failed reference observation must not leave an earlier consumer candidate
// actionable when discovery cannot produce a replacement observation.
func (s *ImageUpdateService) invalidateConsumerObservations(ctx context.Context, ref string, result *checkDTO.Response) error {
	if s.db == nil {
		return nil
	}
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return nil
	}
	tagged, ok := reference.TagNameOnly(named).(reference.Tagged)
	if !ok {
		return nil
	}
	code := result.Code
	if code == "" {
		code = "update.check-unavailable"
	}
	err = s.db.WithContext(ctx).Model(&ImageUpdateRecord{}).Where("repository=? AND tag=? AND container_id <> ''", reference.TrimNamed(named).String(), tagged.Tag()).Updates(map[string]any{"platform": nil, "has_update": false, "last_error": imageDTO.Failure(code, 503).Error(), "check_time": result.CheckTime}).Error
	if err != nil {
		return imageDTO.Failure("update.persistence-failed", 500)
	}
	return nil
}

// Failed image discovery keeps tag-policy failure behavior and persists the
// same failure for each digest consumer, in any state, without another registry read.
func (s *ImageUpdateService) saveFailedDigestObservations(ctx context.Context, ref string, result *checkDTO.Response) error {
	var consumers []container.Summary
	if s.db != nil && !strings.Contains(ref, "@") {
		if c, err := s.dockerClientInternal(ctx); err == nil {
			if listed, err := c.ContainerList(ctx, client.ContainerListOptions{All: true}); err == nil {
				for _, cnt := range listed.Items {
					policy, err := tagpolicy.Resolve(cnt.Image, updater.DefaultLabelPolicy().TagPolicy(cnt.Labels))
					if err == nil && policy.Strategy == "digest" && !imageref.IsUpdateCheckDisabled(cnt.Labels) && SameUpdateReference(cnt.Image, ref) {
						consumers = append(consumers, cnt)
					}
				}
			}
		}
	}
	if err := s.deleteContainerUpdateRecordsInternal(ctx, consumers); err != nil {
		return imageDTO.Failure("update.persistence-failed", 500)
	}
	if err := s.invalidateConsumerObservations(ctx, ref, result); err != nil {
		return err
	}
	for _, cnt := range consumers {
		scoped := *result
		scoped.ImageRef = cnt.Image
		if err := s.saveConsumerObservation(ctx, cnt, &scoped, identity.LocalImage{}, nil); err != nil {
			return err
		}
	}
	return nil
}
