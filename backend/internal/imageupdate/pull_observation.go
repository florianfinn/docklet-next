// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Acknowledge only native previews satisfied by a local pull.

package imageupdate

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	"gorm.io/gorm"
	"time"
)

func (s *ImageUpdateService) MarkImageRefUpToDateAfterPull(ctx context.Context, ref string) error {
	if s.db == nil {
		return nil
	}
	_, tag, repositories, ok := imageref.ParseUpdateLookup(ref)
	if !ok {
		return nil
	}
	var records []ImageUpdateRecord
	if err := s.db.WithContext(ctx).Where("tag = ? AND repository IN ?", tag, repositoryCandidatesSliceInternal(repositories)).Find(&records).Error; err != nil {
		return err
	}
	native := false
	type acknowledgement struct {
		record                    ImageUpdateRecord
		platform, current, latest string
	}
	ready := []acknowledgement{}
	for _, record := range records {
		info := record.UpdateInfo()
		if info.Platform == "" {
			continue
		}
		native = true
		if info.Error != "" {
			continue
		}
		local, err := s.ResolveLocal(ctx, ref, info.Platform)
		if err != nil {
			return err
		}
		if local.Key.String() != info.LatestDigest {
			continue
		}
		// A newly pulled local ID cannot acknowledge a still-running older consumer.
		if record.ContainerID != "" && record.ImageID != local.ID.String() {
			continue
		}
		p, current, latest := local.CurrentObservation().Storage()
		ready = append(ready, acknowledgement{record, p, current, latest})
	}
	if !native {
		return s.markLegacyImageRefAfterPull(ctx, ref)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, ack := range ready {
			record := ack.record
			// A later check may replace this preview while Docker reads are in flight.
			if err := tx.Model(&ImageUpdateRecord{}).Where(map[string]any{"id": record.ID, "check_time": record.CheckTime, "policy_key": record.PolicyKey, "latest_digest": record.LatestDigest, "latest_version": record.LatestVersion, "current_digest": record.CurrentDigest, "last_error": record.LastError, "has_update": record.HasUpdate}).Updates(map[string]any{"has_update": false, "platform": ack.platform, "current_digest": ack.current, "latest_digest": ack.latest, "current_version": tag, "latest_version": tag, "check_time": time.Now().UTC()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
